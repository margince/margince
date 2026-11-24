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

// wallDay is the hour days from the wall clock. The connector judges which
// meetings are ahead by its own wall clock. A moved fixture clock would put
// them on the wrong side of it.
func wallDay(days int) time.Time {
	return time.Now().UTC().AddDate(0, 0, days).Truncate(time.Hour)
}

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
	ahead := wallDay(7)
	event := func(id string, start time.Time) map[string]any {
		return lateGuestEvent(id, "confirmed", start, relistOrganizer, selfEntry(captureOwner, "accepted"), guestEntry(relistOrganizer, "accepted"))
	}
	r := relistEnv{env: env, kept: event("evt-kept", ahead), past: event("evt-past", wallDay(-2))}
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

// omit takes every event off one calendar of the pair and expires its token.
// Its next sync then lists the calendar in full without the meeting.
func (p *pairEnv) omit(t *testing.T, label string) {
	t.Helper()
	p.cals.mu.Lock()
	p.cals.feeds[label] = nil
	if p.cals.expired == nil {
		p.cals.expired = map[string]bool{}
	}
	p.cals.expired[label] = true
	p.cals.mu.Unlock()
	p.sync(t, label)
}

// connectSecondCalendar connects another Google account of the first
// calendar's seat as calendar c, holding the same meeting under its own id.
// The registered connector reads the same stubbed calendars.
func (p *pairEnv) connectSecondCalendar(t *testing.T, start time.Time, organizer string) {
	t.Helper()
	const second = "owner.second@myco.example"
	ev := lateGuestEvent("review-c", "confirmed", start, organizer,
		selfEntry(second, "accepted"), guestEntry(guestColleagueA, "accepted"), guestEntry(guestKnown, "accepted"))
	ev["iCalUID"] = crossSeatUID
	p.cals.mu.Lock()
	p.cals.owners["c"] = second
	p.cals.feeds["c"] = []map[string]any{ev}
	p.cals.mu.Unlock()
	stub := p.cals.serve(t)
	oauth := gmail.NewOAuth(gmail.OAuthConfig{ClientID: "cid", ClientSecret: "sec", TokenURL: stub.URL + "/token"})
	api := gcal.NewAPI(stub.Client(), stub.URL)
	req, err := gcal.AuthRequestFrom("c", "https://app.test/v1/connectors/gcal/callback")
	if err != nil {
		t.Fatal(err)
	}
	granted, err := gcal.New(oauth, api).Authenticate(context.Background(), req)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if p.conns["c"], err = p.env.registry.Connect(humanWithScopes(p.env.e, p.env.e.Rep1, []principal.Scope{principal.ScopeRead}), "gcal", granted); err != nil {
		t.Fatalf("Connect: %v", err)
	}
}

// onMeeting counts the seat's own rows on the meeting.
func (p *pairEnv) onMeeting(t *testing.T, meeting, seat ids.UUID) int {
	t.Helper()
	return countRows(t, p.env.e, `SELECT count(*) FROM activity_participant
		WHERE activity_id = '`+meeting.String()+`' AND user_id = '`+seat.String()+`'`)
}

func (p *pairEnv) cancelled(t *testing.T, meeting ids.UUID) bool {
	t.Helper()
	return countRows(t, p.env.e, `SELECT count(*) FROM activity
		WHERE id = '`+meeting.String()+`' AND meeting_status = 'canceled'`) == 1
}

// The seat whose calendar left the meeting out joined a row another seat's
// calendar stored, under an event id of its own.
func TestARelistTakesTheSeatOffAMeetingAnotherSeatStored(t *testing.T) {
	p := newPairEnv(t)
	p.bookOrganizedBy(wallDay(5), relistOrganizer)
	p.sync(t, "b")
	p.sync(t, "a")
	meeting := p.rowOf(t, "b")
	if p.onMeeting(t, meeting, p.env.e.Rep1) == 0 {
		t.Fatal("the joining seat is not on the meeting")
	}

	p.omit(t, "a")

	if p.onMeeting(t, meeting, p.env.e.Rep1) != 0 {
		t.Error("the seat whose calendar left the meeting out is still on it")
	}
	if p.cancelled(t, meeting) || p.onMeeting(t, meeting, p.guestSeat) == 0 {
		t.Error("the other seat lost the meeting its calendar still holds")
	}
}

// The organizer's calendar leaving a meeting out is no cancellation while
// another seat's calendar holds it.
func TestAnOrganizersRelistDoesNotCancelAMeetingAnotherSeatHolds(t *testing.T) {
	p := newPairEnv(t)
	p.book(wallDay(5))
	p.sync(t, "a")
	p.sync(t, "b")
	meeting := p.rowOf(t, "a")

	p.omit(t, "a")

	if p.cancelled(t, meeting) {
		t.Error("the organizer's omission cancelled a meeting another seat still holds")
	}
	if p.onMeeting(t, meeting, p.guestSeat) == 0 {
		t.Error("the other seat lost the meeting")
	}
}

// One of a seat's two calendars leaving a meeting out keeps it: the other
// still lists it.
func TestARelistOfOneOfASeatsCalendarsKeepsWhatTheOtherHolds(t *testing.T) {
	p := newPairEnv(t)
	start := wallDay(5)
	p.bookOrganizedBy(start, relistOrganizer)
	p.connectSecondCalendar(t, start, relistOrganizer)
	p.sync(t, "a")
	p.sync(t, "c")
	meeting := p.rowOf(t, "a")
	if countRows(t, p.env.e, `SELECT count(*) FROM capture_import WHERE activity_id = '`+meeting.String()+`'
		AND user_id = '`+p.env.e.Rep1.String()+`' AND cardinality(provider_message_ids) = 2`) != 1 {
		t.Fatal("the seat's import does not hold both calendars' event ids")
	}

	p.omit(t, "c")

	if p.cancelled(t, meeting) || p.onMeeting(t, meeting, p.env.e.Rep1) == 0 {
		t.Error("the seat lost a meeting its other calendar still lists")
	}
}
