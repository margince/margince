// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration && bench

package integration

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/homecache"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Home's fixed-profile API budgets, per read, as p95. A cold read is the first
// answer on a freshly booted app whose connections hold no plan.
const (
	homeColdBudget = 500 * time.Millisecond
	homeWarmBudget = 250 * time.Millisecond
	// homeColdRounds is how many boots the cold p95 is taken over.
	homeColdRounds = 10
)

// homeBriefDelayEnv plants a delay on every /v1/brief the owner's client sends,
// so a run can prove the check turns red. Only this test binary reads it.
const homeBriefDelayEnv = "MARGINCE_HOME_BUDGET_BRIEF_DELAY"

// The cold and warm passes over Home's reads. Their flow names are Home's own,
// so no DailyKnownIssues entry can ever cover one of their rows.
var (
	homeColdFlow = dailyFlow{Name: "home_cold", ID: "HOME-API", Budget: homeColdBudget, PerCall: true, Calls: homeCalls}
	homeWarmFlow = dailyFlow{Name: "home_warm", ID: "HOME-API", Budget: homeWarmBudget, PerCall: true, Calls: homeCalls}
)

// TestHomeBudget times Home's reads for the heaviest owner of a small fixed
// corpus, cold and warm, and fails any read over its budget. Home has no
// known-issue allowance: a slow worklist fails it like any other read.
func TestHomeBudget(t *testing.T) {
	assertHomeReadsCovered(t)
	e := apptest.SetupApp(t)
	if mode := e.Pool.Config().ConnConfig.DefaultQueryExecMode; mode != pgx.QueryExecModeCacheStatement {
		t.Fatalf("the app pool prepares statements in mode %q, want cache_statement: run through make home-budget", mode)
	}
	e.BootstrapWorkspace(t)
	e.DescribeCompany(t)
	start := time.Now()
	profile := seedHomeProfile(t, e, createDailySeats(t, e))
	t.Logf("home-budget: seed %s", time.Since(start).Round(time.Millisecond))

	in := dailyInput{Seat: withPlantedBriefDelay(t, profile.Owner)}
	start = time.Now()
	rows := runHomeCold(t, e, in)
	rows = append(rows, runDailyFlow(t, e, homeWarmFlow, in)...)
	t.Logf("home-budget: measure %s", time.Since(start).Round(time.Millisecond))

	results := homeRequired(rows, homeCalls(in))
	for _, r := range results {
		t.Logf("home-budget: row %s p95=%dms budget=%dms samples=%d 5xx=%d 422=%d",
			r.Row, r.P95.Milliseconds(), r.Budget.Milliseconds(), r.Samples, r.Status5xx, r.Status422)
	}
	if failures := HomeBudgetFailures(results); len(failures) > 0 {
		t.Fatalf("Home is over its budget:\n%s", strings.Join(failures, "\n"))
	}
}

// homePlannedSamples is how many samples each pass takes per read: one per
// cold boot, and the daily sampler's count warm.
var homePlannedSamples = map[string]int{homeColdFlow.Name: homeColdRounds, homeWarmFlow.Name: dailySamples}

// HomeBudgetFailures names every reason the Home check fails. It judges each
// row on its own p95 with the daily rule, and never asks DailyKnownIssues.
func HomeBudgetFailures(results []DailyResult) []string {
	if len(results) == 0 {
		return []string{"no Home read was measured"}
	}
	var failures []string
	for _, r := range results {
		planned, known := homePlannedSamples[r.Flow]
		switch verdict := judgeP95(r.P95, r.Budget, r.Samples); {
		case !known:
			failures = append(failures, fmt.Sprintf("%s: flow %q is neither Home pass", r.Row, r.Flow))
		case verdict == DailyNoData:
			failures = append(failures, fmt.Sprintf("%s: no samples, so no p95 to judge", r.Row))
		case r.Samples != planned:
			failures = append(failures, fmt.Sprintf("%s: %d samples, want %d", r.Row, r.Samples, planned))
		case verdict == DailyOver:
			failures = append(failures, fmt.Sprintf("%s: p95 %d ms is at or over its %d ms budget",
				r.Row, r.P95.Milliseconds(), r.Budget.Milliseconds()))
		}
		if r.Status5xx > 0 {
			failures = append(failures, fmt.Sprintf("%s: %d server errors", r.Row, r.Status5xx))
		}
		if r.Status422 > 0 {
			failures = append(failures, fmt.Sprintf("%s: refused %d times with 422", r.Row, r.Status422))
		}
	}
	return failures
}

