package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	"google.golang.org/api/calendar/v3"
)

func cleanupCalendars() {
	db, err := openDB(".gcalsync.db")
	if err != nil {
		log.Fatalf("Error opening database: %v", err)
	}
	defer db.Close()

	calendars := getCalendarsFromDB(db)

	ctx := context.Background()

	for accountName, calendarIDs := range calendars {
		client := getClient(ctx, oauthConfig, db, accountName)
		calendarService, err := calendar.New(client)
		if err != nil {
			log.Fatalf("Error creating calendar client: %v", err)
		}

		for _, calendarID := range calendarIDs {
			cleanupCalendar(calendarService, calendarID)
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
