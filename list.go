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

	// First list all calendars
	fmt.Println("\n📅 Calendars:")
	rows, err := db.Query("SELECT account_name, calendar_id, provider_type FROM calendars;")
	if err != nil {
		log.Fatalf("❌ Error retrieving calendars from database: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var accountName, calendarID, providerType string
		if err := rows.Scan(&accountName, &calendarID, &providerType); err != nil {
			log.Fatalf("❌ Unable to read calendar record: %v", err)
		}
		
		if providerType == "" {
			providerType = "google" // Default for backward compatibility
		}
		
		fmt.Printf("  👤 %s (📅 %s) [%s]\n", accountName, calendarID, providerType)
	}
	
	// Then list blocker events
	fmt.Println("\n🚫 Blocker Events:")
	blockerRows, err := db.Query("SELECT account_name, calendar_id, count(1) as num_events FROM blocker_events GROUP BY 1,2;")
	if err != nil {
		log.Fatalf("❌ Error retrieving blocker events from database: %v", err)
	}
	defer blockerRows.Close()

	for blockerRows.Next() {
		var accountName, calendarID string
		var numEvents int
		if err := blockerRows.Scan(&accountName, &calendarID, &numEvents); err != nil {
			log.Fatalf("❌ Unable to read blocker event record: %v", err)
		}
		fmt.Printf("  👤 %s (📅 %s) - %d events\n", accountName, calendarID, numEvents)
	}
}
