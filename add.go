package main

import (
	"context"
	"fmt"
	"log"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"
)

func addCalendar() {
	config, err := readConfig(".gcalsync.toml")
	if err != nil {
		log.Fatalf("Error reading config file: %v", err)
	}

	db, err := openDB(".gcalsync.db")
	if err != nil {
		log.Fatalf("Error opening database: %v", err)
	}
	defer db.Close()

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS tokens (
		account_name TEXT PRIMARY KEY,
		token TEXT
	)`)
	if err != nil {
		log.Fatalf("Error creating tokens table: %v", err)
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS calendars (
		account_name TEXT,
		calendar_id TEXT,
		PRIMARY KEY (account_name, calendar_id)
	)`)
	if err != nil {
		log.Fatalf("Error creating calendars table: %v", err)
	}
	fmt.Println("🚀 Starting calendar addition...")
	fmt.Print("👤 Enter account name: ")
	var accountName string
	fmt.Scanln(&accountName)

	fmt.Print("📅 Enter calendar ID: ")
	var calendarID string
	fmt.Scanln(&calendarID)

	ctx := context.Background()
	oauthConfig := &oauth2.Config{
		ClientID:     config.ClientID,
		ClientSecret: config.ClientSecret,
		Endpoint:     google.Endpoint,
		RedirectURL:  "urn:ietf:wg:oauth:2.0:oob",
		Scopes:       []string{calendar.CalendarScope},
	}

	client := getClient(ctx, oauthConfig, db, accountName)

	calendarService, err := calendar.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		log.Fatalf("Error creating calendar client: %v", err)
	}

	_, err = calendarService.CalendarList.Get(calendarID).Do()
	if err != nil {
		log.Fatalf("Error retrieving calendar: %v", err)
	}
	_, err = db.Exec(`INSERT INTO calendars (account_name, calendar_id) VALUES (?, ?)`, accountName, calendarID)
	if err != nil {
		log.Fatalf("Error saving calendar ID: %v", err)
	}

	fmt.Printf("✅ Calendar %s added successfully for account %s\n", calendarID, accountName)
}
