// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration && bench

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// backlogSizesEnv overrides the candidate counts, comma separated. 100,000 is
// measured on request only: the plan reports it and it blocks nothing.
const backlogSizesEnv = "MARGINCE_BENCH_BACKLOG_SIZES"

// backlogConcurrentReaders is the reader count the stage 0 budgets name. Only two
// reps carry a backlog, so the bench sends that many requests at once from one.
const backlogConcurrentReaders = 10

// backlogCountsOnlyEnv set to 1 skips every timed sample. What each lane cap
// drops and what the walk reaches do not depend on the machine's load.
const backlogCountsOnlyEnv = "MARGINCE_BENCH_BACKLOG_COUNTS_ONLY"

func backlogCountsOnly() bool { return os.Getenv(backlogCountsOnlyEnv) == "1" }

// TestDailyWorklistBacklog measures /worklist over a backlog that grows past
// every lane bound, on a small daily corpus. Per size it times the first page.
// It walks every page to see what scrolling reaches. It reads each seeded
// source to its end through its store's own paging, which is the database cost
// of reading the whole admitted day. Run it through make bench-worklist-backlog.
// It records nothing and gates nothing.
func TestDailyWorklistBacklog(t *testing.T) {
	scale, sizes := backlogScale(t), backlogSizes(t)
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	e.DescribeCompany(t)
	seats := createDailySeats(t, e)
	corpus := seedDailyCorpus(t, e, seats, scale)
	pipeline := apptest.DiscoverSeededPipeline(t, e).PipelineID
	mixed := seats.Reps[slices.IndexFunc(seats.Reps, func(s Seat) bool { return s.UserID == corpus.MedianRepID })]
	dominant := seats.Reps[slices.IndexFunc(seats.Reps[:dailyRepsPerTeam], func(s Seat) bool { return s.UserID != corpus.MedianRepID })]
	manager := seats.Managers[0]
	roster := teamRoster(t, e, seats.Teams[0])
	seeder := dailySeeder{t: t, owner: e.Owner}
	t.Logf("perfbench [backlog]: machine %s, %d CPUs, corpus scale %g; mixed rep %s, tasks-only rep %s, manager %s",
		runtime.GOARCH, runtime.NumCPU(), scale, mixed.Name, dominant.Name, manager.Name)

	seeded := 0
	for _, size := range sizes {
		seeder.timed(fmt.Sprintf("backlog to %d", size), func() {
			seeder.backlog(mixed.UserID, pipeline, backlogMixed, seeded, size)
			seeder.backlog(dominant.UserID, pipeline, backlogDominant, seeded, size)
			seeder.analyze("activity", "activity_link", "activity_participant", "capture_import", "deal")
		})
		seeded = size
		subjects := []backlogSubject{
			{seat: mixed, scope: "mine", owners: []string{mixed.UserID}, mix: backlogMixed.name, seeded: size},
			{seat: dominant, scope: "mine", owners: []string{dominant.UserID}, mix: backlogDominant.name, seeded: size},
			{seat: manager, scope: "team", owners: roster, mix: "both", seeded: 2 * size},
		}
		for _, subject := range subjects {
			measureBacklog(t, e, size, subject)
		}
		if !backlogCountsOnly() {
			measureConcurrentReaders(t, e, size, mixed)
		}
	}
}

// backlogSubject is one reader's queue at one size.
type backlogSubject struct {
	seat       Seat
	scope, mix string
	owners     []string
	// seeded is the backlog this scope holds: both reps' for the team.
	seeded int
}

func (b backlogSubject) path() string { return "/v1/worklist?scope=" + b.scope + "&filter=all" }

// backlogAnswer is the part of a /worklist answer the census reads.
type backlogAnswer struct {
	Queue      []json.RawMessage `json:"queue"`
	NextCursor *string           `json:"next_cursor"`
	Summary    struct {
		Total int `json:"total"`
	} `json:"summary"`
	Reach []struct {
		Source        string `json:"source"`
		Considered    int    `json:"considered"`
		MoreAvailable bool   `json:"more_available"`
	} `json:"reach"`
	SourcesUnavailable []json.RawMessage `json:"sources_unavailable"`
	Counts             []struct {
		Category      string `json:"category"`
		Considered    int    `json:"considered"`
		MoreAvailable bool   `json:"more_available"`
		Total         *int   `json:"total"`
	} `json:"counts"`
}

