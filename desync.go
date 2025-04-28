package main

import (
	"context"
	"database/sql" 
	"fmt"
	"log"
	"strings"
)

func desyncCalendars() {
	config, err := readConfig(".gcalsync.toml")
	if err != nil {
		log.Fatalf("Error reading config file: %v", err)
	}

	ctx := context.Background()
	db, err := openDB(".gcalsync.db")
	if err != nil {
		log.Fatalf("Error opening database: %v", err)
	}
	defer db.Close()

	fmt.Println("🚀 Starting calendar desynchronization...")

	// Use the calendar factory to get all providers
	calendarFactory := NewCalendarFactory(ctx, config, db)
	providers, _, err := calendarFactory.GetAllCalendars()
	if err != nil {
		log.Fatalf("Error initializing calendar providers: %v", err)
	}

	// Get all blocker events
	rows, err := db.Query("SELECT be.event_id, be.calendar_id, be.account_name, c.provider_type, c.provider_config FROM blocker_events be LEFT JOIN calendars c ON be.calendar_id = c.calendar_id")
	if err != nil {
		log.Fatalf("❌ Error retrieving blocker events from database: %v", err)
	}
	defer rows.Close()

	var eventIDCalendarIDPairs []struct {
		EventID    string
		CalendarID string
	}

	for rows.Next() {
		var eventID, calendarID, accountName, providerType, providerConfig string
		if err := rows.Scan(&eventID, &calendarID, &accountName, &providerType, &providerConfig); err != nil {
			log.Fatalf("❌ Error scanning blocker event row: %v", err)
		}

		eventIDCalendarIDPairs = append(eventIDCalendarIDPairs, struct {
			EventID    string
			CalendarID string
		}{EventID: eventID, CalendarID: calendarID})

		// If provider type is empty, assume "google" for backward compatibility
		if providerType == "" {
			providerType = "google"
		}

		// For CalDAV, construct the provider key
		var providerKey string
		if providerType == "caldav" {
			if providerConfig == "" || providerConfig == "default" {
				log.Fatalf("Error: Calendar references removed legacy CalDAV configuration. Please remove and re-add this calendar using: ./gcalsync add")
			}
			providerKey = "caldav-" + providerConfig
		} else {
			providerKey = providerType
		}

		// Get the appropriate provider
		provider, ok := providers[accountName][providerKey]
		if !ok {
			// If provider isn't initialized for some reason, fail with error
			log.Fatalf("Error: Provider not found for account %s, key %s. Please run sync or add command to set up the providers.", accountName, providerKey)
		}

		// Delete the event using the provider
		err = provider.DeleteEvent(calendarID, eventID)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				fmt.Printf("  ⚠️ Blocker event not found in calendar: %s\n", eventID)
			} else {
				log.Printf("❌ Error deleting blocker event: %v", err)
			}
		} else {
			fmt.Printf("  ✅ Blocker event deleted: %s\n", eventID)
		}
	}

	// Delete blocker events from the database after the iteration
	for _, pair := range eventIDCalendarIDPairs {
		_, err := db.Exec("DELETE FROM blocker_events WHERE event_id = ? AND calendar_id = ?", pair.EventID, pair.CalendarID)
		if err != nil {
			log.Fatalf("❌ Error deleting blocker event from database: %v", err)
		} else {
			fmt.Printf("  📥 Blocker event deleted from database: %s\n", pair.EventID)
		}
	}

	fmt.Println("Calendars desynced successfully")
}

func getAccountNameByCalendarID(db *sql.DB, calendarID string) string {
	var accountName string
	err := db.QueryRow("SELECT account_name FROM calendars WHERE calendar_id = ?", calendarID).Scan(&accountName)
	if err != nil {
		log.Fatalf("Error retrieving account name for calendar ID %s: %v", calendarID, err)
	}
	return accountName
}