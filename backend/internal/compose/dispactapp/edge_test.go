// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dispactapp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/oidctoken/oidctokentest"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const testTeam = "team-1"

type fakeBots struct {
	bot capture.LiveBot
	err error
}

func (b fakeBots) LiveBotIdentity(context.Context, string) (capture.LiveBot, error) {
	return b.bot, b.err
}

type fakeSeats struct {
	agent identity.AgentIdentity
	err   error
	asked []identity.AppCaller
}

func (s *fakeSeats) InstallationWorkspace(context.Context) (ids.WorkspaceID, error) {
	return ids.WorkspaceID{UUID: ids.NewV7()}, nil
}

func (s *fakeSeats) AuthenticateAppCaller(_ context.Context, c identity.AppCaller) (identity.AgentIdentity, error) {
	s.asked = append(s.asked, c)
	return s.agent, s.err
}

type fakeReplays struct {
	seen map[string]time.Duration
	err  error
}

func (r *fakeReplays) FirstUse(_ context.Context, key string, ttl time.Duration) (bool, error) {
	if r.err != nil {
		return false, r.err
	}
	if _, used := r.seen[key]; used {
		return false, nil
	}
	r.seen[key] = ttl
	return true, nil
}

// harness is one connected install: the rig serves its keys at its address.
type harness struct {
	rig     *oidctokentest.Rig
	edge    *Edge
	seats   *fakeSeats
	replays *fakeReplays
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	rig := oidctokentest.NewRig(t)
	h := &harness{
		rig:     rig,
		seats:   &fakeSeats{agent: identity.AgentIdentity{PassportID: ids.PassportID{UUID: ids.NewV7()}, OnBehalfOf: ids.UserID{UUID: ids.NewV7()}}},
		replays: &fakeReplays{seen: map[string]time.Duration{}},
	}
	h.edge = New(fakeBots{bot: capture.LiveBot{TeamID: testTeam, Address: rig.Srv.URL}}, h.replays, rig.Srv.Client()).
		SetClock(func() time.Time { return rig.Base })
	return h
}

// token mints a call token valid at the rig's base instant, with over applied.
func (h *harness) token(t *testing.T, over map[string]any) string {
	t.Helper()
	claims := map[string]any{
		"iss": h.rig.Srv.URL, "aud": Audience, "sub": "dispact-user-1",
		"iat": h.rig.Base.Add(-10 * time.Second).Unix(), "exp": h.rig.Base.Add(50 * time.Second).Unix(),
		"jti": ids.NewV7().String(), "workspace_id": testTeam,
		"email": "rep@example.test", "email_verified": true, "display_name": "Rep",
	}
	for k, v := range over {
		claims[k] = v
	}
	return h.rig.Mint(t, oidctokentest.KID, "RS256", claims)
}

func (h *harness) authenticate(t *testing.T, bearer string) (context.Context, error) {
	t.Helper()
	return h.edge.Authenticate(t.Context(), h.seats, bearer)
}

func TestAValidCallTokenActsAsTheMembersPassport(t *testing.T) {
	h := newHarness(t)
	ctx, err := h.authenticate(t, h.token(t, nil))
	if err != nil {
		t.Fatalf("a valid call token was refused: %v", err)
	}
	actor, ok := principal.Actor(ctx)
	if !ok || actor.OnBehalfOf != h.seats.agent.OnBehalfOf.UUID || actor.PassportID != h.seats.agent.PassportID.UUID {
		t.Fatalf("the call acts as %+v, want the member's app passport", actor)
	}
	if !agents.IsAppCall(ctx) {
		t.Fatal("the call carries no app surface, so it would see the full tool list")
	}
	want := identity.AppCaller{Issuer: h.rig.Srv.URL, Subject: "dispact-user-1", Email: "rep@example.test", Label: passportLabel}
	if len(h.seats.asked) != 1 || h.seats.asked[0] != want {
		t.Fatalf("identity was asked %+v, want %+v", h.seats.asked, want)
	}
}

