// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package graphcal

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/capture/oauthflow"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// fixedNow pins the clock so a stored cursor's window is fresh and the tests
// below stay on the incremental path rather than tripping the re-anchor.
var fixedNow = time.Date(2026, 7, 16, 9, 0, 0, 0, time.UTC)

// newAt builds a connector whose clock is pinned, so a test states which side
// of the re-anchor threshold it is on rather than inheriting the wall clock.
func newAt(oauth OAuth, api API, now time.Time) *Connector {
	c := New(oauth, api)
	c.now = func() time.Time { return now }
	return c
}

// --- fakes ---------------------------------------------------------------

type fakeOAuth struct {
	refresh, access string
	granted         []string
	// rotated is what Microsoft hands back in place of the stored refresh
	// token; empty means it rotated nothing this round.
	rotated string
}

func (f fakeOAuth) AuthCodeURL(state, _ string) string { return "https://auth?state=" + state }
func (f fakeOAuth) Exchange(context.Context, string, string) (oauthflow.TokenGrant, error) {
	return oauthflow.TokenGrant{RefreshToken: f.refresh, Scopes: f.granted}, nil
}
func (f fakeOAuth) AccessToken(context.Context, string) (string, error) { return f.access, nil }

func (f fakeOAuth) Refresh(context.Context, string) (oauthflow.TokenRefresh, error) {
	return oauthflow.TokenRefresh{AccessToken: f.access, Rotated: f.rotated}, nil
}

type fakeAPI struct {
	email string

	initEvents [][]byte
	initDelta  string
	initCalls  int

	deltaEvents [][]byte
	deltaLink   string
	deltaErr    error
	deltaCalls  int
	seenDelta   string
}

func (f *fakeAPI) Owner(context.Context, string) (string, error) { return f.email, nil }

func (f *fakeAPI) ViewInitial(context.Context, string) ([][]byte, string, error) {
	f.initCalls++
	return f.initEvents, f.initDelta, nil
}

func (f *fakeAPI) ViewDelta(_ context.Context, _, deltaLink string) ([][]byte, string, error) {
	f.deltaCalls++
	f.seenDelta = deltaLink
	if f.deltaErr != nil {
		return nil, "", f.deltaErr
	}
	return f.deltaEvents, f.deltaLink, nil
}

type recordingSink struct{ recs []connector.NormalizedRecord }

func (s *recordingSink) Upsert(_ context.Context, rec connector.NormalizedRecord) (datasource.EntityRef, error) {
	s.recs = append(s.recs, rec)
	return datasource.EntityRef{}, nil
}

// sealedAuth is the credential bundle a connected calendar holds.
func sealedAuth(t *testing.T) connector.Auth {
	t.Helper()
	b, err := json.Marshal(authState{RefreshToken: "r", Owner: owner, Granted: Scopes()})
	if err != nil {
		t.Fatalf("marshal auth: %v", err)
	}
	return b
}

// --- the connector -------------------------------------------------------

func TestDescriptorIsReadOnlyAndProducesActivities(t *testing.T) {
	d := newAt(fakeOAuth{}, &fakeAPI{}, fixedNow).Descriptor()
	if d.Name != connectorName {
		t.Errorf("Name = %q, want %q", d.Name, connectorName)
	}
	if d.RiskTier != mcp.TierAutoExecute {
		t.Errorf("RiskTier = %v, want auto-execute (a read-only capture connector)", d.RiskTier)
	}
	if len(d.Produces) != 1 || d.Produces[0] != datasource.EntityActivity {
		t.Errorf("Produces = %v, want activity only", d.Produces)
	}
}

