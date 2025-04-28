package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"
)

type GoogleCalendarProvider struct {
	service *calendar.Service
	ctx     context.Context
}

func NewGoogleCalendarProvider(ctx context.Context, client *http.Client) (*GoogleCalendarProvider, error) {
	service, err := calendar.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, fmt.Errorf("failed to create calendar service: %w", err)
	}
	return &GoogleCalendarProvider{
		service: service,
		ctx:     ctx,
	}, nil
}

func (g *GoogleCalendarProvider) GetCalendar(calendarID string) error {
	_, err := g.service.CalendarList.Get(calendarID).Do()
	if err != nil {
		return fmt.Errorf("failed to get calendar: %w", err)
	}
	return nil
}

func (g *GoogleCalendarProvider) AddEvent(calendarID string, event *Event) (string, error) {
	googleEvent := &calendar.Event{
		Summary:     event.Summary,
		Description: event.Description,
		Start: &calendar.EventDateTime{
			DateTime: event.Start.Format(time.RFC3339),
		},
		End: &calendar.EventDateTime{
			DateTime: event.End.Format(time.RFC3339),
		},
	}

	createdEvent, err := g.service.Events.Insert(calendarID, googleEvent).Do()
	if err != nil {
		return "", fmt.Errorf("failed to create event: %w", err)
	}

	return createdEvent.Id, nil
}

func (g *GoogleCalendarProvider) UpdateEvent(calendarID string, eventID string, event *Event) error {
	googleEvent := &calendar.Event{
		Summary:     event.Summary,
		Description: event.Description,
		Start: &calendar.EventDateTime{
			DateTime: event.Start.Format(time.RFC3339),
		},
		End: &calendar.EventDateTime{
			DateTime: event.End.Format(time.RFC3339),
		},
	}

	_, err := g.service.Events.Update(calendarID, eventID, googleEvent).Do()
	if err != nil {
		return fmt.Errorf("failed to update event: %w", err)
	}

	return nil
}

func (g *GoogleCalendarProvider) DeleteEvent(calendarID string, eventID string) error {
	err := g.service.Events.Delete(calendarID, eventID).Do()
	if err != nil {
		return fmt.Errorf("failed to delete event: %w", err)
	}
	return nil
}

func (g *GoogleCalendarProvider) ListEvents(calendarID string, timeMin, timeMax time.Time) ([]*Event, error) {
	events, err := g.service.Events.List(calendarID).
		TimeMin(timeMin.Format(time.RFC3339)).
		TimeMax(timeMax.Format(time.RFC3339)).
		SingleEvents(true).
		OrderBy("startTime").
		Do()

	if err != nil {
		return nil, fmt.Errorf("failed to list events: %w", err)
	}

	var result []*Event
	for _, item := range events.Items {
		start, _ := time.Parse(time.RFC3339, item.Start.DateTime)
		end, _ := time.Parse(time.RFC3339, item.End.DateTime)

		result = append(result, &Event{
			ID:          item.Id,
			Summary:     item.Summary,
			Description: item.Description,
			Start:       start,
			End:         end,
			Status:      item.Status,
		})
	}

	return result, nil
}

func (g *GoogleCalendarProvider) GetEvent(calendarID string, eventID string) (*Event, error) {
	item, err := g.service.Events.Get(calendarID, eventID).Do()
	if err != nil {
		return nil, fmt.Errorf("failed to get event: %w", err)
	}

	start, _ := time.Parse(time.RFC3339, item.Start.DateTime)
	end, _ := time.Parse(time.RFC3339, item.End.DateTime)

	return &Event{
		ID:          item.Id,
		Summary:     item.Summary,
		Description: item.Description,
		Start:       start,
		End:         end,
		Status:      item.Status,
	}, nil
}