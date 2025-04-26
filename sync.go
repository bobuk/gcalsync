// sync.go
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"
)

func syncCalendars() {
	config, err := readConfig(".gcalsync.toml")
	if err != nil {
		log.Fatalf("Error reading config file: %v", err)
	}
	useReminders := config.General.DisableReminders
	eventVisibility := config.General.EventVisibility

	db, err := openDB(".gcalsync.db")
	if err != nil {
		log.Fatalf("Error opening database: %v", err)
	}
	defer db.Close()

	// Ensure provider_config column exists
	_, err = db.Exec(`ALTER TABLE calendars ADD COLUMN provider_config TEXT DEFAULT ''`)
	if err != nil && !strings.Contains(err.Error(), "duplicate column name") {
		log.Printf("Warning: Failed to add provider_config column: %v", err)
	}

	calendars := getCalendarsFromDB(db)

	ctx := context.Background()
	fmt.Println("🚀 Starting calendar synchronization...")
	
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
				// Get the server configuration based on provider_config
				var serverConfig CalDAVConfig
				serverName := calInfo.ProviderConfig
				
				// If there's no server name stored, we need to ask the user to reconfigure this calendar
				if serverName == "" || serverName == "default" {
					log.Fatalf("Error: Calendar references removed legacy CalDAV configuration. Please remove and re-add this calendar using: ./gcalsync add")
				}
				
				// Use the server from the CalDAV servers config
				if server, ok := config.CalDAVs[serverName]; ok {
					serverConfig = server
				} else {
					log.Fatalf("Error: CalDAV server '%s' not found in configuration", serverName)
				}
				
				// Create a provider key that includes the server name to allow multiple servers
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
		
		// Sync each calendar using the appropriate provider
		for _, calInfo := range calendarInfos {
			fmt.Printf("  ↪️ Syncing %s calendar: %s\n", calInfo.ProviderType, calInfo.ID)
			
			// Determine which provider to use
			providerKey := calInfo.ProviderType
			if calInfo.ProviderKey != "" {
				providerKey = calInfo.ProviderKey
			}
			
			provider := providers[accountName][providerKey]
			if provider == nil {
				log.Fatalf("Error: Provider not found for key: %s", providerKey)
			}
			
			syncCalendarWithProvider(db, provider, calInfo.ID, calendars, accountName, useReminders, eventVisibility, calInfo.ProviderType)
		}
	}

	fmt.Println("✅ Calendar synchronization completed successfully!")
}

type CalendarInfo struct {
	ID            string
	ProviderType  string
	ProviderConfig string // Stores server name for CalDAV
	ProviderKey   string // Used to lookup the right provider
}

func getCalendarsFromDB(db *sql.DB) map[string][]CalendarInfo {
	calendars := make(map[string][]CalendarInfo)
	
	// Updated query to include provider_config
	rows, err := db.Query("SELECT account_name, calendar_id, provider_type, provider_config FROM calendars")
	if err != nil {
		// Handle case where provider_config column doesn't exist yet
		if strings.Contains(err.Error(), "no such column") {
			rows, err = db.Query("SELECT account_name, calendar_id, provider_type, '' AS provider_config FROM calendars")
			if err != nil {
				log.Fatalf("Error querying calendars: %v", err)
			}
		} else {
			log.Fatalf("Error querying calendars: %v", err)
		}
	}
	
	defer rows.Close()
	for rows.Next() {
		var accountName, calendarID, providerType, providerConfig string
		if err := rows.Scan(&accountName, &calendarID, &providerType, &providerConfig); err != nil {
			log.Fatalf("Error scanning calendar row: %v", err)
		}
		// Default to "google" for backwards compatibility with existing data
		if providerType == "" {
			providerType = "google"
		}
		calendars[accountName] = append(calendars[accountName], CalendarInfo{
			ID:             calendarID,
			ProviderType:   providerType,
			ProviderConfig: providerConfig,
		})
	}
	return calendars
}

// Map of account names to map of provider types to providers
var providers map[string]map[string]CalendarProvider