// homeRequired keeps the rows the judge must see: every read in both passes,
// except an optional one the corpus has nothing behind. A required read with
// no row at all comes back with zero samples, so the judge fails it.
func homeRequired(rows []dailyRow, calls []dailyCall) []DailyResult {
	var required []DailyResult
	for _, flow := range []dailyFlow{homeColdFlow, homeWarmFlow} {
		for _, call := range calls {
			name := dailyRowName(flow.Name, call)
			at := slices.IndexFunc(rows, func(r dailyRow) bool { return r.Measurement.Name == name })
			switch {
			case at >= 0 && (rows[at].Result.Samples > 0 || !call.Optional):
				required = append(required, rows[at].Result)
			case at < 0:
				required = append(required, DailyResult{Flow: flow.Name, Row: name, Budget: flow.Budget})
			}
		}
	}
	return required
}

// homeCachedReads are the Home reads home_read_cache keeps, by their call's name.
// That cache lives in the database, so a reboot alone leaves it filled.
var homeCachedReads = map[string]homecache.Read{"worklist": homecache.Worklist, "magic": homecache.Magic}

// runHomeCold boots the app afresh homeColdRounds times and times each read's
// first answer on it. The probe on the warm app decides which reads are timed.
func runHomeCold(t *testing.T, e *apptest.AppEnv, in dailyInput) []dailyRow {
	t.Helper()
	f, s := homeColdFlow, in.Seat
	var rows []dailyRow
	var timed []dailyCall
	var tallies []dailyTally
	for _, call := range f.Calls(in) {
		hit, absent, tally := probeCalls(t, e, f, s, []dailyCall{call})
		rows = append(rows, dailyAbsentRows(t.Log, f, f.ID, f.Name, s, f.Budget, absent, false)...)
		if len(hit) == 1 {
			timed, tallies = append(timed, call), append(tallies, tally)
		}
	}
	samples := make([][]time.Duration, len(timed))
	for range homeColdRounds {
		since := forgetHomeAnswers(t, e, s)
		booted := coldBoot(t, e)
		for i, call := range timed {
			status, body, elapsed := s.Get(t, booted, call.Path)
			if err := f.admit(dailyResponse{call, status, body}, &tallies[i]); err != nil {
				t.Fatalf("cold %s: %v", call.Name, err)
			}
			samples[i] = append(samples[i], elapsed)
		}
		booted.Close()
		if missed := cacheHits(t, e, s, timed, since); len(missed) > 0 {
			t.Fatalf("cold reads %v were served from home_read_cache, so they timed no assembly", missed)
		}
	}
	for i, call := range timed {
		spec := dailyRowSpec{flow: f.Name, id: f.ID, name: dailyRowName(f.Name, call), budget: f.Budget, gated: true}
		rows = append(rows, dailyMeasuredRow(t, s, spec, samples[i], tallies[i]))
	}
	return rows
}

// forgetHomeAnswers ends the seat's kept Home answers through the cache's own
// Forget, and returns the database clock from before it.
func forgetHomeAnswers(t *testing.T, e *apptest.AppEnv, s Seat) time.Time {
	t.Helper()
	ctx := context.Background()
	var since time.Time
	if err := e.Owner.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&since); err != nil {
		t.Fatalf("reading the database clock: %v", err)
	}
	ws := principal.WithWorkspaceID(ctx, apptest.InstallationWorkspaceUUID(ctx, t, e.Pool))
	if err := homecache.New(e.Pool).Forget(ws, []ids.UUID{ids.MustParse(s.UserID)}); err != nil {
		t.Fatalf("ending %s's kept Home answers: %v", s.Name, err)
	}
	return since
}