func measureBacklog(t *testing.T, e *apptest.AppEnv, size int, b backlogSubject) {
	t.Helper()
	first := backlogPage(t, e, b.seat, b.path())
	walk := walkBacklog(t, e, b)
	t.Logf("perfbench [backlog]: size=%d scope=%s mix=%s seeded=%d total=%d unavailable=%d reach=%s totals=%s walk pages=%d rows=%d in %s",
		size, b.scope, b.mix, b.seeded, first.Summary.Total, len(first.SourcesUnavailable), reachLine(first), totalsLine(first),
		walk.pages, walk.rows, walk.took.Round(time.Millisecond))
	if !backlogCountsOnly() {
		timeFirstPage(t, e, size, b)
	}
	probe := probeBacklog(t, e, b, size)
	t.Logf("perfbench [backlog]: size=%d scope=%s mix=%s enumerate tasks=%d waiting=%d quiet_deals=%d statements=%d p50=%s p95=%s samples=%d",
		size, b.scope, b.mix, probe.tasks, probe.waiting, probe.deals, probe.pages, probe.p50, probe.p95, probe.samples)
	t.Logf("perfbench [backlog]: size=%d scope=%s mix=%s walk reached tasks=%d/%d waiting=%d/%d at_risk=%d/%d complete=%v",
		size, b.scope, b.mix, walk.by["task"], probe.tasks, walk.by["customer_waiting"], probe.waiting,
		walk.by["deal_at_risk"], probe.deals, walk.reaches(probe))
}

// backlogWalk is what scrolling to the end served, by source.
type backlogWalk struct {
	pages, rows int
	took        time.Duration
	by          map[string]int
}

// reaches says whether the walk served every seeded row the stores enumerate.
// It compares each source's walked rows with the probe's count.
func (w backlogWalk) reaches(probe backlogProbe) bool {
	return w.by["task"] >= probe.tasks && w.by["customer_waiting"] >= probe.waiting && w.by["deal_at_risk"] >= probe.deals
}

func timeFirstPage(t *testing.T, e *apptest.AppEnv, size int, b backlogSubject) {
	t.Helper()
	samples, err := dailySampling(1, func(bool) ([]time.Duration, error) {
		status, _, elapsed, err := b.seat.fetch(context.Background(), e.TS.URL, b.path())
		if err == nil && status != 200 {
			err = fmt.Errorf("GET %s answered %d", b.path(), status)
		}
		return []time.Duration{elapsed}, err
	})
	if err != nil {
		t.Fatalf("%s %s at %d: %v", b.scope, b.seat.Name, size, err)
	}
	stats, err := search.MeasureQuery("worklist", Perf8Budget, samples[0])
	if err != nil {
		t.Fatal(err)
	}
	alloc := allocatedBy(func() { backlogPage(t, e, b.seat, b.path()) })
	t.Logf("perfbench [backlog]: size=%d scope=%s mix=%s first_page p50=%s p95=%s samples=%d alloc=%.1fMiB",
		size, b.scope, b.mix, stats.P50, stats.P95, stats.Samples, alloc)
}

func backlogPage(t *testing.T, e *apptest.AppEnv, seat Seat, path string) backlogAnswer {
	t.Helper()
	status, body, _ := seat.Get(t, e, path)
	var answer backlogAnswer
	if status != 200 || json.Unmarshal(body, &answer) != nil {
		t.Fatalf("GET %s as %s answered %d: %s", path, seat.Name, status, clipBody(body))
	}
	return answer
}

// reachLine names each source the first page read, with its considered count
// and a + where it stopped at its bound.
func reachLine(a backlogAnswer) string {
	parts := make([]string, 0, len(a.Reach))
	for _, r := range a.Reach {
		if r.Considered == 0 && !r.MoreAvailable {
			continue
		}
		mark := ""
		if r.MoreAvailable {
			mark = "+"
		}
		parts = append(parts, fmt.Sprintf("%s:%d%s", r.Source, r.Considered, mark))
	}
	return strings.Join(parts, ",")
}

// walkBacklog follows next_cursor at the largest page until the queue ends,
// which is how far scrolling reaches.
func walkBacklog(t *testing.T, e *apptest.AppEnv, b backlogSubject) backlogWalk {
	t.Helper()
	walk := backlogWalk{by: map[string]int{}}
	start, path := time.Now(), b.path()+"&limit=100"
	for {
		answer := backlogPage(t, e, b.seat, path)
		walk.pages, walk.rows = walk.pages+1, walk.rows+len(answer.Queue)
		for _, raw := range answer.Queue {
			var row struct {
				Source string `json:"source"`
			}
			if err := json.Unmarshal(raw, &row); err != nil {
				t.Fatalf("reading a walked row: %v", err)
			}
			walk.by[row.Source]++
		}
		if answer.NextCursor == nil {
			walk.took = time.Since(start)
			return walk
		}
		path = b.path() + "&limit=100&cursor=" + url.QueryEscape(*answer.NextCursor)
	}
}

