package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"

	"github.com/BurntSushi/toml"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/calendar/v3"
	_ "github.com/mattn/go-sqlite3"

)

type Config struct {
	ClientID     string `toml:"client_id"`
	ClientSecret string `toml:"client_secret"`
}

var oauthConfig *oauth2.Config

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
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var config Config
	if err := toml.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return &config, nil
}

func openDB(filename string) (*sql.DB, error) {
	return sql.Open("sqlite3", filename)
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
			fmt.Printf("No token found for account %s. Obtaining a new token.\n", accountName)
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

	return config.Client(ctx, &token)
}
