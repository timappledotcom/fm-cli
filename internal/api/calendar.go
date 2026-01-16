package api

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"fm-cli/internal/model"

	"git.sr.ht/~rockorager/go-jmap"
)

// JMAP Calendar capability URI
const CalendarURI jmap.URI = "urn:ietf:params:jmap:calendars"

// Calendar JMAP types for raw requests
type calendarGetRequest struct {
	AccountID string `json:"accountId"`
}

type calendarEventGetRequest struct {
	AccountID  string   `json:"accountId"`
	IDs        []string `json:"ids,omitempty"`
	Properties []string `json:"properties,omitempty"`
}

type calendarEventQueryRequest struct {
	AccountID string                      `json:"accountId"`
	Filter    *calendarEventFilterCondition `json:"filter,omitempty"`
	Sort      []calendarEventSort         `json:"sort,omitempty"`
	Position  int                         `json:"position,omitempty"`
	Limit     int                         `json:"limit,omitempty"`
}

type calendarEventFilterCondition struct {
	InCalendars []string `json:"inCalendars,omitempty"`
	After       string   `json:"after,omitempty"`
	Before      string   `json:"before,omitempty"`
	Text        string   `json:"text,omitempty"`
}

type calendarEventSort struct {
	Property    string `json:"property"`
	IsAscending bool   `json:"isAscending"`
}

type calendarEventSetRequest struct {
	AccountID string                       `json:"accountId"`
	Create    map[string]calendarEventData `json:"create,omitempty"`
	Update    map[string]calendarEventData `json:"update,omitempty"`
	Destroy   []string                     `json:"destroy,omitempty"`
}

type calendarEventData struct {
	CalendarIDs map[string]bool `json:"calendarIds,omitempty"`
	Type        string          `json:"@type,omitempty"`
	Title       string          `json:"title,omitempty"`
	Description string          `json:"description,omitempty"`
	Location    string          `json:"location,omitempty"`
	Start       string          `json:"start,omitempty"`
	Duration    string          `json:"duration,omitempty"`
	TimeZone    string          `json:"timeZone,omitempty"`
	ShowWithoutTime bool        `json:"showWithoutTime,omitempty"`
	Status      string          `json:"status,omitempty"`
	Alerts      map[string]alertData `json:"alerts,omitempty"`
}

type alertData struct {
	Type    string     `json:"@type"`
	Trigger triggerData `json:"trigger"`
	Action  string     `json:"action"`
}

type triggerData struct {
	Type   string `json:"@type"`
	Offset string `json:"offset"`
}

func (c *Client) getCalendarAccountID() jmap.ID {
	if c.Session == nil {
		return ""
	}
	// Check if calendar capability exists
	if id, ok := c.Session.PrimaryAccounts[CalendarURI]; ok {
		return id
	}
	// Fallback to mail account (Fastmail uses same account for both)
	return c.Session.PrimaryAccounts["urn:ietf:params:jmap:mail"]
}

// FetchCalendars retrieves all calendars
func (c *Client) FetchCalendars() ([]model.Calendar, error) {
	accountID := c.getCalendarAccountID()
	if accountID == "" {
		return nil, fmt.Errorf("no calendar account found")
	}

	req := &jmap.Request{
		Using: []jmap.URI{jmap.CoreURI, CalendarURI},
	}

	reqData := calendarGetRequest{
		AccountID: string(accountID),
	}

	req.Calls = append(req.Calls, &jmap.Invocation{
		Name:   "Calendar/get",
		CallID: "c0",
		Args:   reqData,
	})

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Calendar/get failed: %w", err)
	}

	var calendars []model.Calendar
	for _, inv := range resp.Responses {
		if inv.Name == "error" {
			return nil, fmt.Errorf("JMAP error: %v", inv.Args)
		}
		if inv.Name == "Calendar/get" {
			data, _ := json.Marshal(inv.Args)
			var result struct {
				List []struct {
					ID               string `json:"id"`
					Name             string `json:"name"`
					Color            string `json:"color"`
					IsVisible        bool   `json:"isVisible"`
					IsDefault        bool   `json:"isDefault"`
					MayReadItems     bool   `json:"mayReadItems"`
					MayAddItems      bool   `json:"mayAddItems"`
					MayModifyItems   bool   `json:"mayModifyItems"`
					MayRemoveItems   bool   `json:"mayRemoveItems"`
				} `json:"list"`
			}
			if err := json.Unmarshal(data, &result); err != nil {
				continue
			}
			for _, cal := range result.List {
				calendars = append(calendars, model.Calendar{
					ID:             cal.ID,
					Name:           cal.Name,
					Color:          cal.Color,
					IsVisible:      cal.IsVisible,
					IsDefault:      cal.IsDefault,
					MayReadItems:   cal.MayReadItems,
					MayAddItems:    cal.MayAddItems,
					MayModifyItems: cal.MayModifyItems,
					MayRemoveItems: cal.MayRemoveItems,
				})
			}
		}
	}

	return calendars, nil
}