// totalsLine names each category with its exact total, or its considered
// count and a + where it is a floor.
func totalsLine(a backlogAnswer) string {
	parts := make([]string, 0, len(a.Counts))
	for _, c := range a.Counts {
		if c.Total != nil {
			parts = append(parts, fmt.Sprintf("%s:%d", c.Category, *c.Total))
			continue
		}
		mark := ""
		if c.MoreAvailable {
			mark = "+"
		}
		parts = append(parts, fmt.Sprintf("%s:%d%s", c.Category, c.Considered, mark))
	}
	return strings.Join(parts, ",")
}

// allocatedBy is the heap the whole process allocated while read ran. The
// server is in this process, so it bounds one assembly's allocation from above.
func allocatedBy(read func()) float64 {
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	read()
	runtime.ReadMemStats(&after)
	return float64(after.TotalAlloc-before.TotalAlloc) / (1 << 20)
}

func measureConcurrentReaders(t *testing.T, e *apptest.AppEnv, size int, seat Seat) {
	t.Helper()
	path := "/v1/worklist?scope=mine&filter=all"
	round := func(bool) ([]time.Duration, error) {
		took := make([]time.Duration, backlogConcurrentReaders)
		errs := make([]error, backlogConcurrentReaders)
		var wg sync.WaitGroup
		for i := range backlogConcurrentReaders {
			wg.Go(func() {
				var status int
				status, _, took[i], errs[i] = seat.fetch(context.Background(), e.TS.URL, path)
				if errs[i] == nil && status != 200 {
					errs[i] = fmt.Errorf("GET %s answered %d", path, status)
				}
			})
		}
		wg.Wait()
		return took, joinErrs(errs)
	}
	samples, err := dailySampling(backlogConcurrentReaders, round)
	if err != nil {
		t.Fatalf("ten readers at %d: %v", size, err)
	}
	stats, err := search.MeasureQuery("worklist", Perf8Budget, slices.Concat(samples...))
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("perfbench [backlog]: size=%d burst=%d requests from one reader scope=mine mix=mixed p50=%s p95=%s samples=%d",
		size, backlogConcurrentReaders, stats.P50, stats.P95, stats.Samples)
}

