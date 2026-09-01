// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package graphcal

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// calendarStub serves a canned two-page delta walk over an httptest server, so
// the paging and the terminal link are exercised against real HTTP.
func calendarStub(t *testing.T) (*httptest.Server, *[]string) {
	t.Helper()
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.URL.Path+"?"+r.URL.RawQuery)
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("page") == "2" {
			_, _ = w.Write([]byte(`{"value":[{"id":"evt-2"}],"@odata.deltaLink":"` + baseOf(r) + `/me/calendarView/delta?page=next"}`))
			return
		}
		_, _ = w.Write([]byte(`{"value":[{"id":"evt-1"}],"@odata.nextLink":"` + baseOf(r) + `/me/calendarView/delta?page=2"}`))
	}))
	t.Cleanup(srv.Close)
	return srv, &seen
}

func baseOf(r *http.Request) string { return "http://" + r.Host }

func TestViewInitialWalksEveryPageAndReturnsTheTerminalLink(t *testing.T) {
	srv, seen := calendarStub(t)
	api := NewAPI(srv.Client(), srv.URL)

	events, delta, err := api.ViewInitial(context.Background(), "tok")
	if err != nil {
		t.Fatalf("ViewInitial: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("collected %d event(s), want both pages", len(events))
	}
	if !strings.HasSuffix(delta, "page=next") {
		t.Errorf("deltaLink = %q, want the link the terminal page carried", delta)
	}
	if len(*seen) != 2 {
		t.Fatalf("made %d request(s) %v, want one per page", len(*seen), *seen)
	}
	// The first request must bound the window rather than asking for the whole
	// calendar: an unbounded calendarView is a request Graph refuses, and a
	// decade of standups is not what a first connect should stream.
	first := (*seen)[0]
	if !strings.Contains(first, "startDateTime") || !strings.Contains(first, "endDateTime") {
		t.Errorf("first request = %q, want the calendar window bounded", first)
	}
}

// The delta link is fetched WITH THE ACCESS TOKEN ATTACHED and persisted for
// every later cycle, so a link pointing anywhere but Graph must be refused
// before the request — not trusted because the previous hop was Graph.
func TestAContinuationLinkOffTheGraphAPIIsRefused(t *testing.T) {
	var reached bool
	evil := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached = true }))
	t.Cleanup(evil.Close)

	api := NewAPI(http.DefaultClient, "https://graph.microsoft.com/v1.0")
	_, _, err := api.ViewDelta(context.Background(), "tok", evil.URL+"/steal")
	if !errors.Is(err, connector.ErrUnreachable) {
		t.Fatalf("ViewDelta(foreign link) = %v, want it refused as unreachable", err)
	}
	if reached {
		t.Fatal("the foreign host was called — the mailbox's bearer token left Microsoft")
	}
}

func TestAnExpiredDeltaIsReportedAsCursorGone(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusGone)
	}))
	t.Cleanup(srv.Close)

	_, _, err := NewAPI(srv.Client(), srv.URL).ViewDelta(context.Background(), "tok", srv.URL+"/me/calendarView/delta?page=old")
	if !errors.Is(err, connector.ErrCursorGone) {
		t.Fatalf("a 410 = %v, want the expired-cursor sentinel so Sync re-anchors", err)
	}
}

func TestProviderFailuresMapOntoTheSharedVocabulary(t *testing.T) {
	cases := map[int]error{
		http.StatusUnauthorized:        connector.ErrAuthRejected,
		http.StatusForbidden:           connector.ErrAuthRejected,
		http.StatusTooManyRequests:     connector.ErrRateLimited,
		http.StatusInternalServerError: connector.ErrUnreachable,
	}
	for status, want := range cases {
		t.Run(http.StatusText(status), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(status)
			}))
			t.Cleanup(srv.Close)
			_, _, err := NewAPI(srv.Client(), srv.URL).ViewInitial(context.Background(), "tok")
			if !errors.Is(err, want) {
				t.Fatalf("status %d = %v, want %v", status, err, want)
			}
		})
	}
}

