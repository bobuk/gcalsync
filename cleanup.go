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

	calendars := getCalendarsFromDB(db)

	ctx := context.Background()
	
	// Create provider instances for each account
	providers = make(map[string]map[string]CalendarProvider)
	
	for accountName, calendarInfos := range calendars {
		fmt.Printf("📅 Setting up account: %s\n", accountName)
		providers[accountName] = make(map[string]CalendarProvider)
		
		// Initialize providers for each type needed by this account
		for i, calInfo := range calendarInfos {
			switch calInfo.ProviderType {
			case "google":
				if _, exists := providers[accountName]["google"]; !exists {
					client := getClient(ctx, oauthConfig, db, accountName, config)
					googleProvider, err := NewGoogleCalendarProvider(ctx, client)
					if err != nil {
						log.Fatalf("Error creating Google calendar provider: %v", err)
					}
					providers[accountName]["google"] = googleProvider
				}
				
			case "caldav":
				// Get the server configuration from provider_config
				var serverConfig CalDAVConfig
				serverName := calInfo.ProviderConfig
				
				// If there's no server name, we need the user to reconfigure
				if serverName == "" || serverName == "default" {
					log.Fatalf("Error: Calendar references removed legacy CalDAV configuration. Please remove and re-add this calendar using: ./gcalsync add")
				}
				
				// Use the server from CalDAV servers config
				if server, ok := config.CalDAVs[serverName]; ok {
					serverConfig = server
				} else {
					log.Fatalf("Error: CalDAV server '%s' not found in configuration", serverName)
				}
				
				// Create a provider key that includes the server name
				providerKey := "caldav-" + serverName
				
				// Only create the provider if we don't already have one for this server
				if _, exists := providers[accountName][providerKey]; !exists {
					caldavProvider, err := NewCalDAVProvider(ctx, serverConfig.ServerURL, serverConfig.Username, serverConfig.Password)
					if err != nil {
						log.Fatalf("Error connecting to CalDAV server %s: %v", serverName, err)
					}
					providers[accountName][providerKey] = caldavProvider
				}
				
				// Update the calendar info to use the correct provider key
				calendarInfos[i].ProviderKey = providerKey
				
			default:
				log.Fatalf("Error: Unsupported provider type: %s", calInfo.ProviderType)
			}
		}

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

	fmt.Println("Calendars desynced successfully")
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