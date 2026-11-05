// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

//nolint:tagliatelle // The fixture implements Google’s calendar wire format.
package apptest

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
)

// BookingProvider stands in for Google's OAuth and calendar hosts in front of
// fallback, holding the events the gcal adapter writes.
type BookingProvider struct {
	fallback http.RoundTripper
	calendar string
	mu       sync.Mutex
	events   map[string]json.RawMessage
}

// NewBookingProvider answers for the calendar ada@example.com, and passes
// every other host to fallback.
func NewBookingProvider(fallback http.RoundTripper) *BookingProvider {
	return &BookingProvider{fallback: fallback, calendar: "ada@example.com", events: make(map[string]json.RawMessage)}
}

// SetCalendar changes the account's primary calendar, as a user who
// reconnects a different Google account would.
func (b *BookingProvider) SetCalendar(id string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.calendar = id
}

func (b *BookingProvider) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Host == "oauth2.googleapis.com" {
		return BookingProviderResponse(r, `{"access_token":"fixture-access","refresh_token":"fixture-refresh","scope":"https://www.googleapis.com/auth/calendar.readonly https://www.googleapis.com/auth/calendar.events.owned"}`), nil
	}
	if r.URL.Host != "www.googleapis.com" {
		return b.fallback.RoundTrip(r)
	}
	switch {
	case strings.HasSuffix(r.URL.Path, "/users/me/calendarList"):
		b.mu.Lock()
		calendar := b.calendar
		b.mu.Unlock()
		return BookingProviderResponse(r, fmt.Sprintf(`{"items":[{"id":%q,"summary":"Work","accessRole":"owner","primary":true}]}`, calendar)), nil
	case strings.HasSuffix(r.URL.Path, "/calendars/primary"):
		return BookingProviderResponse(r, `{"id":"ada@example.com"}`), nil
	case strings.HasSuffix(r.URL.Path, "/freeBusy"):
		return BookingProviderResponse(r, `{"calendars":{"ada@example.com":{"busy":[]}}}`), nil
	case strings.Contains(r.URL.Path, "/events"):
		return b.event(r)
	default:
		return nil, fmt.Errorf("unexpected calendar fixture request: %s %s", r.Method, r.URL.Path)
	}
}

// BookingProviderResponse is a 200 answer to r carrying the JSON body.
func BookingProviderResponse(r *http.Request, body string) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}
}

// EnableGoogleBookingPage connects the signed-in caller's Google calendar
// through the real OAuth callback and switches their booking page on.
func EnableGoogleBookingPage(t *testing.T, e *AppEnv) {
	t.Helper()
	var connection struct {
		AuthorizeURL string `json:"authorize_url"`
	}
	if status := e.Call(t, "POST", "/v1/connectors/gcal/connect", nil, nil, &connection); status != http.StatusOK {
		t.Fatalf("calendar connect: %d", status)
	}
	auth, err := url.Parse(connection.AuthorizeURL)
	if err != nil {
		t.Fatal(err)
	}
	redirect := e.Client.CheckRedirect
	e.Client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	status := e.Call(t, "GET", "/v1/connectors/gcal/callback?code=fixture&state="+url.QueryEscape(auth.Query().Get("state")), nil, nil, nil)
	e.Client.CheckRedirect = redirect
	if status != http.StatusFound && status != http.StatusSeeOther {
		t.Fatalf("calendar callback: %d", status)
	}
	zone := json.RawMessage(`{"timezone":"UTC","source":"chosen"}`)
	if status := e.Call(t, "PUT", "/v1/me/timezone", zone, nil, nil); status != http.StatusOK {
		t.Fatalf("time zone: %d", status)
	}
	hours := json.RawMessage(`{"start_time":"09:00","end_time":"17:00","days":[1,2,3,4,5]}`)
	if status := e.Call(t, "PUT", "/v1/me/working-hours", hours, nil, nil); status != http.StatusOK {
		t.Fatalf("working hours: %d", status)
	}
	profile := map[string]any{"provider": "gcal", "calendar_id": "ada@example.com", "enabled": true, "duration_minutes": 30, "notice_minutes": 0, "buffer_minutes": 0, "horizon_days": 30, "title": "Meeting", "host_name": "Ada", "location": "Video call"}
	if status := e.Call(t, "PUT", "/v1/scheduling/profile", profile, nil, nil); status != http.StatusOK {
		t.Fatalf("enable booking page: %d", status)
	}
}

// DeliveredEvent is the event body the gcal adapter last sent for one invitation.
// A new event takes its invitation's id as its own.
func (b *BookingProvider) DeliveredEvent(t *testing.T, invitationID string) json.RawMessage {
	t.Helper()
	b.mu.Lock()
	defer b.mu.Unlock()
	event, found := b.events[strings.ReplaceAll(invitationID, "-", "")]
	if !found {
		t.Fatalf("Google holds no event for invitation %s", invitationID)
	}
	return event
}

func (b *BookingProvider) event(r *http.Request) (*http.Response, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	_, id, _ := strings.Cut(r.URL.Path, "/events/")
	if r.Method == http.MethodPost || r.Method == http.MethodPatch {
		return b.saveEvent(r, id)
	}
	if id != "" {
		event, found := b.events[id]
		if r.Method == http.MethodDelete {
			delete(b.events, id)
			return BookingProviderResponse(r, `{}`), nil
		}
		response := BookingProviderResponse(r, string(event))
		if !found {
			response.StatusCode = http.StatusNotFound
		}
		return response, nil
	}
	events := make([]json.RawMessage, 0, len(b.events))
	for _, event := range b.events {
		events = append(events, event)
	}
	encoded, err := json.Marshal(struct {
		Zone   string            `json:"timeZone"`
		Events []json.RawMessage `json:"items"`
	}{"UTC", events})
	if err != nil {
		return nil, err
	}
	return BookingProviderResponse(r, string(encoded)), nil
}

func (b *BookingProvider) saveEvent(r *http.Request, id string) (*http.Response, error) {
	if r.URL.Query().Get("sendUpdates") != "all" {
		return nil, errors.New("calendar write omitted attendee notification")
	}
	r.Body = http.MaxBytesReader(nil, r.Body, 1<<20)
	var event map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
		return nil, err
	}
	if id == "" {
		if err := json.Unmarshal(event["id"], &id); err != nil {
			return nil, err
		}
	}
	encodedID, err := json.Marshal(id)
	if err != nil {
		return nil, err
	}
	event["id"] = encodedID
	encoded, err := json.Marshal(event)
	if err != nil {
		return nil, err
	}
	b.events[id] = encoded
	return BookingProviderResponse(r, string(encoded)), nil
}