func joinErrs(errs []error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

func backlogScale(t *testing.T) float64 {
	t.Helper()
	if os.Getenv(dailyScaleEnv) == "" {
		return 0.05
	}
	scale, err := dailyScale()
	if err != nil {
		t.Fatal(err)
	}
	return scale
}

func backlogSizes(t *testing.T) []int {
	t.Helper()
	raw := os.Getenv(backlogSizesEnv)
	if raw == "" {
		return []int{100, 1_000, 10_000}
	}
	var sizes []int
	for field := range strings.SplitSeq(raw, ",") {
		size, err := strconv.Atoi(strings.TrimSpace(field))
		if err != nil || size <= 0 || (len(sizes) > 0 && size <= sizes[len(sizes)-1]) {
			t.Fatalf("%s=%q: set increasing positive counts, such as 100,1000,10000,100000", backlogSizesEnv, raw)
		}
		sizes = append(sizes, size)
	}
	return sizes
}

func teamRoster(t *testing.T, e *apptest.AppEnv, team string) []string {
	t.Helper()
	var roster []string
	if err := e.Owner.QueryRow(context.Background(),
		`SELECT array_agg(user_id::text) FROM team_membership WHERE team_id = $1::uuid`, team).Scan(&roster); err != nil {
		t.Fatalf("reading team %s: %v", team, err)
	}
	return roster
}

// backlogProbe is one enumeration of every seeded source to its end.
type backlogProbe struct {
	tasks, waiting, deals, pages int
	p50, p95                     time.Duration
	samples                      int
}

// probeBacklog reads each seeded source whole, as the seat, through the lane's
// own store with the lane's bound lifted. Tasks and quiet deals page by their
// cursors, and waiting pages by its keyset. Each page is one statement, so a
// complete assembly needs at least this many per request.
func probeBacklog(t *testing.T, e *apptest.AppEnv, b backlogSubject, size int) backlogProbe {
	t.Helper()
	ctx := seatContext(t, e, b.seat.UserID)
	owners := make([]ids.UUID, len(b.owners))
	for i, owner := range b.owners {
		owners[i] = ids.MustParse(owner)
	}
	activityStore, dealStore := activities.NewStore(e.DB()), deals.NewStore(e.DB(), compose.DealsInstallation())
	if backlogCountsOnly() {
		probe, err := enumerateBacklog(ctx, activityStore, dealStore, b.scope, owners)
		if err != nil {
			t.Fatalf("enumerating %s as %s: %v", b.scope, b.seat.Name, err)
		}
		return probe
	}
	runs := dailySamples
	if size > 10_000 {
		runs = 5
	}
	var probe backlogProbe
	var durations []time.Duration
	// The first run is a warm-up, so every timed statement runs on a cached plan.
	for i := range runs + 1 {
		start := time.Now()
		got, err := enumerateBacklog(ctx, activityStore, dealStore, b.scope, owners)
		if err != nil {
			t.Fatalf("enumerating %s as %s: %v", b.scope, b.seat.Name, err)
		}
		if i > 0 {
			durations = append(durations, time.Since(start))
		}
		probe = got
	}
	stats, err := search.MeasureQuery("enumerate", Perf8Budget, durations)
	if err != nil {
		t.Fatal(err)
	}
	probe.p50, probe.p95, probe.samples = stats.P50, stats.P95, stats.Samples
	return probe
}

func enumerateBacklog(ctx context.Context, activityStore *activities.Store, dealStore *deals.Store, scope string, owners []ids.UUID) (backlogProbe, error) {
	var got backlogProbe
	now := time.Now()
	until := now.Truncate(24 * time.Hour).Add(24 * time.Hour)
	page := 200
	in := activities.ListActivitiesInput{OpenAndDueBy: &until, IncludeEmailRequests: true, WorklistAsOf: &now, ReadableOnly: true, Limit: &page}
	if scope == "mine" {
		own := ids.From[ids.UserKind](owners[0])
		in.OwnQueueOf = &own
	}
	for {
		rows, next, err := activityStore.ListActivities(ctx, in)
		if err != nil {
			return got, err
		}
		got.tasks, got.pages = got.tasks+len(rows), got.pages+1
		if !next.HasMore {
			break
		}
		in.Cursor = &next.NextCursor
	}
	var before activities.WaitingPosition
	for {
		rows, err := waitingPage(ctx, activityStore, now, before, scope, owners)
		if err != nil {
			return got, err
		}
		got.waiting, got.pages = got.waiting+len(rows), got.pages+1
		if len(rows) < activities.WaitingScanCap {
			break
		}
		last := rows[len(rows)-1]
		before = activities.WaitingPosition{At: last.OccurredAt, ID: last.ActivityID}
	}
	quiet := deals.QuietThresholdDays
	list := deals.ListDealsInput{QuietForDays: &quiet, Limit: &page, ScopeOwners: &storekit.OwnerSet{IDs: owners}}
	for {
		rows, next, err := dealStore.ListDeals(ctx, list)
		if err != nil {
			return got, err
		}
		got.deals, got.pages = got.deals+len(rows), got.pages+1
		if !next.HasMore {
			return got, nil
		}
		list.Cursor = &next.NextCursor
	}
}

func waitingPage(ctx context.Context, activityStore *activities.Store, now time.Time, before activities.WaitingPosition, scope string, owners []ids.UUID) ([]activities.WaitingReply, error) {
	if scope == "mine" {
		return activityStore.WaitingRepliesForOwnerBefore(ctx, now, before, owners[0])
	}
	return activityStore.WaitingRepliesAnsweredByBefore(ctx, now, before, owners)
}

// seatContext is the seat as the gates see it after sign-in. It carries the
// seat's own roles, teams and row scope, so the reads pay the same scope clauses.
func seatContext(t *testing.T, e *apptest.AppEnv, user string) context.Context {
	t.Helper()
	ctx := context.Background()
	tx, err := e.Owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil {
			t.Errorf("closing the seat read: %v", err)
		}
	}()
	actor, err := identity.SeatActor(ctx, tx, ids.MustParse(user))
	if err != nil {
		t.Fatalf("resolving seat %s: %v", user, err)
	}
	ctx = principal.WithWorkspaceID(ctx, apptest.InstallationWorkspaceUUID(ctx, t, e.Owner))
	return principal.WithActor(principal.WithCorrelationID(ctx, ids.NewV7()), actor)
}
