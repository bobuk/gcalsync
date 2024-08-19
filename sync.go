// sync.go
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"
)

func syncCalendars() {
	config, err := readConfig(".gcalsync.toml")
	if err != nil {
		log.Fatalf("Error reading config file: %v", err)
	}
	useReminders := !config.DisableReminders

	db, err := openDB(".gcalsync.db")
	if err != nil {
		log.Fatalf("Error opening database: %v", err)
	}
	defer db.Close()

	calendars := getCalendarsFromDB(db)

	fmt.Println("🚀 Starting calendar synchronization...")
	for accountName, calendarIDs := range calendars {
		fmt.Printf("📅 Syncing calendars for account: %s\n", accountName)
		for _, calendarID := range calendarIDs {
			fmt.Printf("  ↪️ Syncing calendar: %s\n", calendarID)
			err := syncCalendar(db, calendarID, calendars, accountName, useReminders)
			if err != nil {
				log.Printf("Error syncing calendar %s for account %s: %v\n", calendarID, accountName, err)
			}
		}

		fmt.Println("✅ Calendar synchronization completed successfully!")
	}

	fmt.Println("Calendars synced successfully")
}

func getCalendarsFromDB(db *sql.DB) map[string][]string {
	calendars := make(map[string][]string)
	rows, _ := db.Query("SELECT account_name, calendar_id FROM calendars")
	defer rows.Close()
	for rows.Next() {
		var accountName, calendarID string
		if err := rows.Scan(&accountName, &calendarID); err != nil {
			log.Fatalf("Error scanning calendar row: %v", err)
		}
		calendars[accountName] = append(calendars[accountName], calendarID)
	}
	return calendars
}