func TestAuthenticateSealsTheRefreshTokenAndTheOwner(t *testing.T) {
	c := newAt(fakeOAuth{refresh: "r-1", access: "a-1", granted: Scopes()}, &fakeAPI{email: owner}, fixedNow)
	req, err := AuthRequestFrom("code-1", "https://api.example.com/v1/connectors/graphcal/callback")
	if err != nil {
		t.Fatalf("AuthRequestFrom: %v", err)
	}
	auth, err := c.Authenticate(context.Background(), req)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	var st authState
	if err := json.Unmarshal(auth, &st); err != nil {
		t.Fatalf("unmarshal sealed auth: %v", err)
	}
	if st.RefreshToken != "r-1" {
		t.Errorf("RefreshToken = %q, want the exchanged one", st.RefreshToken)
	}
	if st.Owner != owner {
		t.Errorf("Owner = %q, want %q", st.Owner, owner)
	}
	// The access token is minted per sync and must never be persisted.
	if strings.Contains(string(auth), "a-1") {
		t.Error("the sealed bundle carries the short-lived access token")
	}
	if label, err := c.AccountLabel(auth); err != nil || label != owner {
		t.Errorf("AccountLabel = (%q,%v), want the authorizing account", label, err)
	}
	granted, err := c.GrantedScopes(auth)
	if err != nil || len(granted) != len(Scopes()) {
		t.Errorf("GrantedScopes = (%v,%v), want what Microsoft granted", granted, err)
	}
}

func TestAuthenticateRefusesAnEmptyCode(t *testing.T) {
	c := newAt(fakeOAuth{}, &fakeAPI{email: owner}, fixedNow)
	req, err := AuthRequestFrom("", "https://api.example.com/cb")
	if err != nil {
		t.Fatalf("AuthRequestFrom: %v", err)
	}
	if _, err := c.Authenticate(context.Background(), req); !errors.Is(err, connector.ErrAuthRejected) {
		t.Fatalf("Authenticate(no code) = %v, want an auth rejection", err)
	}
}

func TestSyncWithNoCursorAnchorsAndCapturesTheWindow(t *testing.T) {
	kept := eventJSON(t, "evt-1", "Kickoff", "2026-07-16T09:00:00.0000000", "UTC", owner, "client@acme.com")
	internal := eventJSON(t, "evt-2", "Standup", "2026-07-16T10:00:00.0000000", "UTC", owner, "peer@myco.com")
	api := &fakeAPI{email: owner, initEvents: [][]byte{kept, internal}, initDelta: "https://graph/delta?$1"}
	sink := &recordingSink{}

	cur, err := newAt(fakeOAuth{access: "a"}, api, fixedNow).Sync(context.Background(), sealedAuth(t), nil, sink)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if api.initCalls != 1 || api.deltaCalls != 0 {
		t.Errorf("calls = (init %d, delta %d), want the initial anchor only", api.initCalls, api.deltaCalls)
	}
	if len(sink.recs) != 1 || sink.recs[0].NaturalKey.SourceID != "evt-1" {
		t.Fatalf("captured %d record(s) %+v, want only the meeting with an outside party", len(sink.recs), sink.recs)
	}
	if got := mustCursor(t, cur); got.DeltaLink != "https://graph/delta?$1" || got.Email != owner {
		t.Errorf("cursor = %+v, want the advanced link bound to the account", got)
	}
}

func TestSyncWithACursorResumesFromIt(t *testing.T) {
	api := &fakeAPI{email: owner, deltaLink: "https://graph/delta?$2"}
	sink := &recordingSink{}
	prior := marshalCursor("https://graph/delta?$1", owner, fixedNow)

	cur, err := newAt(fakeOAuth{access: "a"}, api, fixedNow).Sync(context.Background(), sealedAuth(t), prior, sink)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if api.initCalls != 0 || api.seenDelta != "https://graph/delta?$1" {
		t.Errorf("calls = (init %d, resumed from %q), want the stored link resumed", api.initCalls, api.seenDelta)
	}
	if got := mustCursor(t, cur); got.DeltaLink != "https://graph/delta?$2" {
		t.Errorf("cursor = %+v, want the advanced link", got)
	}
}

func TestAnExpiredDeltaReAnchorsRatherThanFailing(t *testing.T) {
	api := &fakeAPI{
		email: owner, deltaErr: ErrDeltaGone,
		initEvents: [][]byte{}, initDelta: "https://graph/delta?fresh",
	}
	prior := marshalCursor("https://graph/delta?stale", owner, fixedNow)

	cur, err := newAt(fakeOAuth{access: "a"}, api, fixedNow).Sync(context.Background(), sealedAuth(t), prior, &recordingSink{})
	if err != nil {
		t.Fatalf("Sync after an expired delta = %v, want a bounded re-anchor", err)
	}
	if api.initCalls != 1 {
		t.Errorf("initCalls = %d, want one re-anchor", api.initCalls)
	}
	if got := mustCursor(t, cur); got.DeltaLink != "https://graph/delta?fresh" {
		t.Errorf("cursor = %+v, want the fresh link", got)
	}
}

