package main

import (
	"fmt"
	"log"
)

func listCalendars() {
	db, err := openDB(".gcalsync.db")
	if err != nil {
		log.Fatalf("Error opening database: %v", err)
	}
	defer db.Close()

	fmt.Println("📋 Here's the list of calendars you are syncing:")

	rows, err := db.Query(`SELECT c.account_name, c.calendar_id, c.mode, COALESCE(b.num_events, 0) as num_events
		FROM calendars c
		LEFT JOIN (SELECT account_name, calendar_id, count(1) as num_events FROM blocker_events GROUP BY 1,2) b
		ON c.account_name = b.account_name AND c.calendar_id = b.calendar_id`)
	if err != nil {
		log.Fatalf("❌ Error retrieving calendars from database: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var accountName, calendarID, mode string
		var numEvents int
		if err := rows.Scan(&accountName, &calendarID, &mode, &numEvents); err != nil {
			log.Fatalf("❌ Unable to read calendar record or no calendars defined: %v", err)
		}
		fmt.Printf("  👤 %s (📅 %s) [mode: %s] - %d blocker events\n", accountName, calendarID, mode, numEvents)
	}
}
