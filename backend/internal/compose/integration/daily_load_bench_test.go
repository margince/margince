// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration && bench

package integration

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/search"
)

// The morning load's pacing: background polls, a cheap route timed four times
// as often, and a bound on the whole start, which repeats until the cheap route
// has dailySamples samples.
const (
	dailyPollEvery  = 2 * time.Second
	dailyCheapEvery = 500 * time.Millisecond
	dailyLoadBound  = 5 * time.Minute
)

// morningLoad is what the whole team's start did to a cheap request and to the pool.
type morningLoad struct {
	CheapP50, CheapP95, CheapP99 time.Duration
	CheapSamples                 int
	S5xx, S422                   int
	PoolWaitTotal, PoolWaitMax   time.Duration
	Acquires                     int64
	PoolSize                     int32
}

// dailyLoadTally collects what every goroutine of the load saw; any refusal
// other than a counted one is a broken run and is reported after the wait.
type dailyLoadTally struct {
	mu       sync.Mutex
	s5xx     int
	s422     int
	cheap    []time.Duration
	problems []string
	unwired  int
	// errorsBy counts server errors per path, with the first answer, as evidence.
	errorsBy map[string]string
}

// admit counts one answer; a 404 is allowed only on a call whose latency row
// had no data, because the screen meets the same "nothing yet" there, and an
// unwired 501 is counted apart from server faults.
func (l *dailyLoadTally) admit(seat, row string, r dailyResponse, err error, absent map[string]bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	switch {
	case err != nil:
		l.problems = append(l.problems, fmt.Sprintf("%s: %v", seat, err))
	case r.status >= 200 && r.status < 300:
	case unwiredAnswer(r.call.Path, r.status, r.body):
		l.unwired++
	case r.status >= 500, r.status == http.StatusUnprocessableEntity:
		if r.status >= 500 {
			l.s5xx++
		} else {
			l.s422++
		}
		if _, seen := l.errorsBy[r.call.Path]; !seen {
			l.errorsBy[r.call.Path] = fmt.Sprintf("%d: %s", r.status, clipBody(r.body))
		}
	case r.status == http.StatusNotFound && absent[row]:
	default:
		l.problems = append(l.problems, fmt.Sprintf("%s: GET %s answered %d: %s", seat, r.call.Path, r.status, clipBody(r.body)))
	}
}

// dailyLoadCall is one call of the morning journey, under its latency row's name.
type dailyLoadCall struct {
	row  string
	call dailyCall
}

// morningJourney is the start of a day: shell, Worklist, Home, two palette
// searches and two record opens, in the order a user meets them.
func morningJourney(in dailyInput) []dailyLoadCall {
	var journey []dailyLoadCall
	add := func(flow string, calls ...dailyCall) {
		for _, call := range calls {
			journey = append(journey, dailyLoadCall{dailyRowName(flow, call), call})
		}
	}
	add("app_shell", appShellCalls(in)...)
	add("worklist", worklistCalls(in)[0])
	add("home", homeCalls(in)...)
	add("palette_search", paletteCalls(in)[:2]...)
	add("record_open", recordOpenCalls(in)[0], recordOpenCalls(in)[2])
	return journey
}