// FetchEvents retrieves calendar events within a date range
func (c *Client) FetchEvents(calendarIDs []string, start, end time.Time) ([]model.CalendarEvent, error) {
	accountID := c.getCalendarAccountID()
	if accountID == "" {
		return nil, fmt.Errorf("no calendar account found")
	}

	req := &jmap.Request{
		Using: []jmap.URI{jmap.CoreURI, CalendarURI},
	}

	// Query for events in date range
	queryReq := calendarEventQueryRequest{
		AccountID: string(accountID),
		Filter: &calendarEventFilterCondition{
			InCalendars: calendarIDs,
			After:       start.Format(time.RFC3339),
			Before:      end.Format(time.RFC3339),
		},
		Sort: []calendarEventSort{
			{Property: "start", IsAscending: true},
		},
		Limit: 100,
	}

	req.Calls = append(req.Calls, &jmap.Invocation{
		Name:   "CalendarEvent/query",
		CallID: "q0",
		Args:   queryReq,
	})

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("CalendarEvent/query failed: %w", err)
	}

	// Get event IDs from query response
	var eventIDs []string
	for _, inv := range resp.Responses {
		if inv.Name == "error" {
			return nil, fmt.Errorf("JMAP error: %v", inv.Args)
		}
		if inv.Name == "CalendarEvent/query" {
			data, _ := json.Marshal(inv.Args)
			var result struct {
				IDs []string `json:"ids"`
			}
			json.Unmarshal(data, &result)
			eventIDs = result.IDs
		}
	}

	if len(eventIDs) == 0 {
		return []model.CalendarEvent{}, nil
	}

	// Fetch event details
	req2 := &jmap.Request{
		Using: []jmap.URI{jmap.CoreURI, CalendarURI},
	}

	getReq := calendarEventGetRequest{
		AccountID: string(accountID),
		IDs:       eventIDs,
		Properties: []string{
			"id", "calendarIds", "title", "description", "location",
			"start", "duration", "timeZone", "showWithoutTime", "status",
			"recurrenceRules", "alerts", "participants", "created", "updated",
		},
	}

	req2.Calls = append(req2.Calls, &jmap.Invocation{
		Name:   "CalendarEvent/get",
		CallID: "g0",
		Args:   getReq,
	})

	resp2, err := c.Client.Do(req2)
	if err != nil {
		return nil, fmt.Errorf("CalendarEvent/get failed: %w", err)
	}

	var events []model.CalendarEvent
	for _, inv := range resp2.Responses {
		if inv.Name == "CalendarEvent/get" {
			data, _ := json.Marshal(inv.Args)
			var result struct {
				List []struct {
					ID          string          `json:"id"`
					CalendarIDs map[string]bool `json:"calendarIds"`
					Title       string          `json:"title"`
					Description string          `json:"description"`
					Location    string          `json:"location"`
					Start       string          `json:"start"`
					Duration    string          `json:"duration"`
					TimeZone    string          `json:"timeZone"`
					ShowWithoutTime bool        `json:"showWithoutTime"`
					Status      string          `json:"status"`
					Created     string          `json:"created"`
					Updated     string          `json:"updated"`
					Alerts      map[string]struct {
						Trigger struct {
							Offset string `json:"offset"`
						} `json:"trigger"`
						Action string `json:"action"`
					} `json:"alerts"`
					Participants map[string]struct {
						Name   string `json:"name"`
						Email  string `json:"email"`
						Kind   string `json:"kind"`
						Roles  map[string]bool `json:"roles"`
						ParticipationStatus string `json:"participationStatus"`
					} `json:"participants"`
				} `json:"list"`
			}
			if err := json.Unmarshal(data, &result); err != nil {
				continue
			}

			for _, e := range result.List {
				event := model.CalendarEvent{
					ID:          e.ID,
					Title:       e.Title,
					Description: e.Description,
					Location:    e.Location,
					Duration:    e.Duration,
					ShowWithoutTime: e.ShowWithoutTime,
					IsAllDay:    e.ShowWithoutTime,
					Status:      e.Status,
				}

				// Get first calendar ID
				for calID := range e.CalendarIDs {
					event.CalendarID = calID
					break
				}

				// Parse start time
				if e.Start != "" {
					if t, err := parseJSCalendarTime(e.Start, e.TimeZone); err == nil {
						event.Start = t
						// Calculate end time from duration
						if dur, err := parseDuration(e.Duration); err == nil {
							event.End = t.Add(dur)
						}
					}
				}

				// Parse created/updated
				if e.Created != "" {
					event.Created, _ = time.Parse(time.RFC3339, e.Created)
				}
				if e.Updated != "" {
					event.Updated, _ = time.Parse(time.RFC3339, e.Updated)
				}

				// Convert alerts
				for id, a := range e.Alerts {
					event.Alerts = append(event.Alerts, model.EventAlert{
						ID:      id,
						Trigger: a.Trigger.Offset,
						Action:  a.Action,
					})
				}

				// Convert participants
				for _, p := range e.Participants {
					role := "attendee"
					if p.Roles["owner"] {
						role = "owner"
					} else if p.Roles["optional"] {
						role = "optional"
					}
					event.Participants = append(event.Participants, model.EventParticipant{
						Name:   p.Name,
						Email:  p.Email,
						Kind:   p.Kind,
						Role:   role,
						Status: p.ParticipationStatus,
					})
				}

				events = append(events, event)
			}
		}
	}

	// Sort events by start time
	sort.Slice(events, func(i, j int) bool {
		return events[i].Start.Before(events[j].Start)
	})

	return events, nil
}

