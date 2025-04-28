package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/emersion/go-ical"
	"github.com/emersion/go-webdav"
	"github.com/emersion/go-webdav/caldav"
)

type CalDAVProvider struct {
	client    *caldav.Client
	ctx       context.Context
	serverURL string
}

func NewCalDAVProvider(ctx context.Context, serverURL, username, password string) (*CalDAVProvider, error) {
	baseURL, err := url.Parse(serverURL)
	if err != nil {
		return nil, fmt.Errorf("invalid CalDAV server URL: %w", err)
	}

	// Create HTTP client with authentication if needed
	var httpClient webdav.HTTPClient = http.DefaultClient
	if username != "" && password != "" {
		httpClient = webdav.HTTPClientWithBasicAuth(httpClient, username, password)
	}

	c, err := caldav.NewClient(httpClient, baseURL.String())
	if err != nil {
		return nil, fmt.Errorf("failed to create CalDAV client: %w", err)
	}

	// Test connection
	_, err = c.FindCalendars(ctx, "")  // Empty path means server root
	if err != nil {
		return nil, fmt.Errorf("failed to connect to CalDAV server: %w", err)
	}

	return &CalDAVProvider{
		client:    c,
		ctx:       ctx,
		serverURL: serverURL,
	}, nil
}

func (c *CalDAVProvider) GetCalendar(calendarID string) error {
	calURL, err := url.Parse(calendarID)
	if err != nil {
		return fmt.Errorf("invalid calendar URL: %w", err)
	}

	// Extract the calendar home set from the URL (usually the parent path)
	homeSetPath := "/"
	if calURL.Path != "" {
		parts := strings.Split(strings.TrimRight(calURL.Path, "/"), "/")
		if len(parts) > 1 {
			homeSetPath = "/" + strings.Join(parts[:len(parts)-1], "/")
		}
	}

	// Find all calendars in the home set
	calendars, err := c.client.FindCalendars(c.ctx, homeSetPath)
	if err != nil {
		return fmt.Errorf("failed to find calendars: %w", err)
	}

	// Check if our target calendar is among them
	for _, cal := range calendars {
		if cal.Path == calURL.Path {
			return nil // Found it!
		}
	}

	return fmt.Errorf("calendar not found at path: %s", calURL.Path)
}

func (c *CalDAVProvider) AddEvent(calendarID string, event *Event) (string, error) {
	calURL, err := url.Parse(calendarID)
	if err != nil {
		return "", fmt.Errorf("invalid calendar URL: %w", err)
	}
	
	// Create unique ID for the event
	eventUID := "gcalsync-" + time.Now().Format("20060102T150405Z")
	
	// Create iCal event
	icalEvent := ical.NewEvent()
	icalEvent.Component.Props.SetText("UID", eventUID)
	icalEvent.Component.Props.SetText("SUMMARY", event.Summary)
	icalEvent.Component.Props.SetText("DESCRIPTION", event.Description)
	icalEvent.Component.Props.SetDateTime("DTSTART", event.Start)
	icalEvent.Component.Props.SetDateTime("DTEND", event.End)
	icalEvent.Component.Props.SetText("STATUS", "CONFIRMED")
	
	// Create iCal calendar
	calendar := ical.NewCalendar()
	calendar.Component.Children = append(calendar.Component.Children, icalEvent.Component)

	// Create path for the new event
	path := calURL.Path + "/" + eventUID + ".ics"
	
	// Use PutCalendarObject to create the event
	_, err = c.client.PutCalendarObject(c.ctx, path, calendar)
	if err != nil {
		return "", fmt.Errorf("failed to create event: %w", err)
	}

	return eventUID, nil
}

func (c *CalDAVProvider) UpdateEvent(calendarID string, eventID string, event *Event) error {
	calURL, err := url.Parse(calendarID)
	if err != nil {
		return fmt.Errorf("invalid calendar URL: %w", err)
	}
	
	// Create updated iCal event
	icalEvent := ical.NewEvent()
	icalEvent.Component.Props.SetText("UID", eventID)
	icalEvent.Component.Props.SetText("SUMMARY", event.Summary)
	icalEvent.Component.Props.SetText("DESCRIPTION", event.Description)
	icalEvent.Component.Props.SetDateTime("DTSTART", event.Start)
	icalEvent.Component.Props.SetDateTime("DTEND", event.End)
	if event.Status != "" {
		icalEvent.Component.Props.SetText("STATUS", strings.ToUpper(event.Status))
	} else {
		icalEvent.Component.Props.SetText("STATUS", "CONFIRMED")
	}
	
	// Create iCal calendar
	calendar := ical.NewCalendar()
	calendar.Component.Children = append(calendar.Component.Children, icalEvent.Component)

	// Update CalDAV event using the same PutCalendarObject method
	// The eventID + .ics is the typical filename format for CalDAV events
	path := calURL.Path + "/" + eventID + ".ics"
	
	// Use PutCalendarObject to update the event (create or replace)
	_, err = c.client.PutCalendarObject(c.ctx, path, calendar)
	if err != nil {
		return fmt.Errorf("failed to update event: %w", err)
	}

	return nil
}

