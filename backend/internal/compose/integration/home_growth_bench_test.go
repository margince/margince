// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration && bench

package integration

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/testdb"
)

// The settings a run may change. The defaults are the small run a laptop
// finishes in minutes; the nightly lane raises the steps.
const (
	growthStepsEnv   = "MARGINCE_BENCH_GROWTH_STEPS"
	growthScaleEnv   = "MARGINCE_BENCH_GROWTH_SCALE"
	growthDelayEnv   = "MARGINCE_BENCH_GROWTH_DELAY"
	growthHorizonEnv = "MARGINCE_BENCH_GROWTH_HORIZON"
	growthStretchEnv = "MARGINCE_BENCH_GROWTH_CPU_STRETCH"
)

// growthBreach starts the failure of a measured budget, so the nightly lane can
// tell a breach from a run that never got to measure.
const growthBreach = "home-growth: breach"

const (
	growthColdRounds  = 3
	growthWarmSamples = 5
	// growthRepeatOpens are the opens after the first, which a kept answer serves.
	growthRepeatOpens = 4
)

// growthBudgets come from the Worklist PoC's measurements on 8 years of bench
// mail: 2.4 s of server CPU per read with open cases, 314 MB per connection.
var growthBudgets = growthLimits{
	Line:             10 * time.Second,
	WarmServer:       2400 * time.Millisecond,
	ConnectionMemory: 314 << 20,
	CacheHits:        0.75,
}

// growthRead is one Home read, sent by the seat that opens it.
type growthRead struct {
	Name string
	Seat Seat
	Call dailyCall
}

// TestHomeGrowth measures Home's reads as the mail grows by a year per step.
// Every round trip waits a fixed delay. It fits T ≈ k·W + r·R + A per read, and
// fails when a read is predicted to reach 10 s within the horizon.
func TestHomeGrowth(t *testing.T) {
	if os.Getenv(database.SQLCensusDirEnv) != "" {
		t.Fatal("SQL file capture changes latency; unset " + database.SQLCensusDirEnv + " for this bench")
	}
	steps, scale := growthEnvInt(t, growthStepsEnv, 2, 2), growthEnvFloat(t, growthScaleEnv, homeBudgetScale, 0)
	limits := growthBudgets
	limits.Horizon = growthEnvInt(t, growthHorizonEnv, 4, 0)
	assume := growthAssumptions{
		RoundTrip: growthEnvDuration(t, growthDelayEnv, 3*time.Millisecond),
		Stretch:   growthEnvFloat(t, growthStretchEnv, 1, 1),
	}
	t.Logf("home-growth: steps=%d scale=%g round-trip=%s cpu-stretch=%g horizon=%d", steps, scale, assume.RoundTrip, assume.Stretch, limits.Horizon)
	e := apptest.SetupApp(t)
	if mode := e.Pool.Config().ConnConfig.DefaultQueryExecMode; mode != pgx.QueryExecModeCacheStatement {
		t.Fatalf("the app pool prepares statements in mode %q, want cache_statement: run through make bench-growth", mode)
	}
	e.BootstrapWorkspace(t)
	e.DescribeCompany(t)
	seats := createDailySeats(t, e)
	profile := seedHomeProfile(t, e, seats, scale)
	reads := growthReads(t, e, profile.Owner, growthManagerOf(t, seats, profile.Owner))

	series := make([]growthSeries, len(reads))
	for i, read := range reads {
		series[i].Read = read.Name
	}
	other := growthOther{ConnectionMemory: map[int]int64{}, Cache: map[int]growthCache{}}
	for years := 1; years <= steps; years++ {
		if years > 1 {
			growHistory(t, e, years-1)
		}
		cold := measureGrowthCold(t, e, reads, assume.RoundTrip)
		warm := measureGrowthWarm(t, e, reads, assume.RoundTrip, years, &other)
		for i := range reads {
			step := growthStep{Years: years, Cold: cold[i], Warm: warm[i]}
			series[i].Steps = append(series[i].Steps, step)
			logGrowthStep(t, reads[i].Name, step)
		}
	}
	for _, s := range series {
		logGrowthFit(t, s, assume, limits.Line)
	}
	t.Logf("home-growth: cache per step %v", other.Cache)
	if failures := growthFailures(series, other, limits, assume); len(failures) > 0 {
		t.Fatalf("%s\n%s", growthBreach, strings.Join(failures, "\n"))
	}
}