func TestACallTokenIsRefusedWhenAnyRuleFails(t *testing.T) {
	base := oidctokentest.NewRig(t).Base
	cases := map[string]map[string]any{
		"another issuer":            {"iss": "https://other.example.test"},
		"another audience":          {"aud": "app:margince:other"},
		"another team":              {"workspace_id": "team-2"},
		"no team":                   {"workspace_id": nil},
		"no expiry":                 {"exp": nil},
		"a life over 120 seconds":   {"iat": base.Add(-30 * time.Second).Unix(), "exp": base.Add(91 * time.Second).Unix()},
		"expiry before issue":       {"iat": base.Unix(), "exp": base.Add(-time.Second).Unix()},
		"expired past the skew":     {"iat": base.Add(-100 * time.Second).Unix(), "exp": base.Add(-31 * time.Second).Unix()},
		"issued past the skew":      {"iat": base.Add(31 * time.Second).Unix(), "exp": base.Add(90 * time.Second).Unix()},
		"no token id":               {"jti": nil},
		"no subject":                {"sub": nil},
		"an unconfirmed email":      {"email_verified": false},
		"no email_verified claim":   {"email_verified": nil},
		"a confirmed but empty one": {"email": ""},
	}
	for name, over := range cases {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t)
			if _, err := h.authenticate(t, h.token(t, over)); !errors.Is(err, ErrRejected) {
				t.Fatalf("a token with %s answered %v, want a rejection", name, err)
			}
			if len(h.seats.asked) != 0 {
				t.Fatal("a rejected token still reached the member lookup")
			}
		})
	}
}

func TestATokenWithinTheSkewIsAccepted(t *testing.T) {
	h := newHarness(t)
	early := h.token(t, map[string]any{"iat": h.rig.Base.Add(29 * time.Second).Unix(), "exp": h.rig.Base.Add(89 * time.Second).Unix()})
	if _, err := h.authenticate(t, early); err != nil {
		t.Fatalf("a token issued 29 s ahead was refused: %v", err)
	}
	late := h.token(t, map[string]any{"iat": h.rig.Base.Add(-80 * time.Second).Unix(), "exp": h.rig.Base.Add(-29 * time.Second).Unix()})
	if _, err := h.authenticate(t, late); err != nil {
		t.Fatalf("a token expired 29 s ago was refused: %v", err)
	}
}

func TestTheHeaderAlgorithmIsNeverTrusted(t *testing.T) {
	h := newHarness(t)
	for _, alg := range []string{"none", "HS256", "RS512"} {
		token := h.rig.Mint(t, oidctokentest.KID, alg, map[string]any{"iss": h.rig.Srv.URL})
		if _, err := h.authenticate(t, token); !errors.Is(err, ErrRejected) {
			t.Errorf("a token whose header says %s answered %v, want a rejection", alg, err)
		}
	}
}

func TestATokenSignedByAnotherKeyIsRefused(t *testing.T) {
	h := newHarness(t)
	forger := oidctokentest.NewRig(t)
	token := forger.Mint(t, oidctokentest.KID, "RS256", map[string]any{
		"iss": h.rig.Srv.URL, "aud": Audience, "sub": "x", "jti": "j", "workspace_id": testTeam,
		"iat": h.rig.Base.Unix(), "exp": h.rig.Base.Add(time.Minute).Unix(),
	})
	if _, err := h.authenticate(t, token); !errors.Is(err, ErrRejected) {
		t.Fatalf("a token signed by a key the install never published answered %v", err)
	}
}

func TestAnUnknownKeyIdRefetchesTheKeysAtMostOnceAMinute(t *testing.T) {
	h := newHarness(t)
	if _, err := h.authenticate(t, h.token(t, nil)); err != nil {
		t.Fatalf("warming the key cache: %v", err)
	}
	before := h.rig.CertsHitCount()
	for range 5 {
		token := h.rig.Mint(t, "rotated-key", "RS256", map[string]any{"iss": h.rig.Srv.URL})
		if _, err := h.authenticate(t, token); !errors.Is(err, ErrRejected) {
			t.Fatalf("a token under an unpublished key answered %v", err)
		}
	}
	if fetched := h.rig.CertsHitCount() - before; fetched != 0 {
		t.Fatalf("five unknown key ids inside the minute fetched the keys %d more times, want 0 after the first fetch", fetched)
	}
	later := h.rig.Base.Add(61 * time.Second)
	h.edge.verifier.SetClock(func() time.Time { return later })
	for range 3 {
		if _, err := h.authenticate(t, h.rig.Mint(t, "rotated-key", "RS256", nil)); !errors.Is(err, ErrRejected) {
			t.Fatalf("a token under an unpublished key answered %v", err)
		}
	}
	if fetched := h.rig.CertsHitCount() - before; fetched != 1 {
		t.Fatalf("a minute later the keys were fetched %d times, want once", fetched)
	}
}