// runMorningLoad starts every seat's day at once, with the admin's session as
// background traffic never reported as a seat, and times a cheap route on a
// session of its own throughout. Sessions start their journey again until the
// cheap route has its samples. absent names the latency rows with no data.
func runMorningLoad(t *testing.T, e *apptest.AppEnv, s Seats, c DailyCorpus, absent map[string]bool) morningLoad {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), dailyLoadBound)
	defer cancel()
	sessions := append(slices.Concat(s.Reps, s.Managers), Seat{Name: "admin", Role: "admin", Client: e.Client})
	base := newDailyInput(t, e, sessions[0], s.Teams[0], c)
	inputs := make([]dailyInput, len(sessions))
	for i, seat := range sessions {
		inputs[i] = base
		inputs[i].Seat, inputs[i].Team = seat, teamOf(s, seat)
	}
	cheap := cheapRouteSeat(t, e, s.Reps[len(s.Reps)-1])
	tally := &dailyLoadTally{errorsBy: map[string]string{}}
	before := e.Pool.Stat()
	done := make(chan struct{})
	watched := make(chan time.Duration, 1)
	go func() { watched <- watchPoolWait(e.Pool, before, done) }()
	cheapDone := make(chan struct{})
	go func() {
		defer close(cheapDone)
		timeCheapRoute(ctx, e, cheap, tally, done)
	}()

	var wg sync.WaitGroup
	for _, in := range inputs {
		wg.Go(func() {
			for {
				runMorningSession(ctx, e, in, tally, absent)
				if ctx.Err() != nil || tally.cheapSamples() >= dailySamples {
					return
				}
			}
		})
	}
	wg.Wait()
	close(done)
	<-cheapDone
	worst := <-watched
	after := e.Pool.Stat()
	if ctx.Err() != nil {
		t.Errorf("the morning load ran past its %s bound", dailyLoadBound)
	}
	if len(tally.problems) > 0 {
		t.Fatalf("the morning load met %d unexpected answers:\n%s", len(tally.problems), strings.Join(tally.problems, "\n"))
	}
	if tally.unwired > 0 {
		t.Logf("morning load: %d answers were the 501 of an operation this composition does not wire", tally.unwired)
	}
	for path, first := range tally.errorsBy {
		t.Logf("morning load: GET %s answered %s", path, first)
	}
	stats, err := search.MeasureQuery("morning_load_cheap_route", Load1Budget, tally.cheap)
	if err != nil {
		t.Fatalf("the cheap route was never timed during the morning load: %v", err)
	}
	return morningLoad{
		CheapP50: stats.P50, CheapP95: stats.P95, CheapP99: stats.P99, CheapSamples: stats.Samples,
		S5xx: tally.s5xx, S422: tally.s422,
		PoolWaitTotal: after.EmptyAcquireWaitTime() - before.EmptyAcquireWaitTime(),
		PoolWaitMax:   worst,
		Acquires:      after.AcquireCount() - before.AcquireCount(),
		PoolSize:      e.Pool.Config().MaxConns,
	}
}

func (l *dailyLoadTally) cheapSamples() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.cheap)
}

func teamOf(s Seats, seat Seat) string {
	for i, manager := range s.Managers {
		if manager.UserID == seat.UserID {
			return s.Teams[i]
		}
	}
	if i := slices.IndexFunc(s.Reps, func(r Seat) bool { return r.UserID == seat.UserID }); i >= dailyRepsPerTeam {
		return s.Teams[1]
	}
	return s.Teams[0]
}

// cheapRouteSeat signs the seat in again on a client of its own, so the cheap
// route's requests never queue behind that seat's journey on one connection pool.
func cheapRouteSeat(t *testing.T, e *apptest.AppEnv, seat Seat) Seat {
	t.Helper()
	seat.Client = seatClient(t, e)
	mustCall(t, seat.env(e), "POST", "/v1/auth/login", AnyMap{
		"email": seat.Email, "password": dailySeatPassword,
	}, http.StatusOK, nil)
	return seat
}

// runMorningSession is one session's start of day, polling in the background
// as the app shell does until the journey ends.
func runMorningSession(ctx context.Context, e *apptest.AppEnv, in dailyInput, tally *dailyLoadTally, absent map[string]bool) {
	stop := make(chan struct{})
	var polls sync.WaitGroup
	polls.Go(func() { pollMorning(ctx, e, in.Seat, tally, stop) })
	for _, step := range morningJourney(in) {
		status, body, _, err := in.Seat.fetch(ctx, e.TS.URL, step.call.Path)
		tally.admit(in.Seat.Name, step.row, dailyResponse{step.call, status, body}, err, absent)
	}
	close(stop)
	polls.Wait()
}