// growthReads are Home's reads for the heaviest owner, and their manager's team
// view: the largest Worklist read a member opens. A read the corpus has
// nothing behind is left out, as the Home check leaves it out.
func growthReads(t *testing.T, e *apptest.AppEnv, owner, manager Seat) []growthRead {
	t.Helper()
	var reads []growthRead
	timed, _, _ := probeCalls(t, e, homeWarmFlow, owner, homeCalls(dailyInput{}))
	for _, call := range timed {
		reads = append(reads, growthRead{Name: "rep_" + call.Name, Seat: owner, Call: call})
	}
	team := dailyCall{Name: "team", Path: worklistDay + "team&view=today"}
	if timed, _, _ := probeCalls(t, e, homeWarmFlow, manager, []dailyCall{team}); len(timed) != 1 {
		t.Fatalf("the manager's team view has nothing to time")
	}
	return append(reads, growthRead{Name: "manager_team", Seat: manager, Call: team})
}

// growthManagerOf is the manager who leads the owner's team: Managers[i] leads
// Teams[i], and the reps fill the teams in order.
func growthManagerOf(t *testing.T, seats Seats, owner Seat) Seat {
	t.Helper()
	at := slices.IndexFunc(seats.Reps, func(s Seat) bool { return s.UserID == owner.UserID })
	if at < 0 {
		t.Fatalf("the heaviest owner %s holds no rep seat", owner.Name)
	}
	return seats.Managers[at/dailyRepsPerTeam]
}

// meterGrowthPool holds every round trip for the delay and times the server.
// Idle pings are off, so the meter counts the reads' own trips.
func meterGrowthPool(t *testing.T, cfg *pgxpool.Config, delay time.Duration) *testdb.RoundTripMeter {
	t.Helper()
	cfg.ShouldPing = func(context.Context, pgxpool.ShouldPingParams) bool { return false }
	cfg.ConnConfig.TLSConfig = nil
	cfg.ConnConfig.Fallbacks = nil
	return testdb.MeterRoundTrips(t, cfg, delay)
}

// measureGrowthCold times each read on a freshly booted app, with no prepared
// plan and no kept answer, as the first open after a restart.
func measureGrowthCold(t *testing.T, e *apptest.AppEnv, reads []growthRead, delay time.Duration) [][]growthSample {
	t.Helper()
	samples := make([][]growthSample, len(reads))
	for range growthColdRounds {
		for i, read := range reads {
			var meter *testdb.RoundTripMeter
			booted := coldBootWith(t, e, func(cfg *pgxpool.Config) { meter = meterGrowthPool(t, cfg, delay) })
			if plans := preparedPlans(t, booted); len(plans) != 0 {
				booted.Close()
				t.Fatalf("cold %s already holds %d prepared plans", read.Name, len(plans))
			}
			since := forgetHomeAnswers(t, e, read.Seat)
			samples[i] = append(samples[i], timeGrowthRead(t, booted, read, meter))
			booted.Close()
			if missed := cacheHits(t, e, read.Seat, []dailyCall{read.Call}, since); len(missed) > 0 {
				t.Fatalf("cold read %s was served from home_read_cache, so it timed no assembly", read.Name)
			}
		}
	}
	return samples
}

