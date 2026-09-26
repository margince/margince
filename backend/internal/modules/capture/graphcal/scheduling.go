// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//nolint:tagliatelle // Calendar provider field names are an external wire contract.
package graphcal

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/modules/capture/calendarwire"
	"github.com/margince/margince/backend/internal/modules/capture/graphconn"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

const calendarUTC = "UTC"

type schedulingAPI interface {
	List(context.Context, string) ([]connector.CalendarOption, error)
	Busy(context.Context, string, string, time.Time, time.Time) ([]connector.CalendarInterval, error)
	Inspect(context.Context, string, string, string) (connector.CalendarState, error)
	Lookup(context.Context, string, connector.CalendarAppointment) (*connector.CalendarReceipt, error)
	Save(context.Context, string, connector.CalendarAppointment) (connector.CalendarReceipt, error)
	Cancel(context.Context, string, string, string) error
}

func (c *Connector) calendarAccess(ctx context.Context, auth connector.Auth) (string, error) {
	st, err := readAuth(auth)
	if err != nil {
		return "", err
	}
	refreshed, err := c.oauth.Refresh(ctx, st.RefreshToken, st.Granted)
	if err != nil {
		return "", err
	}
	graphconn.ReportRotation(ctx, connectorName, c.rotations, st, refreshed.Rotated)
	return refreshed.AccessToken, nil
}

// CalendarBusy reads current occupancy using the connection’s credential.
func (c *Connector) CalendarBusy(ctx context.Context, auth connector.Auth, calendar string, from, to time.Time) ([]connector.CalendarInterval, error) {
	token, err := c.calendarAccess(ctx, auth)
	if err != nil {
		return nil, err
	}
	api, ok := c.api.(schedulingAPI)
	if !ok {
		return nil, connector.ErrUnreachable
	}
	return api.Busy(ctx, token, calendar, from, to)
}

// CalendarSave sends one stable calendar intent through the provider.
func (c *Connector) CalendarSave(ctx context.Context, auth connector.Auth, in connector.CalendarAppointment) (connector.CalendarReceipt, error) {
	token, err := c.calendarAccess(ctx, auth)
	if err != nil {
		return connector.CalendarReceipt{}, err
	}
	api, ok := c.api.(schedulingAPI)
	if !ok {
		return connector.CalendarReceipt{}, connector.ErrUnreachable
	}
	return api.Save(ctx, token, in)
}

// CalendarCancel cancels the event through its immutable provider identity.
func (c *Connector) CalendarCancel(ctx context.Context, auth connector.Auth, calendar, event string) error {
	token, err := c.calendarAccess(ctx, auth)
	if err != nil {
		return err
	}
	api, ok := c.api.(schedulingAPI)
	if !ok {
		return connector.ErrUnreachable
	}
	return api.Cancel(ctx, token, calendar, event)
}

type scheduledTime struct {
	At   string `json:"dateTime"`
	Zone string `json:"timeZone"`
}
type scheduledAttendee struct {
	Address struct {
		Address string `json:"address"`
	} `json:"emailAddress"`
	Type string `json:"type"`
}
type scheduledEvent struct {
	ID         string            `json:"id,omitempty"`
	UID        string            `json:"iCalUId,omitempty"`
	URL        string            `json:"webLink,omitempty"`
	RequestID  string            `json:"transactionId,omitempty"`
	Properties []requestProperty `json:"singleValueExtendedProperties,omitempty"`
	Subject    string            `json:"subject"`
	Body       struct {
		Type    string `json:"contentType"`
		Content string `json:"content"`
	} `json:"body"`
	Location struct {
		Name string `json:"displayName"`
	} `json:"location"`
	Start     scheduledTime       `json:"start"`
	End       scheduledTime       `json:"end"`
	Attendees []scheduledAttendee `json:"attendees"`
	AllDay    bool                `json:"isAllDay,omitempty"`
	Canceled  bool                `json:"isCancelled,omitempty"`
	ShowAs    string              `json:"showAs,omitempty"`
	Response  struct {
		Response string `json:"response"`
	} `json:"responseStatus,omitempty"`
}

func calendarPath(calendar string) string {
	if calendar == "primary" {
		return "/me/calendar"
	}
	return "/me/calendars/" + url.PathEscape(calendar)
}

func graphInstant(at scheduledTime) (time.Time, error) {
	if at.Zone != calendarUTC {
		return time.Time{}, fmt.Errorf("calendar: provider did not return UTC")
	}
	return time.Parse("2006-01-02T15:04:05.999999999", strings.TrimSuffix(at.At, "Z"))
}