// CreateEvent creates a new calendar event
func (c *Client) CreateEvent(event model.CalendarEvent) (string, error) {
	accountID := c.getCalendarAccountID()
	if accountID == "" {
		return "", fmt.Errorf("no calendar account found")
	}

	req := &jmap.Request{
		Using: []jmap.URI{jmap.CoreURI, CalendarURI},
	}

	eventData := calendarEventData{
		CalendarIDs: map[string]bool{event.CalendarID: true},
		Type:        "Event",
		Title:       event.Title,
		Description: event.Description,
		Location:    event.Location,
		Start:       event.Start.Format("2006-01-02T15:04:05"),
		TimeZone:    "UTC",
		Status:      "confirmed",
	}

	if event.IsAllDay {
		eventData.ShowWithoutTime = true
		eventData.Start = event.Start.Format("2006-01-02")
		eventData.Duration = "P1D"
	} else {
		if event.Duration != "" {
			eventData.Duration = event.Duration
		} else if !event.End.IsZero() {
			eventData.Duration = formatDuration(event.End.Sub(event.Start))
		} else {
			eventData.Duration = "PT1H" // Default 1 hour
		}
	}

	setReq := calendarEventSetRequest{
		AccountID: string(accountID),
		Create: map[string]calendarEventData{
			"new-event": eventData,
		},
	}

	req.Calls = append(req.Calls, &jmap.Invocation{
		Name:   "CalendarEvent/set",
		CallID: "s0",
		Args:   setReq,
	})

	resp, err := c.Client.Do(req)
	if err != nil {
		return "", fmt.Errorf("CalendarEvent/set failed: %w", err)
	}

	for _, inv := range resp.Responses {
		if inv.Name == "error" {
			return "", fmt.Errorf("JMAP error: %v", inv.Args)
		}
		if inv.Name == "CalendarEvent/set" {
			data, _ := json.Marshal(inv.Args)
			var result struct {
				Created map[string]struct {
					ID string `json:"id"`
				} `json:"created"`
				NotCreated map[string]struct {
					Type        string `json:"type"`
					Description string `json:"description"`
				} `json:"notCreated"`
			}
			json.Unmarshal(data, &result)

			if len(result.NotCreated) > 0 {
				for _, err := range result.NotCreated {
					return "", fmt.Errorf("failed to create event: %s - %s", err.Type, err.Description)
				}
			}
			if created, ok := result.Created["new-event"]; ok {
				return created.ID, nil
			}
		}
	}

	return "", fmt.Errorf("no event ID returned")
}

// UpdateEvent updates an existing calendar event
func (c *Client) UpdateEvent(event model.CalendarEvent) error {
	accountID := c.getCalendarAccountID()
	if accountID == "" {
		return fmt.Errorf("no calendar account found")
	}

	req := &jmap.Request{
		Using: []jmap.URI{jmap.CoreURI, CalendarURI},
	}

	eventData := calendarEventData{
		Title:       event.Title,
		Description: event.Description,
		Location:    event.Location,
		Start:       event.Start.Format("2006-01-02T15:04:05"),
		TimeZone:    "UTC",
	}

	if event.IsAllDay {
		eventData.ShowWithoutTime = true
		eventData.Start = event.Start.Format("2006-01-02")
		eventData.Duration = "P1D"
	} else {
		if event.Duration != "" {
			eventData.Duration = event.Duration
		} else if !event.End.IsZero() {
			eventData.Duration = formatDuration(event.End.Sub(event.Start))
		}
	}

	setReq := calendarEventSetRequest{
		AccountID: string(accountID),
		Update: map[string]calendarEventData{
			event.ID: eventData,
		},
	}

	req.Calls = append(req.Calls, &jmap.Invocation{
		Name:   "CalendarEvent/set",
		CallID: "s0",
		Args:   setReq,
	})

	resp, err := c.Client.Do(req)
	if err != nil {
		return fmt.Errorf("CalendarEvent/set failed: %w", err)
	}

	for _, inv := range resp.Responses {
		if inv.Name == "error" {
			return fmt.Errorf("JMAP error: %v", inv.Args)
		}
		if inv.Name == "CalendarEvent/set" {
			data, _ := json.Marshal(inv.Args)
			var result struct {
				NotUpdated map[string]struct {
					Type        string `json:"type"`
					Description string `json:"description"`
				} `json:"notUpdated"`
			}
			json.Unmarshal(data, &result)

			if len(result.NotUpdated) > 0 {
				for _, err := range result.NotUpdated {
					return fmt.Errorf("failed to update event: %s - %s", err.Type, err.Description)
				}
			}
		}
	}

	return nil
}

