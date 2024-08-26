package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
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
}

type Token struct {
	AccessToken  string    `json:"access_token"`
	TokenType    string    `json:"token_type"`
	RefreshToken string    `json:"refresh_token"`
	Expiry       time.Time `json:"expiry"`
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
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline, oauth2.ApprovalForce)
	fmt.Printf("Go to the following link in your browser then type or paste the "+
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

func getClient(ctx context.Context, config *oauth2.Config, db *sql.DB, accountName string) (*http.Client, error) {
	var tokenJSON []byte
	err := db.QueryRow("SELECT token FROM tokens WHERE account_name = ?", accountName).Scan(&tokenJSON)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("no token found for account %s", accountName)
		}
		return nil, fmt.Errorf("error retrieving token from database: %v", err)
	}

	var token oauth2.Token
	err = json.Unmarshal(tokenJSON, &token)
	if err != nil {
		log.Fatalf("Error unmarshaling token: %v", err)
	}

	tokenSource := config.TokenSource(ctx, &token)
	client := oauth2.NewClient(ctx, tokenSource)

	// Test the token
	testURL := "https://www.googleapis.com/oauth2/v3/tokeninfo"
	resp, err := client.Get(testURL)
	if err != nil || resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("invalid token for account %s", accountName)
	}

	return client, nil
}

func refreshToken(ctx context.Context, db *sql.DB, accountName string) (*oauth2.Token, error) {
	var tokenJSON []byte
	err := db.QueryRow("SELECT token FROM tokens WHERE account_name = ?", accountName).Scan(&tokenJSON)
	if err != nil {
		return nil, fmt.Errorf("error retrieving token from database: %v", err)
	}

	var token oauth2.Token
	err = json.Unmarshal(tokenJSON, &token)
	if err != nil {
		return nil, fmt.Errorf("error unmarshaling token: %v", err)
	}

	tokenSource := oauthConfig.TokenSource(ctx, &token)
	newToken, err := tokenSource.Token()
	if err != nil {
		return nil, fmt.Errorf("error refreshing token: %v", err)
	}

	if newToken.AccessToken != token.AccessToken {
		fmt.Printf("Token refreshed for account %s.\n", accountName)
		saveToken(db, accountName, newToken)
	}

	return newToken, nil
}

func getCalendarService(ctx context.Context, db *sql.DB, accountName string) (*calendar.Service, error) {
	client, err := getClient(ctx, oauthConfig, db, accountName)
	if err != nil {
		return nil, fmt.Errorf("unable to get client: %v", err)
	}
	service, err := calendar.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, fmt.Errorf("unable to create calendar service: %v", err)
	}
	return service, nil
}

func reAuthenticateAccount(db *sql.DB, accountName string) error {
	fmt.Printf("Please re-authenticate account %s\n", accountName)
	token := getTokenFromWeb(oauthConfig)
	return saveToken(db, accountName, token)
}