func TestATokenIdIsSpentOnce(t *testing.T) {
	h := newHarness(t)
	token := h.token(t, nil)
	if _, err := h.authenticate(t, token); err != nil {
		t.Fatalf("first use: %v", err)
	}
	if _, err := h.authenticate(t, token); !errors.Is(err, ErrRejected) {
		t.Fatalf("a replayed token answered %v, want a rejection", err)
	}
	for _, ttl := range h.replays.seen {
		if want := 80 * time.Second; ttl != want {
			t.Fatalf("the token id is held for %s, want until exp plus the skew (%s)", ttl, want)
		}
	}
}

func TestAStoreThatCannotAnswerFailsClosed(t *testing.T) {
	h := newHarness(t)
	h.replays.err = errors.New("redis down")
	if _, err := h.authenticate(t, h.token(t, nil)); !errors.Is(err, agents.ErrAuthUnavailable) {
		t.Fatalf("an unreachable replay store answered %v, want unavailable", err)
	}

	h = newHarness(t)
	h.edge.bots = fakeBots{err: errors.New("db down")}
	if _, err := h.authenticate(t, h.token(t, nil)); !errors.Is(err, agents.ErrAuthUnavailable) {
		t.Fatalf("an unreadable bot answered %v, want unavailable", err)
	}
}

func TestNoConnectedBotRefusesEveryToken(t *testing.T) {
	h := newHarness(t)
	h.edge.bots = fakeBots{err: capture.ErrNoConnection}
	if _, err := h.authenticate(t, h.token(t, nil)); !errors.Is(err, ErrRejected) {
		t.Fatalf("a call with no connected bot answered %v, want a rejection", err)
	}
}

func TestARevokedPassportOrAChangedEmailIsRefused(t *testing.T) {
	h := newHarness(t)
	for _, refusal := range []error{apperrors.ErrNotFound, apperrors.ErrConflict} {
		h.seats.err = refusal
		if _, err := h.authenticate(t, h.token(t, nil)); !errors.Is(err, ErrRejected) {
			t.Fatalf("identity's refusal %v answered %v, want a rejection", refusal, err)
		}
	}
}

func TestACallerWithNoMemberMayOnlyListTools(t *testing.T) {
	h := newHarness(t)
	h.seats.err = identity.ErrNoSeat
	ctx, err := h.authenticate(t, h.token(t, nil))
	if err != nil {
		t.Fatalf("a seatless caller was refused before the method was known: %v", err)
	}
	if _, acts := principal.Actor(ctx); acts {
		t.Fatal("a seatless caller acts as somebody")
	}
	if !agents.IsAppCall(ctx) {
		t.Fatal("a seatless caller carries no app surface")
	}
}

func TestTheNoSeatRefusalIs403(t *testing.T) {
	rec := httptest.NewRecorder()
	noSeat{}.WriteRefusal(rec, httptest.NewRequest(http.MethodPost, "/mcp", nil))
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), NoSeatCode) {
		t.Fatalf("the refusal answered %d %s, want 403 %s", rec.Code, rec.Body.String(), NoSeatCode)
	}
}

func TestOnlyACallTokenTakesTheDispactBranch(t *testing.T) {
	h := newHarness(t)
	var passed []string
	next := func(r *http.Request) (context.Context, error) {
		passed = append(passed, r.Header.Get("Authorization"))
		return r.Context(), nil
	}
	authenticate := h.edge.Or(h.seats, next)
	for _, bearer := range []string{"mgp_abc", h.token(t, nil)} {
		r := httptest.NewRequest(http.MethodPost, "/mcp", nil)
		r.Header.Set("Authorization", "Bearer "+bearer)
		if _, err := authenticate(r); err != nil {
			t.Fatalf("bearer %.8s…: %v", bearer, err)
		}
	}
	if len(passed) != 1 || passed[0] != "Bearer mgp_abc" {
		t.Fatalf("the passport path saw %v, want only the passport", passed)
	}
	if len(h.seats.asked) != 1 {
		t.Fatal("the call token never reached the Dispact branch")
	}
	var unwired *Edge
	r := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	r.Header.Set("Authorization", "Bearer "+h.token(t, nil))
	if _, err := unwired.Or(h.seats, next)(r); err != nil || len(passed) != 2 {
		t.Fatal("an install with no Dispact edge did not hand the token to the passport path")
	}
}

func TestAnAppCallIsPacedByItsPassport(t *testing.T) {
	h := newHarness(t)
	ctx, err := h.authenticate(t, h.token(t, nil))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := PaceKey(ctx, "digest"), "app:"+h.seats.agent.PassportID.String(); got != want {
		t.Fatalf("an app call is paced under %q, want %q", got, want)
	}
	if got := PaceKey(t.Context(), "digest"); got != "digest" {
		t.Fatalf("a passport call is paced under %q, want its own key", got)
	}
}