func syncCalendarWithProvider(db *sql.DB, provider CalendarProvider, calendarID string, calendars map[string][]CalendarInfo, accountName string, useReminders bool, eventVisibility string, providerType string) {
	
	now := time.Now()
	startOfCurrentMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	endOfNextMonth := startOfCurrentMonth.AddDate(0, 2, -1)
	
	var allEventsId = map[string]bool{}

	fmt.Printf("    📥 Retrieving events for calendar: %s\n", calendarID)
	events, err := provider.ListEvents(calendarID, startOfCurrentMonth, endOfNextMonth)
	if err != nil {
		log.Fatalf("Error retrieving events: %v", err)
	}

	for _, event := range events {
		allEventsId[event.ID] = true
		
		// Skip "working location" events (only for Google provider as it's Google-specific)
		if providerType == "google" && strings.Contains(event.Summary, "working location") {
			continue
		}
		
		if !strings.Contains(event.Summary, "O_o") {
			fmt.Printf("    ✨ Syncing event: %s\n", event.Summary)
			for otherAccountName, calendarInfos := range calendars {
				for _, otherCalendarInfo := range calendarInfos {
					if otherCalendarInfo.ID != calendarID {
						var existingBlockerEventID string
						var last_updated string
						var originCalendarID string
						var responseStatus string
						err := db.QueryRow("SELECT event_id, last_updated, origin_calendar_id, response_status FROM blocker_events WHERE calendar_id = ? AND origin_event_id = ?", 
							otherCalendarInfo.ID, event.ID).Scan(&existingBlockerEventID, &last_updated, &originCalendarID, &responseStatus)

						// We'll use current time as update time if not available
						updatedTime := time.Now().Format(time.RFC3339)
						
						// Get original event's response status for the calendar owner
						originalResponseStatus := "accepted" // default

						// Only skip if event exists, is up to date, and response status hasn't changed
						if err == nil && last_updated == updatedTime && originCalendarID == calendarID && responseStatus == originalResponseStatus {
							fmt.Printf("      ⚠️ Blocker event already exists for origin event ID %s in calendar %s and up to date\n", event.ID, otherCalendarInfo.ID)
							continue
						}

						// Get provider for target calendar
						providerKey := otherCalendarInfo.ProviderType
						if otherCalendarInfo.ProviderKey != "" {
							providerKey = otherCalendarInfo.ProviderKey
						}
						
						otherProvider := providers[otherAccountName][providerKey]
						if otherProvider == nil {
							log.Fatalf("Error: Provider not found for account %s, key %s", otherAccountName, providerKey)
						}

						blockerSummary := fmt.Sprintf("O_o %s", event.Summary)
						blockerDescription := event.Description

						// Ensure event has end time
						endTime := event.End
						if endTime.IsZero() {
							endTime = event.Start.Add(time.Hour)
						}

						// Create blocker event
						blockerEvent := &Event{
							Summary:     blockerSummary,
							Description: blockerDescription,
							Start:       event.Start,
							End:         endTime,
							Status:      "confirmed",
						}

						var newEventID string

						if existingBlockerEventID != "" {
							// Update existing blocker event
							err = otherProvider.UpdateEvent(otherCalendarInfo.ID, existingBlockerEventID, blockerEvent)
							newEventID = existingBlockerEventID
						} else {
							// Create new blocker event
							newEventID, err = otherProvider.AddEvent(otherCalendarInfo.ID, blockerEvent)
						}

						if err == nil {
							fmt.Printf("      ➕ Blocker event created or updated: %s (Response: %s)\n", blockerEvent.Summary, originalResponseStatus)
							fmt.Printf("      📅 Destination calendar: %s\n", otherCalendarInfo.ID)
							result, err := db.Exec(`INSERT OR REPLACE INTO blocker_events
								(event_id, origin_calendar_id, calendar_id, account_name, origin_event_id, last_updated, response_status)
								VALUES (?, ?, ?, ?, ?, ?, ?)`,
								newEventID, calendarID, otherCalendarInfo.ID, otherAccountName, event.ID, updatedTime, originalResponseStatus)
							if err != nil {
								log.Printf("Error inserting blocker event into database: %v\n", err)
							} else {
								rowsAffected, _ := result.RowsAffected()
								fmt.Printf("      📥 Blocker event inserted into database. Rows affected: %d\n", rowsAffected)
							}
						}

						if err != nil {
							log.Fatalf("Error creating blocker event: %v", err)
						}
					}
				}
			}
		}
	}

	// Delete blocker events that no longer exist from this calendar in other calendars
	fmt.Printf("    🗑 Deleting blocker events that no longer exist in calendar %s from other calendars…\n", calendarID)
	for otherAccountName, calendarInfos := range calendars {
		for _, otherCalendarInfo := range calendarInfos {
			if otherCalendarInfo.ID != calendarID {
				// Get provider for target calendar
				providerKey := otherCalendarInfo.ProviderType
				if otherCalendarInfo.ProviderKey != "" {
					providerKey = otherCalendarInfo.ProviderKey
				}
				
				otherProvider := providers[otherAccountName][providerKey]
				rows, err := db.Query("SELECT event_id, origin_event_id FROM blocker_events WHERE calendar_id = ? AND origin_calendar_id = ?", 
					otherCalendarInfo.ID, calendarID)
				if err != nil {
					log.Fatalf("Error retrieving blocker events: %v", err)
				}
				
				eventsToDelete := make([]string, 0)

				defer rows.Close()
				for rows.Next() {
					var eventID string
					var originEventID string
					if err := rows.Scan(&eventID, &originEventID); err != nil {
						log.Fatalf("Error scanning blocker event row: %v", err)
					}

					// Check if original event still exists
					if val := allEventsId[originEventID]; !val {
						// Try to get the event from the original calendar to verify it's truly gone
						var sourceEvent *Event
						// We'll catch the error later if the event doesn't exist
						sourceEvent, _ = getEventFromProvider(provider, calendarID, originEventID)
						
						if sourceEvent == nil || sourceEvent.Status == "cancelled" {
							fmt.Printf("    🚩 Event marked for deletion: %s\n", eventID)
							eventsToDelete = append(eventsToDelete, eventID)
						}
					}
				}

				for _, eventID := range eventsToDelete {
					fmt.Printf("      🗑 Deleting blocker event: %s\n", eventID)
					
					// Check if the event still exists in the target calendar
					var targetEvent *Event
					targetEvent, err = getEventFromProvider(otherProvider, otherCalendarInfo.ID, eventID)
					
					alreadyDeleted := false
					if err != nil || targetEvent == nil {
						alreadyDeleted = true
					}

					if !alreadyDeleted {
						err = otherProvider.DeleteEvent(otherCalendarInfo.ID, eventID)
						if err != nil {
							if targetEvent != nil && targetEvent.Status != "cancelled" {
								log.Fatalf("Error deleting blocker event: %v", err)
							} else {
								fmt.Printf("     ❗️ Event already deleted in the other calendar: %s\n", eventID)
							}
						}
					}
					
					_, err = db.Exec("DELETE FROM blocker_events WHERE event_id = ?", eventID)
					if err != nil {
						log.Fatalf("Error deleting blocker event from database: %v", err)
					}

					fmt.Printf("      ✅ Blocker event deleted: %s\n", eventID)
				}
			}
		}
	}
}

// Helper function to get a single event from a provider
func getEventFromProvider(provider CalendarProvider, calendarID, eventID string) (*Event, error) {
	// This implementation is a temporary solution since the interface doesn't have a GetEvent method
	// Each provider should implement GetEvent in future versions
	// For now, we'll query over a large time range to try to find the event
	
	// Query over a 10-year range to try to find the event
	startTime := time.Now().AddDate(-5, 0, 0) // 5 years ago
	endTime := time.Now().AddDate(5, 0, 0)    // 5 years in future
	
	events, err := provider.ListEvents(calendarID, startTime, endTime)
	if err != nil {
		return nil, err
	}
	
	for _, event := range events {
		if event.ID == eventID {
			return event, nil
		}
	}
	
	return nil, fmt.Errorf("event not found")
}