// measureGrowthWarm times each read assembling on a warmed app, then records
// what its connections hold and how often a repeat open is a kept answer.
func measureGrowthWarm(t *testing.T, e *apptest.AppEnv, reads []growthRead, delay time.Duration, years int, other *growthOther) [][]growthSample {
	t.Helper()
	var meter *testdb.RoundTripMeter
	warm := e.RebootWith(t, func(cfg *pgxpool.Config) { meter = meterGrowthPool(t, cfg, delay) })
	defer warm.Close()
	for _, seat := range growthSeats(reads) {
		var calls []dailyCall
		for _, read := range reads {
			if read.Seat.UserID == seat.UserID {
				calls = append(calls, read.Call)
			}
		}
		warmHomePlans(t, warm, seat, calls)
	}
	samples := make([][]growthSample, len(reads))
	for range growthWarmSamples {
		for i, read := range reads {
			forgetHomeAnswers(t, e, read.Seat)
			samples[i] = append(samples[i], timeGrowthRead(t, warm, read, meter))
		}
	}
	held, largest, measured := growthConnectionMemory(t, e, warm)
	if measured {
		other.ConnectionMemory[years] = held
	}
	hits, opens := growthRepeatOpenHits(t, e, warm, reads[0].Seat)
	other.Cache[years] = growthCache{Hits: hits, Opens: opens}
	t.Logf("home-growth: year=%d connection-memory=%dMB (%s) cache=%d/%d", years, held>>20, largest, hits, opens)
	return samples
}

func growthSeats(reads []growthRead) []Seat {
	var seats []Seat
	for _, read := range reads {
		if !slices.ContainsFunc(seats, func(s Seat) bool { return s.UserID == read.Seat.UserID }) {
			seats = append(seats, read.Seat)
		}
	}
	return seats
}

// timeGrowthRead sends one read and takes what the meter saw during it.
func timeGrowthRead(t *testing.T, env *apptest.AppEnv, read growthRead, meter *testdb.RoundTripMeter) growthSample {
	t.Helper()
	trips, server := meter.Trips.Load(), meter.ServerTime()
	status, body, elapsed := read.Seat.Get(t, env, read.Call.Path)
	if status != http.StatusOK {
		t.Fatalf("%s answered %d: %s", read.Name, status, clipBody(body))
	}
	return growthSample{Elapsed: elapsed, Server: meter.ServerTime() - server, Trips: meter.Trips.Load() - trips}
}

// growthMemoryReader lets the app role read its own backend's memory, which
// Postgres shows only to a privileged role. It lives in the throwaway bench
// database and goes with it.
const growthMemoryReader = `CREATE OR REPLACE FUNCTION public.bench_growth_backend_memory()
      RETURNS TABLE (total bigint, largest text)
      LANGUAGE sql SECURITY DEFINER SET search_path = pg_catalog
      AS $$ SELECT sum(total_bytes)::bigint,
                   (SELECT string_agg(name || '=' || (bytes >> 20) || 'MB', ' ' ORDER BY bytes DESC)
                      FROM (SELECT name, sum(total_bytes)::bigint AS bytes FROM pg_backend_memory_contexts
                             GROUP BY name ORDER BY bytes DESC LIMIT 3) top)
              FROM pg_backend_memory_contexts $$`

// growthConnectionMemory is the most memory any idle connection of the pool
// holds, and its three largest kinds: its plans and caches, which outlive the
// read. The peak during a read is not visible from outside the server process.
func growthConnectionMemory(t *testing.T, e, env *apptest.AppEnv) (int64, string, bool) {
	t.Helper()
	ctx := context.Background()
	if _, err := e.Owner.Exec(ctx, growthMemoryReader); err != nil {
		t.Fatalf("installing the bench's memory reader: %v", err)
	}
	idle := env.Pool.AcquireAllIdle(ctx)
	defer func() {
		for _, conn := range idle {
			conn.Release()
		}
	}()
	var most int64
	var largest string
	for _, conn := range idle {
		var held int64
		var kinds string
		if err := conn.QueryRow(ctx, `SELECT total, largest FROM public.bench_growth_backend_memory()`,
			pgx.QueryExecModeSimpleProtocol).Scan(&held, &kinds); err != nil {
			t.Fatalf("reading a connection's memory: %v", err)
		}
		if held > most {
			most, largest = held, kinds
		}
	}
	return most, largest, len(idle) > 0
}

