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
	// config, err := readConfig(".gcalsync.toml")
	// if err != nil {
	// 	log.Fatalf("Error reading config file: %v", err)
	// }

	db, err := openDB(".gcalsync.db")
	if err != nil {
		log.Fatalf("Error opening database: %v", err)
	}
	defer db.Close()

	calendars := getCalendarsFromDB(db)

	ctx := context.Background()
	fmt.Println("🚀 Starting calendar synchronization...")
	for accountName, calendarIDs := range calendars {
		fmt.Printf("📅 Syncing calendars for account: %s\n", accountName)
		client := getClient(ctx, oauthConfig, db, accountName)
		calendarService, err := calendar.NewService(ctx, option.WithHTTPClient(client))
		if err != nil {
			log.Fatalf("Error creating calendar client: %v", err)
		}

		for _, calendarID := range calendarIDs {
			fmt.Printf("  ↪️ Syncing calendar: %s\n", calendarID)
			syncCalendar(db, calendarService, calendarID, calendars, accountName)
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

func syncCalendar(db *sql.DB, calendarService *calendar.Service, calendarID string, calendars map[string][]string, accountName string) {
	ctx := context.Background()
	calendarService = tokenExpired(db, accountName, calendarService, ctx)
	pageToken := ""

	now := time.Now()
	startOfCurrentMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	endOfNextMonth := startOfCurrentMonth.AddDate(0, 2, -1)
	timeMin := startOfCurrentMonth.Format(time.RFC3339)
	timeMax := endOfNextMonth.Format(time.RFC3339)

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
			log.Fatalf("Error retrieving events: %v", err)
		}

		for _, event := range events.Items {
			if !strings.Contains(event.Summary, "O_o") {
				fmt.Printf("    ✨ Syncing event: %s\n", event.Summary)
				for otherAccountName, calendarIDs := range calendars {
					for _, otherCalendarID := range calendarIDs {
						if otherCalendarID != calendarID {
							var existingBlockerEventID string
							var last_updated string
							err := db.QueryRow("SELECT event_id, last_updated FROM blocker_events WHERE calendar_id = ? AND origin_event_id = ?", otherCalendarID, event.Id, event.Updated).Scan(&existingBlockerEventID, &last_updated)
							if err == nil && last_updated == event.Updated {
								fmt.Printf("      ⚠️ Blocker event already exists for origin event ID %s in calendar %s\n and up to date", event.Id, otherCalendarID)
								continue
							}

							client := getClient(ctx, oauthConfig, db, otherAccountName)
							otherCalendarService, err := calendar.NewService(ctx, option.WithHTTPClient(client))
							if err != nil {
								log.Fatalf("Error creating calendar client: %v", err)
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
							var res *calendar.Event

							if existingBlockerEventID != "" {
								res, err = otherCalendarService.Events.Update(otherCalendarID, existingBlockerEventID, blockerEvent).Do()
							} else {
								res, err = otherCalendarService.Events.Insert(otherCalendarID, blockerEvent).Do()
							}
							if err == nil {
								fmt.Printf("      ➕ Blocker event created or updated: %s\n", blockerEvent.Summary)
								fmt.Printf("      📅 Destination calendar: %s\n", otherCalendarID)
								result, err := db.Exec(`INSERT OR REPLACE INTO blocker_events (event_id, calendar_id, account_name, origin_event_id, last_updated)
														VALUES (?, ?, ?, ?, ?)`, res.Id, otherCalendarID, otherAccountName, event.Id, event.Updated)
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
		pageToken = events.NextPageToken
		if pageToken == "" {
			break
		}
	}
}
