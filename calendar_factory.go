package main

import (
	"context"
	"database/sql"
	"fmt"
)

// CalendarFactory handles creation and management of calendar providers
type CalendarFactory struct {
	config *Config
	db     *sql.DB
	ctx    context.Context
}

// NewCalendarFactory creates a new calendar factory instance
func NewCalendarFactory(ctx context.Context, config *Config, db *sql.DB) *CalendarFactory {
	return &CalendarFactory{
		config: config,
		db:     db,
		ctx:    ctx,
	}
}

// GetAllCalendars returns all calendar providers for all accounts
func (cf *CalendarFactory) GetAllCalendars() (map[string]map[string]CalendarProvider, map[string][]CalendarInfo, error) {
	calendars := getCalendarsFromDB(cf.db)
	providers := make(map[string]map[string]CalendarProvider)

	for accountName, calendarInfos := range calendars {
		providers[accountName] = make(map[string]CalendarProvider)

		// Initialize providers for each type needed by this account
		for i, calInfo := range calendarInfos {
			switch calInfo.ProviderType {
			case "google":
				if _, exists := providers[accountName]["google"]; !exists {
					client := getClient(cf.ctx, oauthConfig, cf.db, accountName, cf.config)
					googleProvider, err := NewGoogleCalendarProvider(cf.ctx, client)
					if err != nil {
						return nil, nil, fmt.Errorf("error creating Google calendar provider: %w", err)
					}
					providers[accountName]["google"] = googleProvider
				}

			case "caldav":
				// Get the server configuration based on provider_config
				var serverConfig CalDAVConfig
				serverName := calInfo.ProviderConfig

				// If there's no server name stored, we need to ask the user to reconfigure this calendar
				if serverName == "" || serverName == "default" {
					return nil, nil, fmt.Errorf("calendar references removed legacy CalDAV configuration; please remove and re-add this calendar")
				}

				// Use the server from the CalDAV servers config
				if server, ok := cf.config.CalDAVs[serverName]; ok {
					serverConfig = server
				} else {
					return nil, nil, fmt.Errorf("CalDAV server '%s' not found in configuration", serverName)
				}

				// Create a provider key that includes the server name to allow multiple servers
				providerKey := "caldav-" + serverName

				// Only create the provider if we don't already have one for this server
				if _, exists := providers[accountName][providerKey]; !exists {
					caldavProvider, err := NewCalDAVProvider(cf.ctx, serverConfig.ServerURL, serverConfig.Username, serverConfig.Password)
					if err != nil {
						return nil, nil, fmt.Errorf("error connecting to CalDAV server %s: %w", serverName, err)
					}
					providers[accountName][providerKey] = caldavProvider
				}

				// Update the calendar info to use the correct provider key
				calendarInfos[i].ProviderKey = providerKey

			default:
				return nil, nil, fmt.Errorf("unsupported provider type: %s", calInfo.ProviderType)
			}
		}
	}

	return providers, calendars, nil
}

// CreateCalendarProvider creates a specific calendar provider
func (cf *CalendarFactory) CreateCalendarProvider(providerType string, accountName string, serverName string) (CalendarProvider, error) {
	switch providerType {
	case "google":
		client := getClient(cf.ctx, oauthConfig, cf.db, accountName, cf.config)
		return NewGoogleCalendarProvider(cf.ctx, client)

	case "caldav":
		// Get the server configuration
		if serverName == "" || serverName == "default" {
			return nil, fmt.Errorf("no server name provided for CalDAV provider")
		}

		// Use the server from the CalDAV servers config
		var serverConfig CalDAVConfig
		if server, ok := cf.config.CalDAVs[serverName]; ok {
			serverConfig = server
		} else {
			return nil, fmt.Errorf("CalDAV server '%s' not found in configuration", serverName)
		}

		return NewCalDAVProvider(cf.ctx, serverConfig.ServerURL, serverConfig.Username, serverConfig.Password)

	default:
		return nil, fmt.Errorf("unsupported provider type: %s", providerType)
	}
}

// ValidateCalendarAccess checks if the provided calendar ID is accessible
func (cf *CalendarFactory) ValidateCalendarAccess(provider CalendarProvider, calendarID string) error {
	return provider.GetCalendar(calendarID)
}