// A stored cursor we cannot read is corruption, not a fresh calendar: stopping
// is what keeps the watermark. Re-anchoring would overwrite it and silently
// drop every event between the real watermark and the new window.
func TestAnUnreadableCursorStopsRatherThanReAnchoring(t *testing.T) {
	api := &fakeAPI{email: owner, initDelta: "https://graph/delta?fresh"}
	for name, cur := range map[string]connector.Cursor{
		"not json":   connector.Cursor("}not json{"),
		"no link":    marshalCursor("", owner, fixedNow),
		"wrong keys": connector.Cursor(`{"sync_token":"x"}`),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := newAt(fakeOAuth{access: "a"}, api, fixedNow).Sync(context.Background(), sealedAuth(t), cur, &recordingSink{}); err == nil {
				t.Fatal("Sync must stop on an unreadable cursor rather than re-anchor over the watermark")
			}
			if api.initCalls != 0 {
				t.Fatalf("initCalls = %d, want none — an unreadable cursor must not trigger a re-anchor", api.initCalls)
			}
		})
	}
}

// A Sink refusal for one event is a per-record drop, never a pull-stopping
// fault: the rest of the round still lands.
func TestASinkSkipDropsOneEventAndKeepsPulling(t *testing.T) {
	first := eventJSON(t, "evt-a", "One", "2026-07-16T09:00:00.0000000", "UTC", owner, "client@acme.com")
	second := eventJSON(t, "evt-b", "Two", "2026-07-16T10:00:00.0000000", "UTC", owner, "other@acme.com")
	api := &fakeAPI{email: owner, initEvents: [][]byte{first, second}, initDelta: "https://graph/delta?$1"}

	sink := &skippingSink{skipID: "evt-a"}
	if _, err := newAt(fakeOAuth{access: "a"}, api, fixedNow).Sync(context.Background(), sealedAuth(t), nil, sink); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if sink.seen != 2 {
		t.Errorf("the sink saw %d event(s), want both offered", sink.seen)
	}
}

type skippingSink struct {
	skipID string
	seen   int
}

func (s *skippingSink) Upsert(_ context.Context, rec connector.NormalizedRecord) (datasource.EntityRef, error) {
	s.seen++
	if rec.NaturalKey.SourceID == s.skipID {
		return datasource.EntityRef{}, connector.ErrSkip
	}
	return datasource.EntityRef{}, nil
}

func TestHealthCheckMintsATokenAndAsksWhoTheAccountIs(t *testing.T) {
	c := newAt(fakeOAuth{access: "a"}, &fakeAPI{email: owner}, fixedNow)
	if err := c.HealthCheck(context.Background(), sealedAuth(t)); err != nil {
		t.Fatalf("HealthCheck: %v", err)
	}
	if err := c.HealthCheck(context.Background(), connector.Auth("}not json{")); err == nil {
		t.Fatal("HealthCheck must reject a malformed bundle")
	}
}

func TestNormalizeReportsADroppedEventAsASkip(t *testing.T) {
	c := newAt(fakeOAuth{}, &fakeAPI{}, fixedNow)
	c.owner = owner
	solo := eventJSON(t, "evt-solo", "Focus time", "2026-07-16T09:00:00.0000000", "UTC", owner)
	if _, err := c.Normalize(context.Background(), solo); !errors.Is(err, connector.ErrSkip) {
		t.Fatalf("Normalize(solo event) = %v, want an ErrSkip-wrapped refusal", err)
	}
	kept := eventJSON(t, "evt-keep", "Demo", "2026-07-16T09:00:00.0000000", "UTC", owner, "client@acme.com")
	recs, err := c.Normalize(context.Background(), kept)
	if err != nil || len(recs) != 1 {
		t.Fatalf("Normalize(kept) = (%d recs, %v), want one record", len(recs), err)
	}
}

