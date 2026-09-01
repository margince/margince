// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// originClock lets the TTL be tested without waiting a minute for it.
type originClock struct{ at time.Time }

func (c *originClock) now() time.Time      { return c.at }
func (c *originClock) add(d time.Duration) { c.at = c.at.Add(d) }

func TestAnAnsweringOriginReadsAsReachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	clock := &originClock{at: time.Now()}
	status := newPublicOriginProbe(srv.URL, newOriginProbeClient(), clock.now).Status(context.Background())
	if status.Reachable == nil || !*status.Reachable {
		t.Fatalf("reachable = %v, want true (detail %q)", status.Reachable, status.Detail)
	}
	if status.CheckedAt == nil {
		t.Error("a probe that ran reported no time")
	}
}

// A dev stack points the origin at Vite, which answers the SPA fallback
// rather than a health endpoint. That is still a reachable origin: the
// question is whether the host answers over this scheme.
func TestAnSPAFallbackStillCountsAsReachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	clock := &originClock{at: time.Now()}
	status := newPublicOriginProbe(srv.URL, newOriginProbeClient(), clock.now).Status(context.Background())
	if status.Reachable == nil || !*status.Reachable {
		t.Errorf("reachable = %v, want true for a 404", status.Reachable)
	}
}

func TestAnOriginThatDoesNotAnswerReadsAsUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	origin := srv.URL
	srv.Close() // nothing listens now

	clock := &originClock{at: time.Now()}
	status := newPublicOriginProbe(origin, newOriginProbeClient(), clock.now).Status(context.Background())
	if status.Reachable == nil || *status.Reachable {
		t.Errorf("reachable = %v, want false", status.Reachable)
	}
	if status.Detail == "" {
		t.Error("an unreachable origin reported no reason")
	}
}

// A screen that polls must not become traffic against the ingress.
func TestTheAnswerIsCachedForItsTTL(t *testing.T) {
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	clock := &originClock{at: time.Now()}
	probe := newPublicOriginProbe(srv.URL, newOriginProbeClient(), clock.now)

	probe.Status(context.Background())
	probe.Status(context.Background())
	if got := hits.Load(); got != 1 {
		t.Errorf("the origin was asked %d times inside one TTL, want 1", got)
	}
	clock.add(originProbeTTL + time.Second)
	probe.Status(context.Background())
	if got := hits.Load(); got != 2 {
		t.Errorf("the origin was asked %d times after the TTL lapsed, want 2", got)
	}
}

// A redirect is not followed: the question is what THIS origin answers,
// and following one would dial wherever the answer pointed.
func TestTheProbeNeverFollowsARedirect(t *testing.T) {
	var elsewhere atomic.Int64
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		elsewhere.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/v1/public/preferences/tok", http.StatusFound)
	}))
	defer srv.Close()

	clock := &originClock{at: time.Now()}
	newPublicOriginProbe(srv.URL, newOriginProbeClient(), clock.now).Status(context.Background())
	if got := elsewhere.Load(); got != 0 {
		t.Errorf("the probe followed a redirect %d time(s); it must not dial where an answer points", got)
	}
}

// The reported detail is for an operator's screen, so it must never
// carry a path — the origin is the only address in it.
func TestTheReportedDetailCarriesNoPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	clock := &originClock{at: time.Now()}
	status := newPublicOriginProbe(srv.URL, newOriginProbeClient(), clock.now).Status(context.Background())
	if status.Detail != "http 200" {
		t.Errorf("detail = %q, want just the status", status.Detail)
	}
}

// The bug this pins: connectorHandlers is REPLACED wholesale by more than
// one option, in an order no single option controls, so a literal that
// captured the probe directly captured whatever was there at the time —
// usually nil — and the row silently never appeared. Reading it through a
// Server method means every literal answers once the origin is wired,
// whatever order the options ran in.
func TestTheOriginStatusSurvivesAHandlerRebuild(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	s := &Server{}
	s.connectorHandlers.publicOrigin = s.originStatus
	// The origin is configured AFTER the handler was built, which is the
	// order that used to lose it.
	s.originProbe = newPublicOriginProbe(srv.URL, newOriginProbeClient(), time.Now)

	status := s.connectorHandlers.publicOrigin(context.Background())
	if status.Origin != srv.URL {
		t.Errorf("origin = %q, want %q — the handler lost the probe", status.Origin, srv.URL)
	}
	if status.Reachable == nil || !*status.Reachable {
		t.Errorf("reachable = %v, want true", status.Reachable)
	}
}

// A role with no origin configured answers an empty status rather than
// panicking, so the screen renders no row instead of failing.
func TestNoConfiguredOriginAnswersAnEmptyStatus(t *testing.T) {
	s := &Server{}
	if got := s.originStatus(context.Background()); got.Origin != "" || got.Reachable != nil {
		t.Errorf("status = %+v, want empty", got)
	}
}