func pollMorning(ctx context.Context, e *apptest.AppEnv, s Seat, tally *dailyLoadTally, stop <-chan struct{}) {
	ticker := time.NewTicker(dailyPollEvery)
	defer ticker.Stop()
	for {
		for _, call := range []dailyCall{{Name: "ai_activity", Path: dailyAIActivityPath()}, {Name: "connectors", Path: "/v1/connectors"}} {
			status, body, _, err := s.fetch(ctx, e.TS.URL, call.Path)
			tally.admit(s.Name, "poll_"+call.Name, dailyResponse{call, status, body}, err, nil)
		}
		select {
		case <-stop:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// timeCheapRoute times GET /v1/me until the journeys end: the request every
// screen makes first, so its tail is what a slow pool costs everybody.
func timeCheapRoute(ctx context.Context, e *apptest.AppEnv, s Seat, tally *dailyLoadTally, done <-chan struct{}) {
	ticker := time.NewTicker(dailyCheapEvery)
	defer ticker.Stop()
	call := dailyCall{Name: "me", Path: "/v1/me"}
	for {
		status, body, elapsed, err := s.fetch(ctx, e.TS.URL, call.Path)
		tally.admit("cheap route", "cheap_me", dailyResponse{call, status, body}, err, nil)
		if err == nil && status == http.StatusOK {
			tally.mu.Lock()
			tally.cheap = append(tally.cheap, elapsed)
			tally.mu.Unlock()
		}
		select {
		case <-done:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// watchPoolWait returns the worst mean wait of an empty-pool acquire over any
// half-second window: the pool's counters hold totals, never a single wait.
func watchPoolWait(pool *pgxpool.Pool, from *pgxpool.Stat, done <-chan struct{}) time.Duration {
	ticker := time.NewTicker(dailyCheapEvery)
	defer ticker.Stop()
	var worst time.Duration
	last := from
	for {
		select {
		case <-done:
			return max(worst, meanEmptyWait(last, pool.Stat()))
		case <-ticker.C:
			now := pool.Stat()
			worst = max(worst, meanEmptyWait(last, now))
			last = now
		}
	}
}

func meanEmptyWait(from, to *pgxpool.Stat) time.Duration {
	waits := to.EmptyAcquireCount() - from.EmptyAcquireCount()
	if waits <= 0 {
		return 0
	}
	return (to.EmptyAcquireWaitTime() - from.EmptyAcquireWaitTime()) / time.Duration(waits)
}

// morningLoadRow records the load against LOAD-1, for the whole team as one seat.
func morningLoadRow(m morningLoad) dailyRow {
	verdict, issue := JudgeDaily("morning_load", m.CheapP95, Load1Budget, m.CheapSamples)
	row := MeasurementFrom("LOAD-1", "morning_load_cheap_route", m.CheapP50, m.CheapP95, m.CheapP99, Load1Budget, m.CheapSamples)
	row.Seat, row.Flow, row.Verdict, row.KnownIssue = "team", "morning_load", string(verdict), issue
	row.Status5xx, row.Status422 = m.S5xx, m.S422
	row.PoolWaitMs = float64(m.PoolWaitTotal) / float64(time.Millisecond)
	row.PoolWaitMaxMs = float64(m.PoolWaitMax) / float64(time.Millisecond)
	row.Acquires, row.PoolSize = m.Acquires, m.PoolSize
	return dailyRow{
		Measurement: row,
		Result: DailyResult{
			Flow: "morning_load", Seat: "team", Verdict: verdict, Issue: issue,
			Status5xx: m.S5xx, Status422: m.S422,
		},
	}
}

// runFirstLoad boots a fresh handler and pool over the seeded database and
// times the median rep's first Worklist and first palette search on it, before
// anything in the new process has warmed. Postgres's own buffers stay warm.
func runFirstLoad(t *testing.T, e *apptest.AppEnv, s Seats, c DailyCorpus) (worklist, palette time.Duration) {
	t.Helper()
	rebooted := e.Reboot(t)
	defer rebooted.Close()
	// One connection is dialled before timing, so the first request measures the
	// app's cold caches rather than the first connection to Postgres.
	conn, err := rebooted.Pool.Acquire(context.Background())
	if err != nil {
		t.Fatalf("warming the rebooted pool: %v", err)
	}
	conn.Release()
	rep := s.Reps[slices.IndexFunc(s.Reps, func(r Seat) bool { return r.UserID == c.MedianRepID })]
	first := func(path string) time.Duration {
		status, body, elapsed := rep.Get(t, rebooted, path)
		if status != http.StatusOK {
			t.Fatalf("first load after a restart: GET %s answered %d: %s", path, status, clipBody(body))
		}
		return elapsed
	}
	worklist = first("/v1/worklist?scope=mine&filter=all")
	palette = first(paletteCall("contract", "contract").Path)
	return worklist, palette
}

// firstLoadRows records one try each, ungated: a single sample has no p95.
func firstLoadRows(t *testing.T, worklist, palette time.Duration) []dailyRow {
	t.Helper()
	row := func(id, name string, d, budget time.Duration) dailyRow {
		m := MeasurementFrom(id, name, d, d, d, budget, 1)
		m.Seat, m.Verdict = "rep", string(DailyNotGated)
		m.Note = "one connection dialled before timing; the app's caches start empty"
		t.Logf("perfbench [daily]: %s rep first=%s budget=%s samples=1 %s", name, d, budget, DailyNotGated)
		return dailyRow{Measurement: m, Result: DailyResult{Flow: name, Seat: "rep", Verdict: DailyNotGated}}
	}
	return []dailyRow{
		row("PERF-8", "worklist_first_load", worklist, Perf8Budget),
		row("PERF-10", "search_first_load", palette, Perf10Budget),
	}
}