// growthRepeatOpenHits opens the seat's cached Home reads once to fill the
// cache, then again growthRepeatOpens times, and counts the kept answers.
func growthRepeatOpenHits(t *testing.T, e, env *apptest.AppEnv, seat Seat) (hits, opens int) {
	t.Helper()
	forgetHomeAnswers(t, e, seat)
	var cached []dailyCall
	for _, call := range homeCalls(dailyInput{}) {
		if _, kept := homeCachedReads[call.Name]; kept {
			cached = append(cached, call)
		}
	}
	open := func() {
		for _, call := range cached {
			if status, body, _ := seat.Get(t, env, call.Path); status != http.StatusOK {
				t.Fatalf("reopening %s answered %d: %s", call.Name, status, clipBody(body))
			}
		}
	}
	open()
	count := func(outcome string) int {
		total := 0
		for _, call := range cached {
			total += homeCacheReads(t, homeCachedReads[call.Name], outcome)
		}
		return total
	}
	hit, miss := count("hit"), count("miss")
	for range growthRepeatOpens {
		open()
	}
	hits = count("hit") - hit
	return hits, hits + count("miss") - miss
}

func logGrowthStep(t *testing.T, name string, step growthStep) {
	t.Helper()
	coldT, coldW, coldR := growthMedians(step.Cold)
	warmT, warmW, warmR := growthMedians(step.Warm)
	t.Logf("home-growth: year=%d read=%s cold p50=%dms p90=%dms db=%dms trips=%.0f warm p50=%dms db=%dms trips=%.0f",
		step.Years, name, coldT.Milliseconds(), growthP90(step.Cold).Milliseconds(), coldW.Milliseconds(), coldR,
		warmT.Milliseconds(), warmW.Milliseconds(), warmR)
}

// logGrowthFit prints the read's fitted model and its prediction for the next
// year. With three or more steps it also shows how well the others foresaw the last.
func logGrowthFit(t *testing.T, s growthSeries, a growthAssumptions, line time.Duration) {
	t.Helper()
	model, err := fitGrowthModel(s.Steps, a.RoundTrip)
	if err != nil {
		t.Logf("home-growth: fit read=%s: %v", s.Read, err)
		return
	}
	last := s.Steps[len(s.Steps)-1]
	crossing := "never within " + strconv.Itoa(growthSearchYears) + " years"
	if years, crosses := model.crossing(line, a); crosses {
		crossing = fmt.Sprintf("at %d years", years)
	}
	t.Logf("home-growth: fit read=%s W=%.3f%+.3f/yr s R=%.0f%+.1f/yr A=%.3f%+.3f/yr s next(%d yr)=%dms reaches %s %s",
		s.Read, model.Work.At, model.Work.PerYear, model.Trips.At, model.Trips.PerYear, model.Rest.At, model.Rest.PerYear,
		last.Years+1, model.predict(float64(last.Years+1), a).Milliseconds(), line, crossing)
	if len(s.Steps) < 3 {
		return
	}
	earlier, err := fitGrowthModel(s.Steps[:len(s.Steps)-1], a.RoundTrip)
	if err != nil {
		t.Logf("home-growth: check read=%s: %v", s.Read, err)
		return
	}
	measured, _, _ := growthMedians(last.Cold)
	t.Logf("home-growth: check read=%s year=%d predicted=%dms measured=%dms",
		s.Read, last.Years, earlier.predict(float64(last.Years), a).Milliseconds(), measured.Milliseconds())
}

func growthEnvInt(t *testing.T, name string, fallback, least int) int {
	t.Helper()
	raw := os.Getenv(name)
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < least {
		t.Fatalf("%s=%q is not a whole number of at least %d", name, raw, least)
	}
	return n
}

func growthEnvFloat(t *testing.T, name string, fallback, least float64) float64 {
	t.Helper()
	raw := os.Getenv(name)
	if raw == "" {
		return fallback
	}
	f, err := strconv.ParseFloat(raw, 64)
	if err != nil || f < least || f <= 0 {
		t.Fatalf("%s=%q is not a number above zero and at least %g", name, raw, least)
	}
	return f
}

func growthEnvDuration(t *testing.T, name string, fallback time.Duration) time.Duration {
	t.Helper()
	raw := os.Getenv(name)
	if raw == "" {
		return fallback
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d < 0 {
		t.Fatalf("%s=%q is not a duration of zero or more", name, raw)
	}
	return d
}