// DeleteEvent deletes a calendar event
func (c *Client) DeleteEvent(eventID string) error {
	accountID := c.getCalendarAccountID()
	if accountID == "" {
		return fmt.Errorf("no calendar account found")
	}

	req := &jmap.Request{
		Using: []jmap.URI{jmap.CoreURI, CalendarURI},
	}

	setReq := calendarEventSetRequest{
		AccountID: string(accountID),
		Destroy:   []string{eventID},
	}

	req.Calls = append(req.Calls, &jmap.Invocation{
		Name:   "CalendarEvent/set",
		CallID: "s0",
		Args:   setReq,
	})

	resp, err := c.Client.Do(req)
	if err != nil {
		return fmt.Errorf("CalendarEvent/set failed: %w", err)
	}

	for _, inv := range resp.Responses {
		if inv.Name == "error" {
			return fmt.Errorf("JMAP error: %v", inv.Args)
		}
		if inv.Name == "CalendarEvent/set" {
			data, _ := json.Marshal(inv.Args)
			var result struct {
				NotDestroyed map[string]struct {
					Type        string `json:"type"`
					Description string `json:"description"`
				} `json:"notDestroyed"`
			}
			json.Unmarshal(data, &result)

			if len(result.NotDestroyed) > 0 {
				for _, err := range result.NotDestroyed {
					return fmt.Errorf("failed to delete event: %s - %s", err.Type, err.Description)
				}
			}
		}
	}

	return nil
}

// Helper functions

func parseJSCalendarTime(s, tz string) (time.Time, error) {
	// Try various formats
	formats := []string{
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02",
	}
	
	var loc *time.Location
	if tz != "" {
		loc, _ = time.LoadLocation(tz)
	}
	if loc == nil {
		loc = time.UTC
	}

	for _, format := range formats {
		if t, err := time.ParseInLocation(format, s, loc); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unable to parse time: %s", s)
}

func parseDuration(s string) (time.Duration, error) {
	if s == "" {
		return time.Hour, nil // Default 1 hour
	}

	// Handle ISO 8601 duration format
	s = strings.TrimPrefix(s, "P")
	
	var d time.Duration
	
	// Check for days
	if idx := strings.Index(s, "D"); idx != -1 {
		var days int
		fmt.Sscanf(s[:idx], "%d", &days)
		d += time.Duration(days) * 24 * time.Hour
		s = s[idx+1:]
	}
	
	// Check for time portion
	s = strings.TrimPrefix(s, "T")
	
	if idx := strings.Index(s, "H"); idx != -1 {
		var hours int
		fmt.Sscanf(s[:idx], "%d", &hours)
		d += time.Duration(hours) * time.Hour
		s = s[idx+1:]
	}
	
	if idx := strings.Index(s, "M"); idx != -1 {
		var mins int
		fmt.Sscanf(s[:idx], "%d", &mins)
		d += time.Duration(mins) * time.Minute
		s = s[idx+1:]
	}
	
	if idx := strings.Index(s, "S"); idx != -1 {
		var secs int
		fmt.Sscanf(s[:idx], "%d", &secs)
		d += time.Duration(secs) * time.Second
	}

	if d == 0 {
		return time.Hour, nil
	}
	return d, nil
}

func formatDuration(d time.Duration) string {
	hours := int(d.Hours())
	mins := int(d.Minutes()) % 60
	
	if hours >= 24 && mins == 0 && hours%24 == 0 {
		return fmt.Sprintf("P%dD", hours/24)
	}
	
	result := "PT"
	if hours > 0 {
		result += fmt.Sprintf("%dH", hours)
	}
	if mins > 0 {
		result += fmt.Sprintf("%dM", mins)
	}
	if result == "PT" {
		return "PT1H"
	}
	return result
}
