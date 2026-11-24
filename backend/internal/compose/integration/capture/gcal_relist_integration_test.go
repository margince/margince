// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// A calendar drops an instance a series split replaced without a
// cancellation. A full re-list is the only word on it, so a stored future
// meeting the complete listing left out is retired.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture/gcal"
	"github.com/margince/margince/backend/internal/modules/capture/gmail"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const relistOrganizer = "host@customer.test"

// relistCalendar is a stubbed Google Calendar whose token can expire and whose
// full listing can fail.
type relistCalendar struct {
	mu           sync.Mutex
	items        []map[string]any
	tokenExpired bool
	listingFails bool
}

func (c *relistCalendar) serve(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, _ *http.Request) {
		integration.WriteJSON(w, map[string]any{"access_token": "access-tok", "refresh_token": "refresh-tok", "expires_in": 3599})
	})
	mux.HandleFunc("/calendars/primary", func(w http.ResponseWriter, _ *http.Request) {
		integration.WriteJSON(w, map[string]any{"id": captureOwner})
	})
	mux.HandleFunc("/calendars/primary/events", func(w http.ResponseWriter, r *http.Request) {
		c.mu.Lock()
		defer c.mu.Unlock()
		incremental := r.URL.Query().Get("syncToken") != ""
		switch {
		case incremental && c.tokenExpired:
			w.WriteHeader(http.StatusGone)
		case !incremental && c.listingFails:
			w.WriteHeader(http.StatusInternalServerError)
		default:
			integration.WriteJSON(w, map[string]any{"items": c.items, "nextSyncToken": "synctok"})
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// relistEnv is the owner's calendar connected on the production registry,
// holding a kept and a dropped future meeting and a dropped past one.
type relistEnv struct {
	env        captureEnv
	cal        *relistCalendar
	conn       ids.UUID
	kept, past map[string]any
}

func newRelistEnv(t *testing.T) relistEnv {
	t.Helper()
	env := newCaptureEnv(t)
	ahead := time.Now().UTC().AddDate(0, 0, 7).Truncate(time.Hour)
	event := func(id string, start time.Time) map[string]any {
		return lateGuestEvent(id, "confirmed", start, relistOrganizer, selfEntry(captureOwner, "accepted"), guestEntry(relistOrganizer, "accepted"))
	}
	r := relistEnv{env: env, kept: event("evt-kept", ahead), past: event("evt-past", time.Now().UTC().AddDate(0, 0, -2).Truncate(time.Hour))}
	r.cal = &relistCalendar{items: []map[string]any{r.kept, event("evt-dropped", ahead.Add(2*time.Hour)), r.past}}
	stub := r.cal.serve(t)
	oauth := gmail.NewOAuth(gmail.OAuthConfig{ClientID: "cid", ClientSecret: "sec", TokenURL: stub.URL + "/token"})
	api := gcal.NewAPI(stub.Client(), stub.URL)
	env.registry.Register(gcal.New(oauth, api))
	req, err := gcal.AuthRequestFrom("the-code", "https://app.test/v1/connectors/gcal/callback")
	if err != nil {
		t.Fatal(err)
	}
	granted, err := gcal.New(oauth, api).Authenticate(context.Background(), req)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if r.conn, err = env.registry.Connect(humanWithScopes(env.e, env.e.Rep1, []principal.Scope{principal.ScopeRead}), "gcal", granted); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if err := r.sync(); err != nil {
		t.Fatalf("the first sync: %v", err)
	}
	return r
}

func (r relistEnv) sync() error {
	return r.env.registry.SyncOnce(principal.WithWorkspaceID(context.Background(), r.env.e.WS), r.conn)
}

// relist leaves only the kept event on the calendar and expires the sync
// token. The next sync then lists the calendar in full.
func (r relistEnv) relist(listingFails bool) error {
	r.cal.mu.Lock()
	r.cal.items = []map[string]any{r.kept}
	r.cal.tokenExpired, r.cal.listingFails = true, listingFails
	r.cal.mu.Unlock()
	return r.sync()
}

// cancelled counts the event's meeting rows the cancellation closed.
func (r relistEnv) cancelled(t *testing.T, event string) int {
	t.Helper()
	return countRows(t, r.env.e, `SELECT count(*) FROM activity
		WHERE source_system = 'gcal' AND source_id = '`+event+`' AND meeting_status = 'canceled'`)
}

func TestAFullRelistRetiresTheFutureMeetingsItNoLongerHolds(t *testing.T) {
	r := newRelistEnv(t)
	if got := countRows(t, r.env.e, `SELECT count(*) FROM activity WHERE source_system = 'gcal' AND meeting_status IS NULL`); got != 3 {
		t.Fatalf("the first sync stored %d open meetings, want 3", got)
	}

	if err := r.relist(false); err != nil {
		t.Fatalf("the re-list: %v", err)
	}

	if got := r.cancelled(t, "evt-dropped"); got != 1 {
		t.Error("the future meeting the full listing left out is still open")
	}
	if got := r.cancelled(t, "evt-kept"); got != 0 {
		t.Error("the meeting the listing still holds was retired")
	}
	if got := r.cancelled(t, "evt-past"); got != 0 {
		t.Error("a past meeting the listing left out was retired")
	}
}

func TestAnInterruptedRelistRetiresNothing(t *testing.T) {
	r := newRelistEnv(t)

	if err := r.relist(true); err == nil {
		t.Fatal("a sync whose listing failed reported success")
	}

	if got := countRows(t, r.env.e, `SELECT count(*) FROM activity WHERE source_system = 'gcal' AND meeting_status = 'canceled'`); got != 0 {
		t.Errorf("an interrupted listing retired %d meetings, want none", got)
	}
}

// A meeting another seat imports is still on that seat's calendar, so one
// calendar leaving it out does not call it off.
func TestARelistDoesNotCancelAMeetingAnotherSeatHolds(t *testing.T) {
	r := newRelistEnv(t)
	exec(t, r.env.e, `INSERT INTO capture_import (activity_id, user_id)
		SELECT id, $1 FROM activity WHERE source_system = 'gcal' AND source_id = 'evt-dropped'`, r.env.e.Rep3)

	if err := r.relist(false); err != nil {
		t.Fatalf("the re-list: %v", err)
	}

	if got := r.cancelled(t, "evt-dropped"); got != 0 {
		t.Error("a meeting another seat imports was cancelled by one calendar's listing")
	}
}
