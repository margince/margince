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
	"github.com/margince/margince/backend/internal/modules/capture/googleconn"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

type schedulingAPI interface {
	List(context.Context, string) ([]connector.CalendarOption, error)
	Busy(context.Context, string, string, time.Time, time.Time) ([]connector.CalendarInterval, error)
	Inspect(context.Context, string, string, string) (connector.CalendarState, error)
	Lookup(context.Context, string, connector.CalendarAppointment) (*connector.CalendarReceipt, error)
	Save(context.Context, string, connector.CalendarAppointment) (connector.CalendarReceipt, error)
	Cancel(context.Context, string, string, string) error
}

// CalendarBusy reads occupancy using the connection’s current credential.
func (c *Connector) CalendarBusy(ctx context.Context, auth connector.Auth, calendar string, from, to time.Time) ([]connector.CalendarInterval, error) {
	_, token, err := googleconn.Session(ctx, c.oauth, auth)
	if err != nil {
		return nil, err
	}
	api, ok := c.api.(schedulingAPI)
	if !ok {
		return nil, connector.ErrUnreachable
	}
	return api.Busy(ctx, token, calendar, from, to)
}

// CalendarSave creates or updates one provider event using stable request identity.
func (c *Connector) CalendarSave(ctx context.Context, auth connector.Auth, in connector.CalendarAppointment) (connector.CalendarReceipt, error) {
	_, token, err := googleconn.Session(ctx, c.oauth, auth)
	if err != nil {
		return connector.CalendarReceipt{}, err
	}
	api, ok := c.api.(schedulingAPI)
	if !ok {
		return connector.CalendarReceipt{}, connector.ErrUnreachable
	}
	return api.Save(ctx, token, in)
}

// CalendarCancel removes the provider event and notifies attendees.
func (c *Connector) CalendarCancel(ctx context.Context, auth connector.Auth, calendar, event string) error {
	_, token, err := googleconn.Session(ctx, c.oauth, auth)
	if err != nil {
		return err
	}
	api, ok := c.api.(schedulingAPI)
	if !ok {
		return connector.ErrUnreachable
	}
	return api.Cancel(ctx, token, calendar, event)
}

type busyRequest struct {
	From  time.Time      `json:"timeMin"`
	To    time.Time      `json:"timeMax"`
	Items []calendarItem `json:"items"`
}
type calendarItem struct {
	ID string `json:"id"`
}
type busyCalendar struct {
	Busy   []connector.CalendarInterval `json:"busy"`
	Errors []struct {
		Reason string `json:"reason"`
	} `json:"errors"`
}

func (a *httpAPI) Busy(ctx context.Context, token, calendar string, from, to time.Time) ([]connector.CalendarInterval, error) {
	var result struct {
		Calendars map[string]busyCalendar `json:"calendars"`
	}
	_, err := calendarwire.Request(ctx, a.client, token, http.MethodPost, a.base+"/freeBusy", busyRequest{from, to, []calendarItem{{calendar}}}, &result)
	if err != nil {
		return nil, err
	}
	found, ok := result.Calendars[calendar]
	if !ok || len(found.Errors) != 0 {
		return nil, fmt.Errorf("calendar: selected calendar could not be checked")
	}
	// Read only occupancy metadata, so rescheduling can exclude its own event.
	return a.eventBusy(ctx, token, calendar, from, to)
}

type eventTime struct {
	At time.Time `json:"dateTime"`
}
type eventAttendee struct {
	Email string `json:"email"`
}
type scheduledEvent struct {
	ID          string          `json:"id,omitempty"`
	UID         string          `json:"iCalUID,omitempty"`
	URL         string          `json:"htmlLink,omitempty"`
	Status      string          `json:"status,omitempty"`
	Summary     string          `json:"summary"`
	Description string          `json:"description"`
	Location    string          `json:"location"`
	Start       eventTime       `json:"start"`
	End         eventTime       `json:"end"`
	Attendees   []eventAttendee `json:"attendees"`
}

func (a *httpAPI) Save(ctx context.Context, token string, in connector.CalendarAppointment) (connector.CalendarReceipt, error) {
	event := scheduledEvent{
		Summary: in.Subject, Description: in.Description, Location: in.Location,
		Start: eventTime{in.Start}, End: eventTime{in.End},
	}
	for _, email := range in.Attendees {
		event.Attendees = append(event.Attendees, eventAttendee{email})
	}
	path := a.base + "/calendars/" + url.PathEscape(in.CalendarID) + "/events"
	method := http.MethodPost
	if in.EventID != "" {
		path += "/" + url.PathEscape(in.EventID)
		method = http.MethodPatch
	} else {
		// A UUID's hexadecimal alphabet is a valid Google event id.
		event.ID = strings.ReplaceAll(in.RequestID, "-", "")
	}
	var result scheduledEvent
	status, err := calendarwire.Request(ctx, a.client, token, method, path+"?sendUpdates=all", event, &result)
	if status == http.StatusConflict && in.EventID == "" {
		existing, lookupErr := a.Lookup(ctx, token, in)
		if lookupErr != nil {
			return connector.CalendarReceipt{}, lookupErr
		}
		if existing == nil {
			return connector.CalendarReceipt{}, fmt.Errorf("calendar: conflicted invitation could not be reconciled")
		}
		return *existing, nil
	}
	if err != nil {
		return connector.CalendarReceipt{}, err
	}
	if result.ID == "" || result.Status == calendarCanceled {
		return connector.CalendarReceipt{}, fmt.Errorf("calendar: event was not confirmed")
	}
	return connector.CalendarReceipt{EventID: result.ID, UID: result.UID, URL: result.URL}, nil
}

func (a *httpAPI) Cancel(ctx context.Context, token, calendar, event string) error {
	status, err := calendarwire.Request(ctx, a.client, token, http.MethodDelete,
		a.base+"/calendars/"+url.PathEscape(calendar)+"/events/"+url.PathEscape(event)+"?sendUpdates=all", nil, nil)
	if status == http.StatusNotFound || status == http.StatusGone {
		return nil
	}
	return err
}

var _ connector.CalendarScheduler = (*Connector)(nil)

// CalendarLookup recovers an uncertain delivery without creating another event.
func (c *Connector) CalendarLookup(ctx context.Context, auth connector.Auth, in connector.CalendarAppointment) (*connector.CalendarReceipt, error) {
	_, token, err := googleconn.Session(ctx, c.oauth, auth)
	if err != nil {
		return nil, err
	}
	api, ok := c.api.(schedulingAPI)
	if !ok {
		return nil, connector.ErrUnreachable
	}
	return api.Lookup(ctx, token, in)
}

// CalendarList returns provider calendar identities and write permissions.
func (c *Connector) CalendarList(ctx context.Context, auth connector.Auth) ([]connector.CalendarOption, error) {
	_, token, err := googleconn.Session(ctx, c.oauth, auth)
	if err != nil {
		return nil, err
	}
	api, ok := c.api.(schedulingAPI)
	if !ok {
		return nil, connector.ErrUnreachable
	}
	return api.List(ctx, token)
}

// CalendarInspect reads the current interval and cancellation state of one event.
func (c *Connector) CalendarInspect(ctx context.Context, auth connector.Auth, calendar, event string) (connector.CalendarState, error) {
	_, token, err := googleconn.Session(ctx, c.oauth, auth)
	if err != nil {
		return connector.CalendarState{}, err
	}
	api, ok := c.api.(schedulingAPI)
	if !ok {
		return connector.CalendarState{}, connector.ErrUnreachable
	}
	return api.Inspect(ctx, token, calendar, event)
}
