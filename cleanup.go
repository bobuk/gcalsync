package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"
)

func cleanupCalendars() {
	config, err := readConfig(".gcalsync.toml")
	if err != nil {
		log.Fatalf("Error reading config file: %v", err)
	}

	db, err := openDB(".gcalsync.db")
	if err != nil {
		log.Fatalf("Error opening database: %v", err)
	}
	defer db.Close()

	calendars := getCalendarsFromDB(db)

	ctx := context.Background()

	for accountName, calendarIDs := range calendars {
		client := getClient(ctx, oauthConfig, db, accountName, config)
		calendarService, err := calendar.NewService(ctx, option.WithHTTPClient(client))
		if err != nil {
			log.Fatalf("Error creating calendar client: %v", err)
		}

		for _, calendarID := range calendarIDs {
			fmt.Printf("🧹 Cleaning up calendar: %s\n", calendarID)
			cleanupCalendar(calendarService, calendarID)
			db.Exec("DELETE FROM blocker_events WHERE calendar_id = ?", calendarID)
		}
	}

	fmt.Println("Calendars desynced successfully")
}

func cleanupCalendar(calendarService *calendar.Service, calendarID string) {
	// ctx := context.Background()
	pageToken := ""

	for {
		events, err := calendarService.Events.List(calendarID).
			PageToken(pageToken).
			SingleEvents(true).
			OrderBy("startTime").
			Do()
		if err != nil {
			log.Fatalf("Error retrieving events: %v", err)
		}

		for _, event := range events.Items {
			if strings.Contains(event.Summary, "O_o") {
				err := calendarService.Events.Delete(calendarID, event.Id).Do()
				fmt.Printf("Deleted event %s from calendar %s\n", event.Summary, calendarID)
				if err != nil {
					log.Fatalf("Error deleting blocker event: %v", err)
				}
			}
		}

		pageToken = events.NextPageToken
		if pageToken == "" {
			break
		}
	}
}
