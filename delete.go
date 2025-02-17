package main

import (
	"context"
	"fmt"
	"log"

	"google.golang.org/api/calendar/v3"
)

func deleteCalendar() {
	db, err := openDB(".gcalsync.db")
	if err != nil {
		log.Fatalf("Error opening database: %v", err)
	}
	defer db.Close()

	fmt.Println("🚀 Starting calendar deletion...")

	fmt.Print("📅 Enter calendar ID to delete: ")
	var calendarID string
	fmt.Scanln(&calendarID)

	fmt.Print("⚠️  Are you sure you want to delete this calendar? (y/N): ")
	var confirmation string
	fmt.Scanln(&confirmation)

	if confirmation != "y" && confirmation != "Y" {
		fmt.Println("❌ Calendar deletion cancelled")
		return
	}
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM calendars WHERE calendar_id = ?", calendarID).Scan(&count)
	if err != nil {
		log.Fatalf("Error checking if calendar exists: %v", err)
	}
	if count == 0 {
		fmt.Printf("❌ Calendar %s does not exist\n", calendarID)
		return
	}

	ctx := context.Background()
	client := getClient(ctx, oauthConfig, db, "")
	calendarService, err := calendar.New(client)
	if err != nil {
		log.Fatalf("Error creating calendar client: %v", err)
	}

	cleanupCalendar(calendarService, calendarID)

	_, err = db.Exec(`DELETE FROM blocker_events WHERE calendar_id=?`, calendarID)
	if err != nil {
		log.Fatalf("Error deleting blocker events: %v", err)
	}

	_, err = db.Exec(`DELETE FROM calendars WHERE calendar_id=?`, calendarID)
	if err != nil {
		log.Fatalf("Error deleting calendar ID: %v", err)
	}

	fmt.Printf("✅ Calendar %s deleted successfully\n", calendarID)
}
