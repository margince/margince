// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// One meeting on two colleagues' Google Calendars. Each test ends with the
// rows a sync of both calendars must leave: one live meeting both seats import.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture/gcal"
	"github.com/margince/margince/backend/internal/modules/capture/gmail"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// seatCalendars is one stubbed Google Calendar API serving several seats'
// calendars, told apart by the access token each connection was issued.
type seatCalendars struct {
	mu     sync.Mutex
	owners map[string]string
	feeds  map[string][]map[string]any
}

func (c *seatCalendars) set(label string, items ...map[string]any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.feeds[label] = items
}

func (c *seatCalendars) serve(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		label := strings.TrimPrefix(r.Form.Get("code")+r.Form.Get("refresh_token"), "refresh-")
		integration.WriteJSON(w, map[string]any{"access_token": "tok-" + label, "refresh_token": "refresh-" + label, "expires_in": 3599})
	})
	label := func(r *http.Request) string {
		return strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer tok-")
	}
	mux.HandleFunc("/calendars/primary", func(w http.ResponseWriter, r *http.Request) {
		c.mu.Lock()
		defer c.mu.Unlock()
		integration.WriteJSON(w, map[string]any{"id": c.owners[label(r)]})
	})
	mux.HandleFunc("/calendars/primary/events", func(w http.ResponseWriter, r *http.Request) {
		c.mu.Lock()
		defer c.mu.Unlock()
		integration.WriteJSON(w, map[string]any{"items": c.feeds[label(r)], "nextSyncToken": "synctok"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// pairEnv is the organiser's and the guest's calendar on one registry.
type pairEnv struct {
	env       captureEnv
	cals      *seatCalendars
	guestSeat ids.UUID
	conns     map[string]ids.UUID
}

func newPairEnv(t *testing.T) *pairEnv {
	t.Helper()
	env := newCaptureEnv(t)
	guest := seatFor(t, env.e, guestColleagueA)
	seedCaptureRoleFor(t, env.e, guest)
	cals := &seatCalendars{
		owners: map[string]string{"a": captureOwner, "b": guestColleagueA},
		feeds:  map[string][]map[string]any{},
	}
	p := &pairEnv{env: env, cals: cals, guestSeat: guest, conns: map[string]ids.UUID{}}
	stub := p.cals.serve(t)
	oauth := gmail.NewOAuth(gmail.OAuthConfig{ClientID: "cid", ClientSecret: "sec", TokenURL: stub.URL + "/token"})
	api := gcal.NewAPI(stub.Client(), stub.URL)
	env.registry.Register(gcal.New(oauth, api))
	for label, seat := range map[string]ids.UUID{"a": env.e.Rep1, "b": guest} {
		req, err := gcal.AuthRequestFrom(label, "https://app.test/v1/connectors/gcal/callback")
		if err != nil {
			t.Fatal(err)
		}
		granted, err := gcal.New(oauth, api).Authenticate(context.Background(), req)
		if err != nil {
			t.Fatalf("Authenticate: %v", err)
		}
		if p.conns[label], err = env.registry.Connect(humanWithScopes(env.e, seat, []principal.Scope{principal.ScopeRead}), "gcal", granted); err != nil {
			t.Fatalf("Connect: %v", err)
		}
	}
	return p
}

// book puts the meeting on both calendars at start. The organiser invites the
// guest seat and one outside contact. Each calendar numbers the event its own
// way, and both state one iCal UID.
func (p *pairEnv) book(start time.Time) {
	p.bookOrganizedBy(start, captureOwner)
}

// bookOrganizedBy is book with the event's organizer named.
func (p *pairEnv) bookOrganizedBy(start time.Time, organizer string) {
	for label, self := range map[string]string{"a": captureOwner, "b": guestColleagueA} {
		other := guestColleagueA
		if label == "b" {
			other = captureOwner
		}
		ev := lateGuestEvent("review-"+label, "confirmed", start, organizer,
			selfEntry(self, "accepted"), guestEntry(other, "accepted"), guestEntry(guestKnown, "accepted"))
		ev["iCalUID"] = crossSeatUID
		p.cals.set(label, ev)
	}
}

func (p *pairEnv) sync(t *testing.T, label string) {
	t.Helper()
	if err := p.env.registry.SyncOnce(principal.WithWorkspaceID(context.Background(), p.env.e.WS), p.conns[label]); err != nil {
		t.Fatalf("SyncOnce %s: %v", label, err)
	}
}

// rowOf is the meeting stored under one calendar's own event id.
func (p *pairEnv) rowOf(t *testing.T, label string) ids.UUID {
	t.Helper()
	return scalarID(t, p.env.e, `SELECT id FROM activity WHERE source_system = 'gcal' AND source_id = 'review-`+label+`'`)
}

func scalarID(t *testing.T, e *integration.SearchEnv, query string) ids.UUID {
	t.Helper()
	var id ids.UUID
	if err := e.Owner.QueryRow(context.Background(), query).Scan(&id); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return id
}

func (p *pairEnv) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := p.env.e.Owner.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
}

// loseRace leaves what two first syncs racing without the identity lock left:
// both calendars stored a row, and the organiser's row holds the identity.
// Both rows are filed under the same records, so only human work on a row
// can keep it.
func (p *pairEnv) loseRace(t *testing.T, start time.Time) (organiser, guest ids.UUID) {
	t.Helper()
	p.book(start)
	p.sync(t, "a")
	organiser = p.rowOf(t, "a")
	p.exec(t, `DELETE FROM activity_identity WHERE activity_id = $1`, organiser)
	p.sync(t, "b")
	guest = p.rowOf(t, "b")
	p.exec(t, `UPDATE activity_identity SET activity_id = $1 WHERE activity_id = $2`, organiser, guest)
	p.exec(t, `INSERT INTO activity_link (activity_id, entity_type, contact_id, company_id, deal_id, lead_id, project_id)
		SELECT $1, entity_type, contact_id, company_id, deal_id, lead_id, project_id FROM activity_link WHERE activity_id = $2`, guest, organiser)
	return organiser, guest
}

// workedByHand records an outcome a human set on the row.
func (p *pairEnv) workedByHand(t *testing.T, meeting ids.UUID) {
	t.Helper()
	p.exec(t, `UPDATE activity SET meeting_status = 'held' WHERE id = $1`, meeting)
	p.exec(t, `INSERT INTO audit_log (action, entity_type, entity_id, actor_type, actor_id, after)
		VALUES ('update', 'activity', $1, 'human', $2, '{"meeting_status":"held"}')`, meeting, "human:"+p.env.e.Rep1.String())
}

// wantOneMeeting asserts the event is one live row both seats import, holding
// the identity of start.
func (p *pairEnv) wantOneMeeting(t *testing.T, start time.Time) ids.UUID {
	t.Helper()
	e := p.env.e
	if got := countRows(t, e, `SELECT count(*) FROM activity WHERE kind = 'meeting' AND archived_at IS NULL`); got != 1 {
		t.Fatalf("%d live meetings for the event, want 1", got)
	}
	survivor := scalarID(t, e, `SELECT id FROM activity WHERE kind = 'meeting' AND archived_at IS NULL`)
	if got := countRows(t, e, `SELECT count(*) FROM capture_import WHERE activity_id = '`+survivor.String()+`'
		AND user_id IN ('`+e.Rep1.String()+`', '`+p.guestSeat.String()+`')`); got != 2 {
		t.Errorf("%d of the two seats import the meeting, want both", got)
	}
	key := activities.MeetingIdentityKey(crossSeatUID, start.UTC().Format(time.RFC3339))
	if got := countRows(t, e, `SELECT count(*) FROM activity_identity WHERE identity_key = '`+key+`'
		AND activity_id = '`+survivor.String()+`'`); got != 1 {
		t.Errorf("the meeting's identity is not on the surviving row")
	}
	return survivor
}

func TestTwoSeatsFirstSyncsRacingFileOneMeeting(t *testing.T) {
	p := newPairEnv(t)
	start := captureNow.AddDate(0, 0, -3).Truncate(time.Hour)
	p.book(start)

	var ready, done sync.WaitGroup
	ready.Add(1)
	errs := make(chan error, 2)
	for _, label := range []string{"a", "b"} {
		done.Go(func() {
			ready.Wait()
			errs <- p.env.registry.SyncOnce(principal.WithWorkspaceID(context.Background(), p.env.e.WS), p.conns[label])
		})
	}
	ready.Done()
	done.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("SyncOnce: %v", err)
		}
	}

	p.wantOneMeeting(t, start)
}

func TestALostRaceRowFoldsOnTheNextSync(t *testing.T) {
	p := newPairEnv(t)
	start := captureNow.AddDate(0, 0, -3).Truncate(time.Hour)
	organiser, _ := p.loseRace(t, start)

	p.sync(t, "b")

	if survivor := p.wantOneMeeting(t, start); survivor != organiser {
		t.Errorf("the surviving row is %v, want the organiser's row holding the identity", survivor)
	}
}

func TestAMovedMeetingSyncedByTheGuestFirstFoldsOnTheOrganisersSync(t *testing.T) {
	p := newPairEnv(t)
	start := captureNow.AddDate(0, 0, -3).Truncate(time.Hour)
	p.book(start)
	p.sync(t, "a")
	p.sync(t, "b")
	moved := start.Add(2 * time.Hour)
	p.book(moved)

	p.sync(t, "b")
	p.sync(t, "a")

	survivor := p.wantOneMeeting(t, moved)
	if got := countRows(t, p.env.e, `SELECT count(*) FROM activity WHERE id = '`+survivor.String()+`'
		AND occurred_at = '`+moved.UTC().Format(time.RFC3339)+`'`); got != 1 {
		t.Error("the surviving meeting is not at the new start")
	}
}

// A human recorded the outcome on the guest's row, so that row stays and the
// organiser's row is retired onto it.
func TestAHumanWorkedOwnRowSurvivesAndTakesTheCopy(t *testing.T) {
	p := newPairEnv(t)
	start := captureNow.AddDate(0, 0, -3).Truncate(time.Hour)
	_, guest := p.loseRace(t, start)
	p.workedByHand(t, guest)

	p.sync(t, "b")

	if survivor := p.wantOneMeeting(t, start); survivor != guest {
		t.Fatalf("the surviving row is %v, want the guest's worked row %v", survivor, guest)
	}
	if got := countRows(t, p.env.e, `SELECT count(*) FROM activity WHERE id = '`+guest.String()+`' AND meeting_status = 'held'`); got != 1 {
		t.Error("the worked row lost its outcome")
	}
}

// Both rows carry human work, so neither retires and both stay.
func TestAPairNeitherSideCanRetireStaysTwo(t *testing.T) {
	p := newPairEnv(t)
	start := captureNow.AddDate(0, 0, -3).Truncate(time.Hour)
	organiser, guest := p.loseRace(t, start)
	p.workedByHand(t, organiser)
	p.workedByHand(t, guest)

	p.sync(t, "b")

	if got := countRows(t, p.env.e, `SELECT count(*) FROM activity WHERE kind = 'meeting' AND archived_at IS NULL`); got != 2 {
		t.Errorf("%d live meetings, want both worked rows kept", got)
	}
}

// After a live fold both seats see the meeting's hour as busy. The survivor
// lists the guest only the folded row named.
func TestALiveFoldKeepsBothSeatsBusyAndEveryGuest(t *testing.T) {
	p := newPairEnv(t)
	start := captureNow.AddDate(0, 0, -3).Truncate(24 * time.Hour).Add(10 * time.Hour)
	organiser, guest := p.loseRace(t, start)
	p.exec(t, `INSERT INTO activity_participant (activity_id, address, role) VALUES ($1, 'extra@client-co.example', 'attendee')`, guest)

	p.sync(t, "b")

	if survivor := p.wantOneMeeting(t, start); survivor != organiser {
		t.Fatalf("the surviving row is %v, want the organiser's %v", survivor, organiser)
	}
	if got := countRows(t, p.env.e, `SELECT count(*) FROM activity_participant WHERE activity_id = '`+organiser.String()+`'
		AND address = 'extra@client-co.example'`); got != 1 {
		t.Errorf("the survivor lists the folded row's extra guest %d times, want once", got)
	}
	for name, seat := range map[string]ids.UUID{"the organiser": p.env.e.Rep1, "the guest seat": p.guestSeat} {
		own := humanWithScopes(p.env.e, seat, []principal.Scope{principal.ScopeRead})
		slots, _, err := activities.NewStore(p.env.e.DB()).Availability(own, ids.From[ids.UserKind](seat),
			start.Add(-time.Hour), start.Add(3*time.Hour), time.Hour)
		if err != nil {
			t.Fatalf("availability: %v", err)
		}
		for _, s := range slots {
			if s.Start.Equal(start) {
				t.Errorf("%s shows the folded meeting's hour as free", name)
			}
		}
	}
}
