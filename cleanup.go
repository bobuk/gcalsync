package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"google.golang.org/api/calendar/v3"
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

	ctx := context.Background()
	
	// Use the calendar factory to get all providers
	calendarFactory := NewCalendarFactory(ctx, config, db)
	providers, calendars, err := calendarFactory.GetAllCalendars()
	if err != nil {
		log.Fatalf("Error initializing calendar providers: %v", err)
	}

	for accountName, calendarInfos := range calendars {
		fmt.Printf("📅 Setting up account: %s\n", accountName)

		for _, calInfo := range calendarInfos {
			fmt.Printf("🧹 Cleaning up calendar: %s\n", calInfo.ID)
			
			// Determine which provider to use
			providerKey := calInfo.ProviderType
			if calInfo.ProviderKey != "" {
				providerKey = calInfo.ProviderKey
			}
			
			provider := providers[accountName][providerKey]
			if provider == nil {
				log.Fatalf("Error: Provider not found for key: %s", providerKey)
			}
			
			cleanupCalendarWithProvider(provider, calInfo.ID)
			db.Exec("DELETE FROM blocker_events WHERE calendar_id = ?", calInfo.ID)
		}
	}

	fmt.Println("Calendars cleaned up successfully")
}

// Legacy function for backward compatibility
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

// New function that works with any CalendarProvider implementation
func cleanupCalendarWithProvider(provider CalendarProvider, calendarID string) {
	// Get all events for the next year (to ensure we catch all blockers)
	now := time.Now()
	oneYearFromNow := now.AddDate(1, 0, 0)

	events, err := provider.ListEvents(calendarID, now, oneYearFromNow)
	if err != nil {
		log.Fatalf("Error retrieving events: %v", err)
	}

	for _, event := range events {
		if strings.Contains(event.Summary, "O_o") {
			err := provider.DeleteEvent(calendarID, event.ID)
			fmt.Printf("Deleted event %s from calendar %s\n", event.Summary, calendarID)
			if err != nil {
				log.Fatalf("Error deleting blocker event: %v", err)
			}
		}
	}
}