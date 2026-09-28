// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//nolint:tagliatelle // Calendar provider field names are an external wire contract.
package gcal

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/modules/capture/calendarwire"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

//nolint:nilnil // A nil receipt means the provider has no matching delivery.
func (a *httpAPI) Lookup(ctx context.Context, token string, in connector.CalendarAppointment) (*connector.CalendarReceipt, error) {
	id := in.EventID
	if id == "" {
		id = strings.ReplaceAll(in.RequestID, "-", "")
	}
	var event scheduledEvent
	status, err := calendarwire.Request(ctx, a.client, token, http.MethodGet, a.base+"/calendars/"+url.PathEscape(in.CalendarID)+"/events/"+url.PathEscape(id), nil, &event)
	if status == http.StatusNotFound && in.EventID == "" {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if event.ID != id || (!in.Cancel && event.Status == calendarCanceled) {
		return nil, fmt.Errorf("calendar: event identity is unavailable")
	}
	if !in.Cancel && (!event.Start.At.Equal(in.Start) || !event.End.At.Equal(in.End)) {
		if in.EventID != "" {
			return nil, nil
		}
		return nil, fmt.Errorf("calendar: existing invitation has changed; reconcile before retrying")
	}
	return &connector.CalendarReceipt{EventID: event.ID, UID: event.UID, URL: event.URL}, nil
}

type occupancyTime struct {
	At   time.Time `json:"dateTime"`
	Date string    `json:"date"`
}
type occupancyEvent struct {
	ID           string        `json:"id"`
	Status       string        `json:"status"`
	Transparency string        `json:"transparency"`
	Start        occupancyTime `json:"start"`
	End          occupancyTime `json:"end"`
	Attendees    []struct {
		Self     bool   `json:"self"`
		Response string `json:"responseStatus"`
	} `json:"attendees"`
}

func occupancyInstant(value occupancyTime, zone *time.Location) (time.Time, error) {
	if !value.At.IsZero() {
		return value.At, nil
	}
	return time.ParseInLocation("2006-01-02", value.Date, zone)
}

func (a *httpAPI) eventBusy(ctx context.Context, token, calendar string, from, to time.Time) ([]connector.CalendarInterval, error) {
	query := url.Values{"timeMin": {from.Format(time.RFC3339)}, "timeMax": {to.Format(time.RFC3339)}, calendarSingleEvents: {"true"}, calendarMaxResults: {"2500"}, "fields": {"timeZone,nextPageToken,items(id,status,transparency,start,end,attendees(self,responseStatus))"}}
	busy := []connector.CalendarInterval{}
	for page := 0; page < 100; page++ {
		var result struct {
			Zone  string           `json:"timeZone"`
			Next  string           `json:"nextPageToken"`
			Items []occupancyEvent `json:"items"`
		}
		if _, err := calendarwire.Request(ctx, a.client, token, http.MethodGet, a.base+"/calendars/"+url.PathEscape(calendar)+"/events?"+query.Encode(), nil, &result); err != nil {
			return nil, err
		}
		zone, err := time.LoadLocation(result.Zone)
		if err != nil {
			return nil, err
		}
		intervals, err := occupancyIntervals(result.Items, zone)
		if err != nil {
			return nil, err
		}
		busy = append(busy, intervals...)
		if result.Next == "" {
			return busy, nil
		}
		query.Set("pageToken", result.Next)
	}
	return nil, fmt.Errorf("calendar: incomplete availability")
}

func (a *httpAPI) Inspect(ctx context.Context, token, calendar, event string) (connector.CalendarState, error) {
	var out scheduledEvent
	status, err := calendarwire.Request(ctx, a.client, token, http.MethodGet, a.base+"/calendars/"+url.PathEscape(calendar)+"/events/"+url.PathEscape(event), nil, &out)
	if status == http.StatusNotFound || status == http.StatusGone {
		return connector.CalendarState{Canceled: true}, nil
	}
	if err != nil {
		return connector.CalendarState{}, err
	}
	if out.ID != event {
		return connector.CalendarState{}, fmt.Errorf("calendar: mismatched event identity")
	}
	return connector.CalendarState{Start: out.Start.At, End: out.End.At, Canceled: out.Status == calendarCanceled}, nil
}

func occupancyIntervals(events []occupancyEvent, zone *time.Location) ([]connector.CalendarInterval, error) {
	busy := []connector.CalendarInterval{}
	for _, event := range events {
		if event.Status == calendarCanceled || event.Transparency == "transparent" {
			continue
		}
		declined := false
		for _, attendee := range event.Attendees {
			if attendee.Self && attendee.Response == "declined" {
				declined = true
			}
		}
		if declined {
			continue
		}
		start, err := occupancyInstant(event.Start, zone)
		if err != nil {
			return nil, err
		}
		end, err := occupancyInstant(event.End, zone)
		if err != nil {
			return nil, err
		}
		if !end.After(start) {
			return nil, fmt.Errorf("calendar: invalid busy interval")
		}
		busy = append(busy, connector.CalendarInterval{EventID: event.ID, Start: start, End: end})
	}
	return busy, nil
}