// --- helpers -------------------------------------------------------------

func mustCursor(t *testing.T, cur connector.Cursor) cursorState {
	t.Helper()
	var cs cursorState
	if err := json.Unmarshal(cur, &cs); err != nil {
		t.Fatalf("unmarshal cursor: %v", err)
	}
	return cs
}

// THE WINDOW HAS TO MOVE ON ITS OWN.
//
// Graph's calendarView delta reports only events starting inside the range it
// was opened against, and a valid deltaLink is resumed forever — so without a
// scheduled re-anchor a meeting booked past the forward edge is never reported
// and nothing says so. This is the whole reason the cursor dates its window.
func TestAStaleWindowIsReopenedEvenThoughTheDeltaStillWorks(t *testing.T) {
	anchored := fixedNow
	prior := marshalCursor("https://graph/delta?$1", owner, anchored)

	t.Run("fresh window resumes", func(t *testing.T) {
		api := &fakeAPI{email: owner, deltaLink: "https://graph/delta?$2"}
		_, err := newAt(fakeOAuth{access: "a"}, api, anchored.Add(reanchorAfter-time.Hour)).
			Sync(context.Background(), sealedAuth(t), prior, &recordingSink{})
		if err != nil {
			t.Fatalf("Sync: %v", err)
		}
		if api.initCalls != 0 {
			t.Error("a window still inside its lifetime was reopened for nothing")
		}
	})

	t.Run("stale window re-anchors and redates itself", func(t *testing.T) {
		api := &fakeAPI{email: owner, initDelta: "https://graph/delta?fresh", deltaLink: "https://graph/delta?$2"}
		at := anchored.Add(reanchorAfter)
		cur, err := newAt(fakeOAuth{access: "a"}, api, at).
			Sync(context.Background(), sealedAuth(t), prior, &recordingSink{})
		if err != nil {
			t.Fatalf("Sync: %v", err)
		}
		if api.initCalls != 1 || api.deltaCalls != 0 {
			t.Fatalf("calls = (init %d, delta %d), want the window reopened rather than resumed", api.initCalls, api.deltaCalls)
		}
		got := mustCursor(t, cur)
		if got.DeltaLink != "https://graph/delta?fresh" {
			t.Errorf("cursor = %+v, want the link the re-anchor produced", got)
		}
		if !got.AnchoredAt.Equal(at) {
			t.Errorf("AnchoredAt = %v, want %v — a window that never redates itself re-anchors on every sync forever", got.AnchoredAt, at)
		}
	})

	t.Run("an incremental round keeps the original anchor", func(t *testing.T) {
		api := &fakeAPI{email: owner, deltaLink: "https://graph/delta?$2"}
		cur, err := newAt(fakeOAuth{access: "a"}, api, anchored.Add(time.Hour)).
			Sync(context.Background(), sealedAuth(t), prior, &recordingSink{})
		if err != nil {
			t.Fatalf("Sync: %v", err)
		}
		// Refreshing the anchor on every sync would mean it never reads as
		// stale, and the window would never move at all.
		if got := mustCursor(t, cur); !got.AnchoredAt.Equal(anchored) {
			t.Errorf("AnchoredAt = %v, want the window's original %v", got.AnchoredAt, anchored)
		}
	})
}

// A cursor written before the anchor was recorded reopens once, and starts
// carrying one.
func TestACursorWithNoAnchorReopensOnceAndThenKeepsOne(t *testing.T) {
	api := &fakeAPI{email: owner, initDelta: "https://graph/delta?fresh"}
	legacy := connector.Cursor(`{"delta_link":"https://graph/delta?old","email":"` + owner + `"}`)

	cur, err := newAt(fakeOAuth{access: "a"}, api, fixedNow).
		Sync(context.Background(), sealedAuth(t), legacy, &recordingSink{})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if api.initCalls != 1 {
		t.Fatalf("initCalls = %d, want the undated window reopened once", api.initCalls)
	}
	if got := mustCursor(t, cur); !got.AnchoredAt.Equal(fixedNow) {
		t.Errorf("AnchoredAt = %v, want the cursor to start carrying one", got.AnchoredAt)
	}
}
