package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"
)

type Config struct {
	ClientID         string `toml:"client_id"`
	ClientSecret     string `toml:"client_secret"`
	DisableReminders bool   `toml:"disable_reminders"`
	VerbosityLevel   int    `toml:"verbosity_level"`
}

var oauthConfig *oauth2.Config
var configDir string

func initOAuthConfig(config *Config) {
	oauthConfig = &oauth2.Config{
		ClientID:     config.ClientID,
		ClientSecret: config.ClientSecret,
		Endpoint:     google.Endpoint,
		RedirectURL:  "urn:ietf:wg:oauth:2.0:oob",
		Scopes:       []string{calendar.CalendarScope},
	}
}

func readConfig(filename string) (*Config, error) {
	// Try first current dir, then `$HOME/.config/gcalsync/`
	data, err := os.ReadFile(filename)
	if err != nil {
		data, err = os.ReadFile(os.Getenv("HOME") + "/.config/gcalsync/" + filename)
		if err != nil {
			return nil, err
		}
		configDir = os.Getenv("HOME") + "/.config/gcalsync/"
	}

	var config Config
	if err := toml.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	verbosityLevel = config.VerbosityLevel

	return &config, nil
}

func openDB(filename string) (*sql.DB, error) {
	// Try first the same dir, where the config file was found
	db, err := sql.Open("sqlite3", configDir+filename)
	if err != nil {
		// Try the current dir
		db, err = sql.Open("sqlite3", filename)
		if err != nil {
			return nil, err
		}
	}
	return db, nil
}

func getTokenFromWeb(config *oauth2.Config) *oauth2.Token {
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("Go to the following link in your browser then type the "+
		"authorization code: \n%v\n", authURL)

	var authCode string
	if _, err := fmt.Scan(&authCode); err != nil {
		log.Fatalf("Unable to read authorization code: %v", err)
	}

	tok, err := config.Exchange(context.TODO(), authCode)
	if err != nil {
		log.Fatalf("Unable to retrieve token from web: %v", err)
	}
	return tok
}

func saveToken(db *sql.DB, accountName string, token *oauth2.Token) error {
	tokenJSON, err := json.Marshal(token)
	if err != nil {
		return err
	}

	_, err = db.Exec("INSERT OR REPLACE INTO tokens (account_name, token) VALUES (?, ?)", accountName, tokenJSON)
	return err
}

func getClient(ctx context.Context, config *oauth2.Config, db *sql.DB, accountName string) *http.Client {
	var tokenJSON []byte
	err := db.QueryRow("SELECT token FROM tokens WHERE account_name = ?", accountName).Scan(&tokenJSON)
	if err != nil {
		if err == sql.ErrNoRows {
			fmt.Printf("  ❗️ No token found for account %s. Obtaining a new token.\n", accountName)
			token := getTokenFromWeb(config)
			saveToken(db, accountName, token)
			return config.Client(ctx, token)
		}
		log.Fatalf("Error retrieving token from database: %v", err)
	}

	var token oauth2.Token
	err = json.Unmarshal(tokenJSON, &token)
	if err != nil {
		log.Fatalf("Error unmarshaling token: %v", err)
	}

	tokenSource := config.TokenSource(ctx, &token)
	newToken, err := tokenSource.Token()
	if err != nil {
		if strings.Contains(err.Error(), "Token has been expired or revoked") {
			fmt.Printf("  ❗️ Token expired or revoked for account %s. Obtaining a new token.\n", accountName)
			newToken = getTokenFromWeb(config)
			saveToken(db, accountName, newToken)
			return config.Client(ctx, newToken)
		}
		log.Fatalf("Error retrieving token from token source: %v", err)
	}

	if newToken.AccessToken != token.AccessToken {
		fmt.Printf("Token refreshed for account %s.\n", accountName)
		saveToken(db, accountName, newToken)
	}

	// Check if the token is expired and refresh it if necessary
	if token.Expiry.Before(time.Now()) {
		fmt.Printf("  ❗️ Token expired for account %s. Refreshing token.\n", accountName)
		newToken, err := config.TokenSource(ctx, &token).Token()
		if err != nil {
			log.Fatalf("Error refreshing token: %v", err)
		}
		saveToken(db, accountName, newToken)
		return config.Client(ctx, newToken)
	}

	return config.Client(ctx, &token)
}

// Check if the token has expired and refresh if necessary, return updated calendarService
func tokenExpired(db *sql.DB, accountName string, calendarService *calendar.Service, ctx context.Context) *calendar.Service {
	var tokenJSON []byte
	err := db.QueryRow("SELECT token FROM tokens WHERE account_name = ?", accountName).Scan(&tokenJSON)
	if err != nil {
		log.Fatalf("Error retrieving token from database: %v", err)
	}

	var token oauth2.Token
	err = json.Unmarshal(tokenJSON, &token)
	if err != nil {
		log.Fatalf("Error unmarshaling token: %v", err)
	}

	if token.Expiry.Before(time.Now()) {
		fmt.Printf("  ❗️ Token expired for account %s. Refreshing token.\n", accountName)
		newToken, err := oauthConfig.TokenSource(ctx, &token).Token()
		if err != nil {
			log.Fatalf("Error refreshing token: %v", err)
		}
		saveToken(db, accountName, newToken)

		// Create new calendar service with updated token
		calendarService, err = calendar.NewService(ctx, option.WithHTTPClient(oauthConfig.Client(ctx, newToken)))
		if err != nil {
			log.Fatalf("Unable to create new calendar service: %v", err)
		}
	}

	return calendarService
}

func printVerbosely(verbosity int, format string, a ...interface{}) {
	// Print only if verbosity is higher than verbosityLevel
	// verbosityLevel is set in the config file
	// 0 - no output, other than creitical errors
	// 1 - only list calendars being synced
	// 2 - list events being synced
	// 3 - report on blocker events created/deleted
	// 4 - report on blocker events skipped
	// 5 - report everything
	if verbosity <= verbosityLevel {
		fmt.Printf(format, a...)
	}
}
