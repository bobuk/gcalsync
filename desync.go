package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
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

	rows, err := db.Query("SELECT event_id, calendar_id, account_name FROM blocker_events")
	if err != nil {
		log.Fatalf("❌ Error retrieving blocker events from database: %v", err)
	}
	defer rows.Close()

	var eventIDCalendarIDPairs []struct {
		EventID    string
		CalendarID string
	}

	for rows.Next() {
		var eventID, calendarID, accountName string
		if err := rows.Scan(&eventID, &calendarID, &accountName); err != nil {
			log.Fatalf("❌ Error scanning blocker event row: %v", err)
		}

		eventIDCalendarIDPairs = append(eventIDCalendarIDPairs, struct {
			EventID    string
			CalendarID string
		}{EventID: eventID, CalendarID: calendarID})

		client := getClient(ctx, oauthConfig, db, accountName, config)
		calendarService, err := calendar.NewService(ctx, option.WithHTTPClient(client))
		if err != nil {
			log.Fatalf("❌ Error creating calendar client: %v", err)
		}

		err = calendarService.Events.Delete(calendarID, eventID).Do()
		if err != nil {
			if googleErr, ok := err.(*googleapi.Error); ok && googleErr.Code == 404 {
				fmt.Printf("  ⚠️ Blocker event not found in calendar: %s\n", eventID)
			} else {
				log.Fatalf("❌ Error deleting blocker event: %v", err)
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