func syncCalendar(db *sql.DB, calendarID string, calendars map[string][]string, accountName string, useReminders bool) error {
	ctx := context.Background()
	var calendarService *calendar.Service

	// Handle token expiry/re-authentication
	for retries := 0; retries < 3; retries++ {
		log.Printf("Attempt %d to get client for account %s\n", retries+1, accountName)
		client, err := getClient(ctx, oauthConfig, db, accountName)
		if err != nil {
			if strings.Contains(err.Error(), "invalid token") || strings.Contains(err.Error(), "no token found") {
				fmt.Printf("❗️ Authentication error for account %s. Attempting to re-authenticate.\n", accountName)
				err = reAuthenticateAccount(db, accountName)
				if err != nil {
					fmt.Printf("Failed to re-authenticate account %s: %v\n", accountName, err)
					continue
				}
			} else {
				log.Printf("1. Error getting client: %v\n", err)
				return fmt.Errorf("error getting client: %v", err)
			}
		} else {
			calendarService, err = calendar.NewService(ctx, option.WithHTTPClient(client))
			if err == nil {
				break
			}
			return fmt.Errorf("error creating calendar service: %v", err)
		}
	}

	if calendarService == nil {
		return fmt.Errorf("failed to create calendar service after retries")
	}

	pageToken := ""
	now := time.Now()
	startOfCurrentMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	endOfNextMonth := startOfCurrentMonth.AddDate(0, 2, -1)
	timeMin := startOfCurrentMonth.Format(time.RFC3339)
	timeMax := endOfNextMonth.Format(time.RFC3339)

	var allEventsId = map[string]bool{}

	for {
		fmt.Printf("    📥 Retrieving events for calendar: %s\n", calendarID)
		events, err := calendarService.Events.List(calendarID).
			PageToken(pageToken).
			SingleEvents(true).
			TimeMin(timeMin).
			TimeMax(timeMax).
			OrderBy("startTime").
			Do()
		if err != nil {
			return fmt.Errorf("error retrieving events: %v", err)
		}

		for _, event := range events.Items {
			allEventsId[event.Id] = true
			// Google marks "working locations" as events, but we don't want to sync them
			if event.EventType == "workingLocation" {
				continue
			}
			if !strings.Contains(event.Summary, "O_o") {
				fmt.Printf("    ✨ Syncing event: %s\n", event.Summary)
				for otherAccountName, calendarIDs := range calendars {
					for _, otherCalendarID := range calendarIDs {
						if otherCalendarID != calendarID {
							var existingBlockerEventID string
							var last_updated string
							var originCalendarID string
							err := db.QueryRow("SELECT event_id, last_updated, origin_calendar_id FROM blocker_events WHERE calendar_id = ? AND origin_event_id = ?", otherCalendarID, event.Id).Scan(&existingBlockerEventID, &last_updated, &originCalendarID)
							if err == nil && last_updated == event.Updated && originCalendarID == calendarID {
								fmt.Printf("      ⚠️ Blocker event already exists for origin event ID %s in calendar %s and up to date\n", event.Id, otherCalendarID)
								continue
							}

							client, err := getClient(ctx, oauthConfig, db, otherAccountName)
							if err != nil {
								return fmt.Errorf("error getting client: %v", err)
							}
							otherCalendarService, err := calendar.NewService(ctx, option.WithHTTPClient(client))
							if err != nil {
								return fmt.Errorf("error getting calendar service: %v", err)
							}

							blockerSummary := fmt.Sprintf("O_o %s", event.Summary)
							blockerDescription := event.Description

							if event.End == nil {
								startTime, _ := time.Parse(time.RFC3339, event.Start.DateTime)
								duration := time.Hour
								endTime := startTime.Add(duration)
								event.End = &calendar.EventDateTime{DateTime: endTime.Format(time.RFC3339)}
							}

							blockerEvent := &calendar.Event{
								Summary:     blockerSummary,
								Description: blockerDescription,
								Start:       event.Start,
								End:         event.End,
								Attendees: []*calendar.EventAttendee{
									{Email: otherCalendarID},
								},
							}
							if !useReminders {
								blockerEvent.Reminders = nil
							}

							var res *calendar.Event

							if existingBlockerEventID != "" {
								res, err = otherCalendarService.Events.Update(otherCalendarID, existingBlockerEventID, blockerEvent).Do()
							} else {
								res, err = otherCalendarService.Events.Insert(otherCalendarID, blockerEvent).Do()
							}
							if err == nil {
								fmt.Printf("      ➕ Blocker event created or updated: %s\n", blockerEvent.Summary)
								fmt.Printf("      📅 Destination calendar: %s\n", otherCalendarID)
								result, err := db.Exec(`INSERT OR REPLACE INTO blocker_events (event_id, origin_calendar_id, calendar_id, account_name, origin_event_id, last_updated)
														VALUES (?, ?, ?, ?, ?, ?)`, res.Id, calendarID, otherCalendarID, otherAccountName, event.Id, event.Updated)
								if err != nil {
									log.Printf("Error inserting blocker event into database: %v\n", err)
								} else {
									rowsAffected, _ := result.RowsAffected()
									fmt.Printf("      📥 Blocker event inserted into database. Rows affected: %d\n", rowsAffected)
								}
							}

							if err != nil {
								return fmt.Errorf("error creating or updating blocker event: %v", err)
							}
						}
					}
				}
			}
		}
		pageToken = events.NextPageToken
		if pageToken == "" {
			break
		}
	}

	// Delete blocker events that not exists from this calendar in other calendars
	fmt.Printf("    🗑 Deleting blocker events that not exists in calendar %s from other calendars\n", calendarID)
	for otherAccountName, calendarIDs := range calendars {
		for _, otherCalendarID := range calendarIDs {
			if otherCalendarID != calendarID {
				client, err := getClient(ctx, oauthConfig, db, otherAccountName)
				if err != nil {
					log.Printf("3. Error getting client: %v\n", err)
					return fmt.Errorf("error getting client for account %s: %v", otherAccountName, err)
				}
				otherCalendarService, err := calendar.NewService(ctx, option.WithHTTPClient(client))
				rows, err := db.Query("SELECT event_id, origin_event_id FROM blocker_events WHERE calendar_id = ? AND origin_calendar_id = ?", otherCalendarID, calendarID)
				if err != nil {
					return fmt.Errorf("error creating calendar service for account %s: %v", otherAccountName, err)
				}
				eventsToDelete := make([]string, 0)

				defer rows.Close()
				for rows.Next() {
					var eventID string
					var originEventID string
					if err := rows.Scan(&eventID, &originEventID); err != nil {
						return fmt.Errorf("error scanning blocker event row: %v", err)
					}

					if val := allEventsId[originEventID]; !val {

						res, err := calendarService.Events.Get(calendarID, originEventID).Do()
						if err != nil || res == nil || res.Status == "cancelled" {
							fmt.Printf(" Event marked for deletion: %s\n", eventID)
							eventsToDelete = append(eventsToDelete, eventID)
						}
					}
				}

				for _, eventID := range eventsToDelete {
					fmt.Printf("      🗑 Deleting blocker event: %s\n", eventID)
					res, err := otherCalendarService.Events.Get(otherCalendarID, eventID).Do()

					alreadyDeleted := false

					if err != nil {
						alreadyDeleted = strings.Contains(err.Error(), "410")
						if !alreadyDeleted {
							return fmt.Errorf("error retrieving blocker event: %v", err)
						}
					}

					if !alreadyDeleted {
						err = otherCalendarService.Events.Delete(otherCalendarID, eventID).Do()
						if err != nil {
							return fmt.Errorf("error deleting blocker event: %v", err)
						}
					}
					_, err = db.Exec("DELETE FROM blocker_events WHERE event_id = ?", eventID)
					if err != nil {
						return fmt.Errorf("error deleting blocker event from database: %v", err)
					}

					fmt.Printf("      ✅ Blocker event deleted: %s\n", res.Summary)
				}
			}
		}
	}

	return nil
}
