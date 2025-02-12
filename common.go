package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
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
	EventVisibility  string `toml:"block_event_visibility"`
	AuthorizedPorts  []int  `toml:"authorized_ports"`
}

var oauthConfig *oauth2.Config
var configDir string

func initOAuthConfig(config *Config) {
	oauthConfig = &oauth2.Config{
		ClientID:     config.ClientID,
		ClientSecret: config.ClientSecret,
		Endpoint:     google.Endpoint,
		Scopes:       []string{calendar.CalendarScope},
		// RedirectURL will be set dynamically in getTokenFromWeb
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

func getTokenFromWeb(config *oauth2.Config, cfg *Config) *oauth2.Token {
	// Start local server
	listener, err := findAvailablePort(cfg.AuthorizedPorts)
	if err != nil {
		log.Fatalf("Unable to start listener: %v", err)
	}
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port
	config.RedirectURL = fmt.Sprintf("http://localhost:%d", port)

	codeChan := make(chan string)

	var server *http.Server
	server = &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			code := r.URL.Query().Get("code")
			codeChan <- code
			fmt.Fprintf(w, "Authorization successful! You can close this window.")
			go func() {
				time.Sleep(time.Second)
				server.Shutdown(context.Background())
			}()
		}),
	}

	go server.Serve(listener)

	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("Please visit this URL to authorize the application: \n%v\n", authURL)

	// Open browser automatically
	// err = openBrowser(authURL)
	// if err != nil {
	// 	fmt.Printf("Failed to open browser automatically: %v\n", err)
	// 	fmt.Println("Please open the URL manually in your browser.")
	// }

	// Copy URL to clipboard
	err = copyUrlToClipboard(authURL)
	if err != nil {
		fmt.Printf("Failed to copy URL to clipboard: %v\n", err)
		fmt.Println("Please copy the URL manually and open it in your browser.")
	}

	code := <-codeChan

	tok, err := config.Exchange(context.TODO(), code)
	if err != nil {
		log.Fatalf("Unable to retrieve token: %v", err)
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

func getClient(ctx context.Context, config *oauth2.Config, db *sql.DB, accountName string, cfg *Config) *http.Client {
	var tokenJSON []byte
	err := db.QueryRow("SELECT token FROM tokens WHERE account_name = ?", accountName).Scan(&tokenJSON)
	if err != nil {
		if err == sql.ErrNoRows {
			fmt.Printf("  ❗️ No token found for account %s. Obtaining a new token.\n", accountName)
			token := getTokenFromWeb(config, cfg)
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
		if strings.Contains(err.Error(), "Token has been expired or revoked") || strings.Contains(err.Error(), "invalid_grant") {
			fmt.Printf("  ❗️ Token expired or revoked for account %s. Obtaining a new token.\n", accountName)
			newToken = getTokenFromWeb(config, cfg)
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

// Helper function to find an available port in a range
func findAvailablePort(authorizedPorts []int) (net.Listener, error) {
	for _, port := range authorizedPorts {
		listener, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", port))
		if err == nil {
			return listener, nil
		}
	}
	return nil, fmt.Errorf("no available ports in range %v", authorizedPorts)
}

// Open a URL in the default browser
func openBrowser(url string) error {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start"}
	case "darwin":
		cmd = "open"
	default: // "linux", "freebsd", "openbsd", "netbsd"
		cmd = "xdg-open"
	}
	args = append(args, url)
	return exec.Command(cmd, args...).Start()
}

// Copy a URL into a clipboard automatically
func copyUrlToClipboard(url string) error {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "echo", url, "|", "clip"}
	case "darwin":
		cmd = "pbcopy"
		args = []string{url}
	default: // "linux", "freebsd", "openbsd", "netbsd"
		cmd = "xclip"
		args = []string{"-selection", "clipboard"}
	}
	args = append(args, url)
	return exec.Command(cmd, args...).Run()
}
