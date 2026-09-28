// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/platform/keyvault"
)

// Booking scenarios use real OAuth, registry, consent and scheduling wiring;
// only Google's HTTP boundary is replaced.
func setupBookingApp(t *testing.T) *apptest.AppEnv {
	t.Helper()
	e, _ := setupBookingProvider(t)
	return e
}

func setupBookingProvider(t *testing.T) (*apptest.AppEnv, *bookingProviderTransport) {
	t.Helper()
	previous := http.DefaultTransport
	provider := &bookingProviderTransport{fallback: previous, calendar: "ada@example.com", events: make(map[string]json.RawMessage)}
	http.DefaultTransport = provider
	t.Cleanup(func() { http.DefaultTransport = previous })
	vault := keyvault.NewMemory()
	e := apptest.SetupAppWithOptions(t, compose.WithKeyvault(vault), compose.WithGmailCapture(compose.GmailConfig{
		ClientID: "calendar-fixture", ClientSecret: "calendar-fixture", StateKey: "0123456789abcdef0123456789abcdef", PublicBaseURL: "https://mail.example.test",
	}, compose.CaptureConfig{}))
	e.Vault = vault
	return e, provider
}

type bookingProviderTransport struct {
	fallback http.RoundTripper
	calendar string
	mu       sync.Mutex
	events   map[string]json.RawMessage
}

func (b *bookingProviderTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Host == "oauth2.googleapis.com" {
		return bookingProviderResponse(r, `{"access_token":"fixture-access","refresh_token":"fixture-refresh","scope":"https://www.googleapis.com/auth/calendar.readonly https://www.googleapis.com/auth/calendar.events.owned"}`), nil
	}
	if r.URL.Host != "www.googleapis.com" {
		return b.fallback.RoundTrip(r)
	}
	switch {
	case strings.HasSuffix(r.URL.Path, "/users/me/calendarList"):
		b.mu.Lock()
		calendar := b.calendar
		b.mu.Unlock()
		return bookingProviderResponse(r, fmt.Sprintf(`{"items":[{"id":%q,"summary":"Work","accessRole":"owner","primary":true}]}`, calendar)), nil
	case strings.HasSuffix(r.URL.Path, "/calendars/primary"):
		return bookingProviderResponse(r, `{"id":"ada@example.com"}`), nil
	case strings.HasSuffix(r.URL.Path, "/freeBusy"):
		return bookingProviderResponse(r, `{"calendars":{"ada@example.com":{"busy":[]}}}`), nil
	case strings.Contains(r.URL.Path, "/events"):
		return b.event(r)
	default:
		return nil, fmt.Errorf("unexpected calendar fixture request: %s %s", r.Method, r.URL.Path)
	}
}

func bookingProviderResponse(r *http.Request, body string) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}
}

func enableBookingPage(t *testing.T, e *apptest.AppEnv) {
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
	hours := json.RawMessage(`{"start_time":"09:00","end_time":"17:00","days":[1,2,3,4,5],"timezone":"UTC"}`)
	if status := e.Call(t, "PUT", "/v1/me/working-hours", hours, nil, nil); status != http.StatusOK {
		t.Fatalf("working hours: %d", status)
	}
	profile := AnyMap{"provider": "gcal", "calendar_id": "ada@example.com", "enabled": true, "duration_minutes": 30, "notice_minutes": 0, "buffer_minutes": 0, "horizon_days": 30, "title": "Meeting", "host_name": "Ada", "location": "Video call"}
	if status := e.Call(t, "PUT", "/v1/scheduling/profile", profile, nil, nil); status != http.StatusOK {
		t.Fatalf("enable booking page: %d", status)
	}
}
