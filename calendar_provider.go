package main

import (
	"time"
)

type CalendarProvider interface {
	GetCalendar(calendarID string) error
	AddEvent(calendarID string, event *Event) (string, error)
	UpdateEvent(calendarID string, eventID string, event *Event) error
	DeleteEvent(calendarID string, eventID string) error
	ListEvents(calendarID string, timeMin, timeMax time.Time) ([]*Event, error)
}

type Event struct {
	ID          string
	Summary     string
	Description string
	Start       time.Time
	End         time.Time
	Status      string
}
