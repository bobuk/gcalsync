package main

import (
	"fmt"
	"log"
	"os"
)

func listCalendars() {
	db, err := openDB(".gcalsync.db")
	if err != nil {
		// Give it another try in the home directory
		db, err = openDB(os.Getenv("HOME") + "/" + ".gcalsync.db")
		if err != nil {
			log.Fatalf("Error opening database: %v", err)
		}
	}
	defer db.Close()

	fmt.Println("📋 Here's the list of calendars you are syncing:")

	rows, err := db.Query("SELECT account_name, calendar_id, count(1) as num_events FROM blocker_events GROUP BY 1,2;")
	if err != nil {
		log.Fatalf("❌ Error retrieving blocker events from database: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var accountName, calendarID string
		var numEvents int
		if err := rows.Scan(&accountName, &calendarID, &numEvents); err != nil {
			log.Fatalf("❌ Unable to read calendar record or no calendars defined: %v", err)
		}
		fmt.Printf("  👤 %s (📅 %s) - %d\n", accountName, calendarID, numEvents)
	}
}
