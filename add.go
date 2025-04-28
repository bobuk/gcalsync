package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"strings"
)

func addCalendar() {
	config, err := readConfig(".gcalsync.toml")
	if err != nil {
		log.Fatalf("Error reading config file: %v", err)
	}

	// Initialize the global oauthConfig
	initOAuthConfig(config)

	db, err := openDB(".gcalsync.db")
	if err != nil {
		log.Fatalf("Error opening database: %v", err)
	}
	defer db.Close()

	fmt.Println("🚀 Starting calendar addition...")
	fmt.Print("👤 Enter account name: ")
	var accountName string
	fmt.Scanln(&accountName)

	fmt.Print("🔄 Enter provider type (google or caldav): ")
	var providerType string
	fmt.Scanln(&providerType)
	providerType = strings.ToLower(providerType)

	fmt.Print("📅 Enter calendar ID or URL: ")
	reader := bufio.NewReader(os.Stdin)
	calendarID, _ := reader.ReadString('\n')
	calendarID = strings.TrimSpace(calendarID)

	ctx := context.Background()
	var providerConfig string
	
	// Use CalendarFactory to create and validate provider
	calendarFactory := NewCalendarFactory(ctx, config, db)

	// Validate calendar access based on provider type
	if providerType == "google" {
		provider, err := calendarFactory.CreateCalendarProvider(providerType, accountName, "")
		if err != nil {
			log.Fatalf("Error creating Google calendar provider: %v", err)
		}
		
		err = calendarFactory.ValidateCalendarAccess(provider, calendarID)
		if err != nil {
			log.Fatalf("Error retrieving Google calendar: %v", err)
		}
	} else if providerType == "caldav" {
		// Check if we have any CalDAV servers configured
		if len(config.CalDAVs) == 0 {
			log.Fatalf("Error: No CalDAV server configurations found in .gcalsync.toml")
		}

		// List available servers for selection
		fmt.Println("Available CalDAV servers:")
		servers := make([]string, 0, len(config.CalDAVs))
		
		// List all configured servers
		i := 0
		for name, server := range config.CalDAVs {
			displayName := name
			if server.Name != "" {
				displayName = server.Name
			}
			fmt.Printf("  %d: %s (%s)\n", i, displayName, server.ServerURL)
			servers = append(servers, name)
			i++
		}
		
		fmt.Print("Enter server number: ")
		var serverIndex int
		fmt.Scanln(&serverIndex)
		
		if serverIndex < 0 || serverIndex >= len(servers) {
			log.Fatalf("Error: Invalid server selection")
		}
		
		serverName := servers[serverIndex]
		serverConfig := config.CalDAVs[serverName]
		
		fmt.Printf("Using CalDAV server: %s\n", serverConfig.ServerURL)
		
		// Create and validate provider using the factory
		provider, err := calendarFactory.CreateCalendarProvider(providerType, accountName, serverName)
		if err != nil {
			log.Fatalf("Error creating CalDAV provider: %v", err)
		}

		err = calendarFactory.ValidateCalendarAccess(provider, calendarID)
		if err != nil {
			log.Fatalf("Error retrieving CalDAV calendar: %v", err)
		}
		
		// Store the server name in the provider_config field
		providerConfig = serverName
	} else {
		log.Fatalf("Error: Unsupported provider type: %s (must be 'google' or 'caldav')", providerType)
	}

	// Update schema to include provider_config field if not exists
	_, err = db.Exec(`ALTER TABLE calendars ADD COLUMN provider_config TEXT DEFAULT ''`)
	if err != nil && !strings.Contains(err.Error(), "duplicate column name") {
		log.Printf("Warning: Failed to add provider_config column: %v", err)
	}

	_, err = db.Exec(`INSERT INTO calendars (account_name, calendar_id, provider_type, provider_config) VALUES (?, ?, ?, ?)`, 
		accountName, calendarID, providerType, providerConfig)
	if err != nil {
		log.Fatalf("Error saving calendar ID: %v", err)
	}

	fmt.Printf("✅ %s Calendar %s added successfully for account %s\n", 
		strings.ToUpper(providerType), calendarID, accountName)
}