// A round that closes carrying neither link cannot be persisted: an empty
// cursor would force a full re-anchor every cycle, silently and permanently
// doubling the calendar's cost with nothing to report it.
func TestARoundWithNoLinkIsRefusedRatherThanPersisted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"value":[{"id":"evt-1"}]}`))
	}))
	t.Cleanup(srv.Close)

	_, _, err := NewAPI(srv.Client(), srv.URL).ViewInitial(context.Background(), "tok")
	if !errors.Is(err, connector.ErrUnreachable) {
		t.Fatalf("a linkless round = %v, want it refused rather than an empty cursor stored", err)
	}
}

func TestNewOAuthRequestsTheCalendarPermissionAlone(t *testing.T) {
	got := NewOAuth(OAuthConfig{ClientID: "cid", ClientSecret: "sec", Tenant: "common"}).
		AuthCodeURL("state-1", "https://api.example.com/cb")
	if !strings.Contains(got, "Calendars.Read") {
		t.Errorf("authorize URL = %q, want the calendar permission requested", got)
	}
	// Mail.Read belongs to the mailbox connection. A calendar consent that
	// carried it would make one grant reach both, which is exactly the boundary
	// two separate connections exist to keep.
	if strings.Contains(got, "Mail.Read") {
		t.Errorf("authorize URL = %q, want no mail permission on a calendar consent", got)
	}
}

// The window is what the standing connection watches, so it has to be anchored
// on the clock rather than frozen at construction.
func TestTheWindowIsAnchoredOnTheClock(t *testing.T) {
	srv, seen := calendarStub(t)
	api, ok := NewAPI(srv.Client(), srv.URL).(*httpAPI)
	if !ok {
		t.Fatal("NewAPI must return the concrete client so a test can pin its clock")
	}
	pinned := time.Date(2026, 7, 16, 0, 0, 0, 0, time.UTC)
	api.now = func() time.Time { return pinned }

	if _, _, err := api.ViewInitial(context.Background(), "tok"); err != nil {
		t.Fatalf("ViewInitial: %v", err)
	}
	first := (*seen)[0]
	for _, want := range []string{
		pinned.Add(-viewBackwards).Format(time.RFC3339),
		pinned.Add(viewForwards).Format(time.RFC3339),
	} {
		if !strings.Contains(first, escapeQuery(want)) {
			t.Errorf("request %q does not carry %q", first, want)
		}
	}
}

// escapeQuery renders an instant the way url.Values encodes it, so the
// assertion reads the same string the request carries.
func escapeQuery(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r == ':' {
			b.WriteString("%3A")
			continue
		}
		if r == '+' {
			b.WriteString("%2B")
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// The decode must keep each event's ORIGINAL bytes: the stored raw is the
// evidence a later replay reads, and a re-marshalled copy is not it.
func TestCollectedEventsAreTheProvidersOwnBytes(t *testing.T) {
	// The deltaLink has to point back at the server, whose URL is not known
	// until it starts — so the handler reads the host off the request it is
	// serving rather than needing the address in advance.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"value":[{"id":"evt-1","subject":"Kickoff","unknownField":42}],` +
			`"@odata.deltaLink":"` + baseOf(r) + `/me/calendarView/delta?page=next"}`))
	}))
	t.Cleanup(srv.Close)

	events, _, err := NewAPI(srv.Client(), srv.URL).ViewInitial(context.Background(), "tok")
	if err != nil {
		t.Fatalf("ViewInitial: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("collected %d event(s), want one", len(events))
	}
	var back map[string]any
	if err := json.Unmarshal(events[0], &back); err != nil {
		t.Fatalf("the collected bytes are not the event's own JSON: %v", err)
	}
	if _, ok := back["unknownField"]; !ok {
		t.Error("a field this decode does not read was dropped from the stored evidence")
	}
}