// cacheHits names each timed read the cache keeps that stored no fresh answer
// since then. A hit stores nothing, so such a read was served from the cache.
func cacheHits(t *testing.T, e *apptest.AppEnv, s Seat, timed []dailyCall, since time.Time) []string {
	t.Helper()
	var hits []string
	for _, call := range timed {
		read, cached := homeCachedReads[call.Name]
		if !cached {
			continue
		}
		var fresh bool
		if err := e.Owner.QueryRow(context.Background(), `SELECT EXISTS (SELECT 1 FROM home_read_cache
		      WHERE reader_id = $1 AND read = $2 AND computed_at >= $3)`, s.UserID, string(read), since).Scan(&fresh); err != nil {
			t.Fatalf("reading what the cold %s stored: %v", call.Name, err)
		}
		if !fresh {
			hits = append(hits, call.Name)
		}
	}
	return hits
}

// coldBoot opens a new app and pool over the seeded database. One connection
// is dialled first, so a first read never times the dial. Every idle
// connection is then cleared, so no plan or prepared statement survives.
func coldBoot(t *testing.T, e *apptest.AppEnv) *apptest.AppEnv {
	t.Helper()
	ctx := context.Background()
	booted := e.Reboot(t)
	conn, err := booted.Pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("dialling the rebooted pool: %v", err)
	}
	conn.Release()
	for _, idle := range booted.Pool.AcquireAllIdle(ctx) {
		_, err := idle.Conn().PgConn().Exec(ctx, "DISCARD ALL").ReadAll()
		if err == nil {
			// The server dropped its statements; pgx's caches must forget them too.
			err = idle.Conn().DeallocateAll(ctx)
		}
		idle.Release()
		if err != nil {
			t.Fatalf("clearing a rebooted connection: %v", err)
		}
	}
	return booted
}

// homeReadsDeclaration is HOME_READS in the frontend's Home judge, the reads the
// browser check waits for.
var homeReadsDeclaration = regexp.MustCompile(`export const HOME_READS = \[([^\]]*)\] as const;`)

// assertHomeReadsCovered fails when the browser check waits for a read this
// check does not time.
func assertHomeReadsCovered(t *testing.T) {
	t.Helper()
	path := filepath.Join(RepositoryRoot(t), "frontend", "src", "testing", "homejourney.judge.ts")
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the frontend's Home reads: %v", err)
	}
	for _, gap := range homeReadsGaps(source, callNames(homeCalls(dailyInput{}))) {
		t.Errorf("%s: %s", path, gap)
	}
}

// homeReadsGaps names what is wrong with the HOME_READS declaration in source,
// against the read names this check times.
func homeReadsGaps(source []byte, timed []string) []string {
	declared := homeReadsDeclaration.FindSubmatch(source)
	if declared == nil {
		return []string{"declares no HOME_READS list this check can read"}
	}
	var reads, gaps []string
	for _, read := range strings.Split(string(declared[1]), ",") {
		if read = strings.Trim(strings.TrimSpace(read), `"`); read != "" {
			reads = append(reads, read)
		}
	}
	if len(reads) == 0 {
		return []string{"HOME_READS is empty, so the browser check waits for no read"}
	}
	for _, read := range reads {
		if !slices.Contains(timed, read) {
			gaps = append(gaps, fmt.Sprintf("the browser check waits for Home read %q, which homeCalls does not time", read))
		}
	}
	return gaps
}

// withPlantedBriefDelay returns the seat unchanged unless homeBriefDelayEnv
// names a duration; then every /v1/brief it sends waits that long first.
func withPlantedBriefDelay(t *testing.T, s Seat) Seat {
	t.Helper()
	raw := os.Getenv(homeBriefDelayEnv)
	if raw == "" {
		return s
	}
	delay, err := time.ParseDuration(raw)
	if err != nil || delay <= 0 {
		t.Fatalf("%s=%q is not a positive duration", homeBriefDelayEnv, raw)
	}
	t.Logf("home-budget: planted %s on every /v1/brief", delay)
	client := *s.Client
	client.Transport = delayedBrief{next: s.Client.Transport, delay: delay}
	s.Client = &client
	return s
}

// delayedBrief holds a /v1/brief request for its delay before sending it.
type delayedBrief struct {
	next  http.RoundTripper
	delay time.Duration
}

func (d delayedBrief) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Path == "/v1/brief" {
		timer := time.NewTimer(d.delay)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-req.Context().Done():
			return nil, req.Context().Err()
		}
	}
	return d.next.RoundTrip(req)
}
