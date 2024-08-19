package main

import (
	"context"
	"fmt"
	"log"
)

func addCalendar() {
	db, err := openDB(".gcalsync.db")
	if err != nil {
		log.Fatalf("Error opening database: %v", err)
	}
	defer db.Close()

	fmt.Println("🚀 Starting calendar addition...")
	fmt.Print("👤 Enter account name: ")
	var accountName string
	fmt.Scanln(&accountName)

	fmt.Print("📅 Enter calendar ID: ")
	var calendarID string
	fmt.Scanln(&calendarID)

	ctx := context.Background()

	calendarService, err := getCalendarService(ctx, db, accountName)
	if err != nil {
		log.Fatalf("❌ Error creating calendar client: %v", err)
	}

	_, err = calendarService.CalendarList.Get(calendarID).Do()
	if err != nil {
		log.Fatalf("Error retrieving calendar: %v", err)
	}
	_, err = db.Exec(`INSERT INTO calendars (account_name, calendar_id) VALUES (?, ?)`, accountName, calendarID)
	if err != nil {
		log.Fatalf("Error saving calendar ID: %v", err)
	}

	fmt.Printf("✅ Calendar %s added successfully for account %s\n", calendarID, accountName)
}