func (a *httpAPI) Busy(ctx context.Context, token, calendar string, from, to time.Time) ([]connector.CalendarInterval, error) {
	query := url.Values{"startDateTime": {from.Format(time.RFC3339)}, "endDateTime": {to.Format(time.RFC3339)}, "$top": {"100"}, "$select": {"id,start,end,isAllDay,isCancelled,showAs,responseStatus"}}
	next := a.base + calendarPath(calendar) + "/calendarView?" + query.Encode()
	busy := []connector.CalendarInterval{}
	for page := 0; next != "" && page < 100; page++ {
		var result struct {
			Events []scheduledEvent `json:"value"`
			Next   string           `json:"@odata.nextLink"`
		}
		if _, err := calendarwire.Request(ctx, a.client, token, http.MethodGet, next, nil, &result); err != nil {
			return nil, err
		}
		for _, event := range result.Events {
			if event.Canceled || event.ShowAs == "free" || event.Response.Response == "declined" {
				continue
			}
			start, err := graphInstant(event.Start)
			if err != nil {
				return nil, err
			}
			end, err := graphInstant(event.End)
			if err != nil {
				return nil, err
			}
			if !end.After(start) {
				return nil, fmt.Errorf("calendar: invalid busy interval")
			}
			busy = append(busy, connector.CalendarInterval{EventID: event.ID, Start: start, End: end})
		}
		next = result.Next
		if next != "" && !strings.HasPrefix(next, a.base+"/") {
			return nil, fmt.Errorf("calendar: invalid continuation")
		}
	}
	if next != "" {
		return nil, fmt.Errorf("calendar: incomplete availability")
	}
	return busy, nil
}

func (a *httpAPI) Save(ctx context.Context, token string, in connector.CalendarAppointment) (connector.CalendarReceipt, error) {
	event := scheduledEvent{
		Properties: []requestProperty{{meetingRequestProperty, in.RequestID}}, Subject: in.Subject, RequestID: in.RequestID,
		Start: scheduledTime{in.Start.UTC().Format("2006-01-02T15:04:05"), calendarUTC},
		End:   scheduledTime{in.End.UTC().Format("2006-01-02T15:04:05"), calendarUTC},
	}
	event.Body.Type, event.Body.Content = "text", in.Description
	event.Location.Name = in.Location
	for _, email := range in.Attendees {
		attendee := scheduledAttendee{Type: "required"}
		attendee.Address.Address = email
		event.Attendees = append(event.Attendees, attendee)
	}
	path := a.base + calendarPath(in.CalendarID) + "/events"
	method := http.MethodPost
	if in.EventID != "" {
		path = a.base + "/me/events/" + url.PathEscape(in.EventID)
		method = http.MethodPatch
		event.RequestID = ""
	}
	var result scheduledEvent
	if _, err := calendarwire.Request(ctx, a.client, token, method, path, event, &result); err != nil {
		return connector.CalendarReceipt{}, err
	}
	if result.ID == "" || result.Canceled {
		return connector.CalendarReceipt{}, fmt.Errorf("calendar: event was not confirmed")
	}
	return connector.CalendarReceipt{EventID: result.ID, UID: result.UID, URL: result.URL}, nil
}

func (a *httpAPI) Cancel(ctx context.Context, token, _ string, event string) error {
	status, err := calendarwire.Request(ctx, a.client, token, http.MethodDelete, a.base+"/me/events/"+url.PathEscape(event), nil, nil)
	if status == http.StatusNotFound {
		return nil
	}
	return err
}

var _ connector.CalendarScheduler = (*Connector)(nil)

// CalendarLookup recovers uncertain delivery before retrying a write.
func (c *Connector) CalendarLookup(ctx context.Context, auth connector.Auth, in connector.CalendarAppointment) (*connector.CalendarReceipt, error) {
	token, err := c.calendarAccess(ctx, auth)
	if err != nil {
		return nil, err
	}
	api, ok := c.api.(schedulingAPI)
	if !ok {
		return nil, connector.ErrUnreachable
	}
	return api.Lookup(ctx, token, in)
}

// CalendarList reports the current selectable calendars and write authority.
func (c *Connector) CalendarList(ctx context.Context, auth connector.Auth) ([]connector.CalendarOption, error) {
	token, err := c.calendarAccess(ctx, auth)
	if err != nil {
		return nil, err
	}
	api, ok := c.api.(schedulingAPI)
	if !ok {
		return nil, connector.ErrUnreachable
	}
	return api.List(ctx, token)
}

// CalendarInspect reads the current provider interval or cancellation.
func (c *Connector) CalendarInspect(ctx context.Context, auth connector.Auth, calendar, event string) (connector.CalendarState, error) {
	token, err := c.calendarAccess(ctx, auth)
	if err != nil {
		return connector.CalendarState{}, err
	}
	api, ok := c.api.(schedulingAPI)
	if !ok {
		return connector.CalendarState{}, connector.ErrUnreachable
	}
	return api.Inspect(ctx, token, calendar, event)
}