func (c *CalDAVProvider) DeleteEvent(calendarID string, eventID string) error {
	calURL, err := url.Parse(calendarID)
	if err != nil {
		return fmt.Errorf("invalid calendar URL: %w", err)
	}

	// Construct the path to the event file
	path := calURL.Path + "/" + eventID + ".ics"
	
	// Delete the event using RemoveAll method from webdav
	// Note: This uses the underlying webdav.Client's RemoveAll method
	// which is inherited by caldav.Client for deleting resources
	err = c.client.Client.RemoveAll(c.ctx, path)
	if err != nil {
		return fmt.Errorf("failed to delete event: %w", err)
	}

	return nil
}

func (c *CalDAVProvider) ListEvents(calendarID string, timeMin, timeMax time.Time) ([]*Event, error) {
	calURL, err := url.Parse(calendarID)
	if err != nil {
		return nil, fmt.Errorf("invalid calendar URL: %w", err)
	}

	// Setup a CalendarQuery to filter events by time range
	query := &caldav.CalendarQuery{
		CompFilter: caldav.CompFilter{
			Name: "VCALENDAR",
			Comps: []caldav.CompFilter{{
				Name:  "VEVENT",
				Start: timeMin,
				End:   timeMax,
			}},
		},
	}

	// Execute the query to get calendar objects in the time range
	objects, err := c.client.QueryCalendar(c.ctx, calURL.Path, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list events: %w", err)
	}

	// Process the results into our Event structure
	var result []*Event
	for _, obj := range objects {
		// The Data field in CalendarObject is already a *ical.Calendar, no need to parse
		calendar := obj.Data

		// Extract VEVENT components
		for _, comp := range calendar.Component.Children {
			if comp.Name != "VEVENT" {
				continue
			}

			// Extract event properties
			uid := getTextProp(comp.Props, "UID")
			summary := getTextProp(comp.Props, "SUMMARY")
			description := getTextProp(comp.Props, "DESCRIPTION")
			status := getTextProp(comp.Props, "STATUS")
			if status == "" {
				status = "confirmed" // Default status if not specified
			} else {
				// Convert from iCalendar status format (e.g., "CONFIRMED") to lowercase
				status = strings.ToLower(status)
			}

			// Parse dates
			start, _ := comp.Props.DateTime("DTSTART", time.UTC)
			end, _ := comp.Props.DateTime("DTEND", time.UTC)

			// Create Event object
			result = append(result, &Event{
				ID:          uid,
				Summary:     summary,
				Description: description,
				Start:       start,
				End:         end,
				Status:      status,
			})
		}
	}

	return result, nil
}

func (c *CalDAVProvider) GetEvent(calendarID string, eventID string) (*Event, error) {
	calURL, err := url.Parse(calendarID)
	if err != nil {
		return nil, fmt.Errorf("invalid calendar URL: %w", err)
	}

	// Construct the path to the event file
	path := calURL.Path + "/" + eventID + ".ics"

	// Get the calendar object
	object, err := c.client.GetCalendarObject(c.ctx, path)
	if err != nil {
		return nil, fmt.Errorf("failed to get event: %w", err)
	}

	calendar := object.Data

	// Find the VEVENT component
	var eventComp *ical.Component
	for _, comp := range calendar.Component.Children {
		if comp.Name == "VEVENT" {
			eventComp = comp
			break
		}
	}

	if eventComp == nil {
		return nil, fmt.Errorf("no VEVENT component found in calendar object")
	}

	// Extract event properties
	uid := getTextProp(eventComp.Props, "UID")
	summary := getTextProp(eventComp.Props, "SUMMARY")
	description := getTextProp(eventComp.Props, "DESCRIPTION")
	status := getTextProp(eventComp.Props, "STATUS")
	if status == "" {
		status = "confirmed" // Default status if not specified
	} else {
		// Convert from iCalendar status format (e.g., "CONFIRMED") to lowercase
		status = strings.ToLower(status)
	}

	// Parse dates
	start, _ := eventComp.Props.DateTime("DTSTART", time.UTC)
	end, _ := eventComp.Props.DateTime("DTEND", time.UTC)

	// Create Event object
	return &Event{
		ID:          uid,
		Summary:     summary,
		Description: description,
		Start:       start,
		End:         end,
		Status:      status,
	}, nil
}

// Helper function to get text property safely
func getTextProp(props ical.Props, name string) string {
	prop := props.Get(name)
	if prop == nil {
		return ""
	}
	return prop.Value
}