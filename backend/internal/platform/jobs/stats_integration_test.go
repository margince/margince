// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package jobs_test

// Real-Postgres proof of the two reads behind the job surfaces. The SQL is
// the whole of the behaviour here — a fake pool would prove the fixture,
// not the query — so every case below seeds raw river_job rows and reads
// them back through the exported entry point.

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// seed is one raw river_job row. Written directly rather than through
// River's client because these tests need states, timestamps and stored
// error text that a real insert cannot produce on demand.
type seed struct {
	Kind      string
	Queue     string   // empty means "default"
	State     string   // river_job_state
	Workspace ids.UUID // a legacy key from a release that queued one; the zero value writes none
	// Connection is the fan-out unit of the three per-connection dispatchers.
	// The zero value writes no connection_id key, which is what a
	// workspace-grained child's args look like.
	Connection ids.UUID
	Tags       []string
	CreatedAt  time.Time // the zero value means now
	Scheduled  time.Time // the zero value means CreatedAt
	Attempt    int
}

// finalizedStates are the states river_job's finalized_or_finalized_at_null
// CHECK requires a finalized_at for. A fixture that omits it is refused by
// Postgres, not by the assertion, so the helper stamps it rather than
// leaving every caller to remember.
var finalizedStates = map[string]bool{"completed": true, "cancelled": true, "discarded": true}

// seedJob writes one raw river_job row and fails the test if Postgres
// refuses it.
func seedJob(ctx context.Context, t *testing.T, pool *pgxpool.Pool, s seed) {
	t.Helper()

	queue := s.Queue
	if queue == "" {
		queue = "default"
	}
	createdAt := s.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now()
	}
	scheduledAt := s.Scheduled
	if scheduledAt.IsZero() {
		scheduledAt = createdAt
	}

	// A zero workspace writes no key at all, which is exactly what a
	// dispatcher's args look like on the wire.
	args := map[string]any{}
	if s.Workspace != (ids.UUID{}) {
		args["workspace_id"] = s.Workspace.String()
	}
	if s.Connection != (ids.UUID{}) {
		args["connection_id"] = s.Connection.String()
	}
	encodedArgs, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("encoding fixture args: %v", err)
	}

	// river_job.tags is NOT NULL, and a nil Go slice binds as NULL rather
	// than as an empty array — an untagged row is the common case here, so
	// the default belongs in the helper.
	tags := s.Tags
	if tags == nil {
		tags = []string{}
	}

	var finalizedAt *time.Time
	if finalizedStates[s.State] {
		finalized := createdAt
		finalizedAt = &finalized
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO river_job
		    (state, kind, queue, args, tags, errors, max_attempts, attempt,
		     created_at, scheduled_at, finalized_at)
		VALUES ($1::river_job_state, $2, $3, $4::jsonb, $5::varchar(255)[],
		        $6::jsonb[], $7, $8, $9, $10, $11)`,
		s.State, s.Kind, queue, encodedArgs, tags, [][]byte{},
		3, s.Attempt, createdAt, scheduledAt, finalizedAt); err != nil {
		t.Fatalf("seeding a %s %s row: %v", s.State, s.Kind, err)
	}
}

// count answers one group's Count, or 0 when the group is absent — the two
// are different claims, so the absent case is asserted explicitly where it
// matters rather than being folded in here.
func count(snap jobs.Snapshot, kind, state string) int64 {
	var total int64
	for _, r := range snap.Rows {
		if r.Kind == kind && r.State == state {
			total += r.Count
		}
	}
	return total
}

func sweepFor(snap jobs.Snapshot, kind string) (jobs.SweepPass, bool) {
	for _, s := range snap.Sweeps {
		if s.Kind == kind {
			return s, true
		}
	}
	return jobs.SweepPass{}, false
}

func unitFor(snap jobs.Snapshot, kind string) (jobs.SweepUnit, bool) {
	for _, u := range snap.Units {
		if u.Kind == kind {
			return u, true
		}
	}
	return jobs.SweepUnit{}, false
}

// TestStatsCountsLiveWorkPerKindWhateverItsArgsCarry proves a row is counted
// under its kind and state alone. A row queued by an earlier release still
// carries a workspace key and must land beside the rows that do not.
func TestStatsCountsLiveWorkPerKindWhateverItsArgsCarry(t *testing.T) {
	_, pool := migratedAppPool(t)
	ctx := t.Context()

	seedJob(ctx, t, pool, seed{Kind: "a_pass", State: "available", Workspace: ids.NewV7()})
	seedJob(ctx, t, pool, seed{Kind: "a_pass", State: "available"})
	seedJob(ctx, t, pool, seed{Kind: "a_pass", State: "discarded"})

	snap, err := jobs.Stats(ctx, pool)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}

	if got := count(snap, "a_pass", "available"); got != 2 {
		t.Errorf("available = %d, want 2: a legacy workspace key must not split the group", got)
	}
	if got := count(snap, "a_pass", "discarded"); got != 1 {
		t.Errorf("discarded = %d, want 1: a failed pass must stay visible", got)
	}
}

// TestStatsExcludesCompletedWorkFromTheRuntimeGauges — a finished job is
// history, not queue depth. Without this the gauges only fall when River's
// cleaner runs.
func TestStatsExcludesCompletedWorkFromTheRuntimeGauges(t *testing.T) {
	_, pool := migratedAppPool(t)
	ctx := t.Context()
	seedJob(ctx, t, pool, seed{Kind: "done", State: "completed"})

	snap, err := jobs.Stats(ctx, pool)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if got := count(snap, "done", "completed"); got != 0 {
		t.Errorf("a completed row reached the runtime gauges: count = %d", got)
	}
}

// TestStatsMeasuresTheAgeOfRunnableWorkOnlyAndInTheDatabasesOwnClock — the
// gauge answers "how long has runnable work been waiting", so a job
// scheduled for the future contributes nothing (it is not late) while one
// whose scheduled time has passed does (a stopped scheduler is precisely
// what this gauge exists to catch). The age is computed by Postgres from
// its own now(): subtracting a database timestamp from the app clock is a
// live intermittent flake elsewhere in this tree.
func TestStatsMeasuresTheAgeOfRunnableWorkOnlyAndInTheDatabasesOwnClock(t *testing.T) {
	_, pool := migratedAppPool(t)
	ctx := t.Context()
	anHourAgo := time.Now().Add(-time.Hour)

	seedJob(ctx, t, pool, seed{
		Kind: "overdue", Queue: "waiting", State: "scheduled",
		CreatedAt: anHourAgo, Scheduled: anHourAgo,
	})
	seedJob(ctx, t, pool, seed{
		Kind: "not_yet", Queue: "future", State: "scheduled",
		CreatedAt: anHourAgo, Scheduled: time.Now().Add(time.Hour),
	})

	snap, err := jobs.Stats(ctx, pool)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}

	var overdue, notYet *float64
	var sawNotYet bool
	for _, r := range snap.Rows {
		switch r.Kind {
		case "overdue":
			overdue = r.OldestRunnableAgeSeconds
		case "not_yet":
			notYet, sawNotYet = r.OldestRunnableAgeSeconds, true
		}
	}
	if overdue == nil {
		t.Fatal("an hour-overdue scheduled job reported no age at all")
	}
	if *overdue < 3000 {
		t.Errorf("an hour-overdue scheduled job reported age %.0fs: a scheduler that "+
			"stopped must not read as a healthy queue on the gauge meant to catch it", *overdue)
	}
	if !sawNotYet {
		t.Fatal("the future-scheduled row is missing from the snapshot entirely; it is " +
			"queued work and must still be counted, only not aged")
	}
	if notYet != nil {
		t.Errorf("a job scheduled for the future reported age %.0fs; it is not runnable, so "+
			"there is no oldest-runnable job to report and NULL is the honest answer", *notYet)
	}
}

// TestSweepReadsAKindsLatestEndedOutcomeAndSurvivesADeduplicatedRetry seeds the
// shape a rescued dispatcher produces. An older child is still live, so it was
// deduplicated out of the newer fan-out, and a newer one has ended. The kind
// reports one pass, and its outcome is the latest row that ended.
func TestSweepReadsAKindsLatestEndedOutcomeAndSurvivesADeduplicatedRetry(t *testing.T) {
	_, pool := migratedAppPool(t)
	ctx := t.Context()
	older, newer := time.Now().Add(-2*time.Hour), time.Now().Add(-1*time.Hour)
	tagged := []string{jobs.SweepTag}

	seedJob(ctx, t, pool, seed{Kind: "sweep_child", State: "retryable", Tags: tagged, CreatedAt: older})
	seedJob(ctx, t, pool, seed{Kind: "sweep_child", State: "completed", Tags: tagged, CreatedAt: older})
	seedJob(ctx, t, pool, seed{Kind: "sweep_child", State: "cancelled", Tags: tagged, CreatedAt: newer})
	// A hand-triggered job of the same kind, carrying no sweep tag.
	seedJob(ctx, t, pool, seed{Kind: "sweep_child", State: "completed"})

	snap, err := jobs.Stats(ctx, pool)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	pass, ok := sweepFor(snap, "sweep_child")
	if !ok {
		t.Fatal("no sweep reported for a tagged kind")
	}
	if pass.Passes != 1 {
		t.Errorf("Passes = %d, want 1: an installation has one pass of a kind", pass.Passes)
	}
	if pass.Failed != 1 {
		t.Errorf("Failed = %d, want 1: the latest TAGGED row that ended was cancelled, which is "+
			"work that did not happen, and the untagged completed row is not a fleet pass", pass.Failed)
	}
}

// TestTheUnitPairSeesAFailedConnectionTheKindPairMasks is the reason the second
// pair exists. Two connections, the broken one failing first and the healthy
// one completing after it.
//
// Read per kind, the most recent child is the successful one and the pass looks
// clean. Read per connection, one unit of two is dead. Both readings are
// asserted, so a change that fixed the masking in the kind pair would fail here.
//
// telegram_poll is used, not a made-up kind. The unit read is derived from the
// contract, so a fixture kind would prove a query that never runs.
func TestTheUnitPairSeesAFailedConnectionTheKindPairMasks(t *testing.T) {
	_, pool := migratedAppPool(t)
	ctx := t.Context()
	earlier, later := time.Now().Add(-2*time.Hour), time.Now().Add(-1*time.Hour)
	ws := ids.NewV7()
	broken, healthy := ids.NewV7(), ids.NewV7()

	seedJob(ctx, t, pool, seed{
		Kind: "telegram_poll", State: "discarded",
		Workspace: ws, Connection: broken, Tags: []string{jobs.SweepTag}, CreatedAt: earlier,
	})
	seedJob(ctx, t, pool, seed{
		Kind: "telegram_poll", State: "completed",
		Workspace: ws, Connection: healthy, Tags: []string{jobs.SweepTag}, CreatedAt: later,
	})

	snap, err := jobs.Stats(ctx, pool)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}

	pass, ok := sweepFor(snap, "telegram_poll")
	if !ok {
		t.Fatal("no per-workspace sweep reported for a tagged kind")
	}
	if pass.Passes != 1 || pass.Failed != 0 {
		t.Errorf("workspace pair = %d/%d covered/failed, want 1/0 — this pair counts one workspace "+
			"once and reads its LATEST child, which is why the failure is invisible here",
			pass.Passes, pass.Failed)
	}

	unit, ok := unitFor(snap, "telegram_poll")
	if !ok {
		t.Fatal("no per-unit sweep reported for a kind whose dispatcher fans out per connection")
	}
	if unit.Unit != jobs.FanOutConnection {
		t.Errorf("Unit = %d, want FanOutConnection — the label an alert reads the grain off", unit.Unit)
	}
	if unit.Units != 2 {
		t.Errorf("Units = %d, want 2: one workspace holding two connections is two units", unit.Units)
	}
	if unit.Failed != 1 {
		t.Errorf("Failed = %d, want 1 — the broken connection, which the workspace pair reported as zero", unit.Failed)
	}
}

// TestTheUnitPairIgnoresAnUntaggedRowAndAWorkspaceGrainedKind holds the two
// boundaries of the new read. An untagged row of a per-connection kind is a
// job someone triggered by hand, not one connection's share of a fleet pass;
// and a kind whose declared unit IS the workspace must not appear here at all,
// or its number would be published twice and an operator summing the pairs
// would double-count the fleet.
func TestTheUnitPairIgnoresAnUntaggedRowAndAWorkspaceGrainedKind(t *testing.T) {
	_, pool := migratedAppPool(t)
	ctx := t.Context()
	ws := ids.NewV7()

	seedJob(ctx, t, pool, seed{
		Kind: "telegram_poll", State: "discarded",
		Workspace: ws, Connection: ids.NewV7(),
	})
	seedJob(ctx, t, pool, seed{
		Kind: "capture_digest_workspace", State: "discarded",
		Workspace: ws, Tags: []string{jobs.SweepTag},
	})

	snap, err := jobs.Stats(ctx, pool)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if unit, ok := unitFor(snap, "telegram_poll"); ok {
		t.Errorf("an untagged row produced a unit series (%+v); a hand-triggered poll is not one "+
			"connection's share of a fleet pass", unit)
	}
	if unit, ok := unitFor(snap, "capture_digest_workspace"); ok {
		t.Errorf("a workspace-grained kind produced a unit series (%+v); it is already reported by "+
			"the workspace pair, and two series for one number double-count", unit)
	}
	if pass, ok := sweepFor(snap, "capture_digest_workspace"); !ok || pass.Failed != 1 {
		t.Errorf("the workspace-grained kind must still be reported by the workspace pair; got %+v (present=%t)", pass, ok)
	}
}

// TestARealRiverInsertCarriesTheSweepTagIntoTheColumnTheReadFiltersOn
// closes
// the gap every other test in this file leaves open: they seed the tag by
// hand, so they prove the query and not the pipeline. This one goes through
// River's own insert path with the tag on InsertOpts, and asserts the sweep
// read finds it — so a River release that stopped persisting tags, or an
// InsertOpts merge that dropped them, fails here rather than silently
// emptying the sweep gauges in production.
func TestARealRiverInsertCarriesTheSweepTagIntoTheColumnTheReadFiltersOn(t *testing.T) {
	_, pool := migratedAppPool(t)
	ctx := t.Context()

	// A runner of this test's own, because River refuses to insert a kind
	// its Workers bundle does not register and the shared helper registers
	// only its no-op.
	workers := river.NewWorkers()
	river.AddWorker(workers, &taggedWorker{})
	runner, err := jobs.New(pool, jobs.Config{
		Queues:  map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: 1}},
		Workers: workers,
	}, quietLogger())
	if err != nil {
		t.Fatalf("jobs.New: %v", err)
	}

	if err := runner.Enqueue(ctx, taggedArgs{},
		&river.InsertOpts{Tags: []string{jobs.SweepTag}}); err != nil {
		t.Fatalf("enqueueing a tagged job: %v", err)
	}

	snap, err := jobs.Stats(ctx, pool)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	pass, ok := sweepFor(snap, taggedArgs{}.Kind())
	if !ok {
		t.Fatal("a job River inserted with the sweep tag is absent from the sweep read; " +
			"the tag did not survive the insert path the dispatchers use")
	}
	if pass.Passes != 1 {
		t.Errorf("Passes = %d, want 1", pass.Passes)
	}
}

// taggedArgs is a kind for the round-trip above.
type taggedArgs struct{}

func (taggedArgs) Kind() string { return "tagged_round_trip" }

// taggedWorker exists only so River accepts the insert; the round-trip
// asserts what the INSERT wrote, and never runs the job.
type taggedWorker struct {
	river.WorkerDefaults[taggedArgs]
}

func (taggedWorker) Work(context.Context, *river.Job[taggedArgs]) error { return nil }

// TestSweepIgnoresASupersededFailure is the mirror of the above. A pass that
// failed and then succeeded is not a pass being missed.
func TestSweepIgnoresASupersededFailure(t *testing.T) {
	_, pool := migratedAppPool(t)
	ctx := t.Context()
	ws := ids.NewV7()
	seedJob(ctx, t, pool, seed{
		Kind: "sweep_child", State: "discarded", Workspace: ws,
		Tags: []string{jobs.SweepTag}, CreatedAt: time.Now().Add(-2 * time.Hour),
	})
	seedJob(ctx, t, pool, seed{
		Kind: "sweep_child", State: "completed", Workspace: ws,
		Tags: []string{jobs.SweepTag}, CreatedAt: time.Now().Add(-time.Hour),
	})

	snap, err := jobs.Stats(ctx, pool)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	pass, ok := sweepFor(snap, "sweep_child")
	if !ok {
		t.Fatal("no sweep reported for a tagged kind")
	}
	if pass.Failed != 0 {
		t.Errorf("Failed = %d, want 0: the workspace recovered", pass.Failed)
	}
}

// TestSweepOmitsADispatchersOwnRow — a dispatcher carries no workspace, so
// it is not one workspace's share of anything. Counting it would add a
// phantom tenant to every sweep it tagged.
//
// gmail_sync is seeded alongside the made-up kind deliberately: a fixture
// kind can never appear in the standalone-fleet-kind set regardless of
// whether that set is derived correctly, so it alone would keep passing
// even if the new predicate started routing every dispatcher's own row into
// the per-kind arm. gmail_sync is a REAL declared dispatcher (FanOutTo:
// "capture_sync") and is the case that would actually catch that mistake.
func TestSweepOmitsADispatchersOwnRow(t *testing.T) {
	_, pool := migratedAppPool(t)
	ctx := t.Context()
	seedJob(ctx, t, pool, seed{
		Kind: "the_dispatcher", State: "completed",
		Tags: []string{jobs.SweepTag},
	})
	seedJob(ctx, t, pool, seed{
		Kind: "gmail_sync", State: "completed",
		Tags: []string{jobs.SweepTag},
	})

	snap, err := jobs.Stats(ctx, pool)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if _, ok := sweepFor(snap, "the_dispatcher"); ok {
		t.Error("an untenanted row was counted as a workspace's share of a fleet pass")
	}
	if _, ok := sweepFor(snap, "gmail_sync"); ok {
		t.Error("a real dispatcher's own row was counted — its children (capture_sync) carry the per-workspace signal, not its own row")
	}
}

// TestSweepCountsAStandaloneFleetPassByKind: a fleet kind that fans out to
// nothing (embed_drift_sweep) is counted as one pass. It is a real declared
// kind and not a made-up one, because the dispatcher exclusion is derived from
// the contract.
func TestSweepCountsAStandaloneFleetPassByKindNotByWorkspace(t *testing.T) {
	_, pool := migratedAppPool(t)
	ctx := t.Context()

	seedJob(ctx, t, pool, seed{
		Kind: "embed_drift_sweep", State: "discarded",
		Tags: []string{jobs.SweepTag},
	})

	snap, err := jobs.Stats(ctx, pool)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	pass, ok := sweepFor(snap, "embed_drift_sweep")
	if !ok {
		t.Fatal("a tagged row of a standalone Fleet kind is missing from the sweep read entirely")
	}
	if pass.Passes != 1 {
		t.Errorf("Passes = %d, want 1: there is no workspace grain for this kind, and the pass ran once", pass.Passes)
	}
	if pass.Failed != 1 {
		t.Errorf("Failed = %d, want 1: the only tick discarded", pass.Failed)
	}
}

// TestSweepReadsOnlyTheLatestTickOfAStandaloneFleetPass proves the
// "latest outcome, never a batch" rule statsBySweep's own doc states also
// holds for the per-kind arm: an earlier failing tick a later successful
// one supersedes must not still read as failed.
func TestSweepReadsOnlyTheLatestTickOfAStandaloneFleetPass(t *testing.T) {
	_, pool := migratedAppPool(t)
	ctx := t.Context()
	earlier, later := time.Now().Add(-2*time.Hour), time.Now().Add(-time.Hour)

	seedJob(ctx, t, pool, seed{
		Kind: "embed_drift_sweep", State: "discarded",
		Tags: []string{jobs.SweepTag}, CreatedAt: earlier,
	})
	seedJob(ctx, t, pool, seed{
		Kind: "embed_drift_sweep", State: "completed",
		Tags: []string{jobs.SweepTag}, CreatedAt: later,
	})

	snap, err := jobs.Stats(ctx, pool)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	pass, ok := sweepFor(snap, "embed_drift_sweep")
	if !ok {
		t.Fatal("no sweep reported for a tagged standalone Fleet kind")
	}
	if pass.Passes != 1 {
		t.Errorf("Passes = %d, want 1", pass.Passes)
	}
	if pass.Failed != 0 {
		t.Errorf("Failed = %d, want 0: the LATEST tick succeeded", pass.Failed)
	}
}

// TestStatsOnAnEmptyJobTableIsAnEmptySnapshotNotAnError — the honest empty
// case. A fleet with nothing queued is a real state, and the reader above
// must be able to tell it from a failed read.
func TestStatsOnAnEmptyJobTableIsAnEmptySnapshotNotAnError(t *testing.T) {
	_, pool := migratedAppPool(t)

	snap, err := jobs.Stats(t.Context(), pool)
	if err != nil {
		t.Fatalf("Stats on an empty table: %v", err)
	}
	if len(snap.Rows) != 0 || len(snap.Sweeps) != 0 {
		t.Errorf("an empty job table produced %d rows and %d sweeps", len(snap.Rows), len(snap.Sweeps))
	}
}

// TestStatsFailsLoudlyWhenTheJobTableIsUnreachable — a read that could not
// happen must say so rather than answering with an empty snapshot, which
// downstream renders as a healthy idle fleet.
//
// Tagged because the migrated job table is the control: the point is that the
// error comes from the cancelled read, and against an unmigrated database it
// would come from a missing relation instead — the test would pass while
// proving nothing. It issues no query of its own, so a query-counting check
// will read it as infra-free; it is not.
func TestStatsFailsLoudlyWhenTheJobTableIsUnreachable(t *testing.T) {
	_, pool := migratedAppPool(t)

	cancelled, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := jobs.Stats(cancelled, pool); err == nil {
		t.Fatal("Stats answered a snapshot on a cancelled context; an unmeasured fleet " +
			"must not be indistinguishable from an empty one")
	}
}

// A tick that exhausted its attempts stays failed while the next one is merely
// pending. This is the blind spot the gauge was reported with: a periodic pass has
// an unfinished successor almost all the time, so reading the newest row as the
// outcome let every discard read as healthy.
func TestAPendingNextTickDoesNotMaskADiscardedStandalonePass(t *testing.T) {
	_, pool := migratedAppPool(t)
	ctx := t.Context()
	earlier, later := time.Now().Add(-2*time.Hour), time.Now().Add(-time.Hour)

	seedJob(ctx, t, pool, seed{
		Kind: "embed_drift_sweep", State: "discarded",
		Tags: []string{jobs.SweepTag}, CreatedAt: earlier,
	})
	seedJob(ctx, t, pool, seed{
		Kind: "embed_drift_sweep", State: "available",
		Tags: []string{jobs.SweepTag}, CreatedAt: later,
	})

	snap, err := jobs.Stats(ctx, pool)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	pass, ok := sweepFor(snap, "embed_drift_sweep")
	if !ok {
		t.Fatal("no sweep reported for a tagged standalone Fleet kind")
	}
	if pass.Passes != 1 {
		t.Errorf("Passes = %d, want 1: the pass is still covered — a queued tick is reach, "+
			"and only the OUTCOME waits for a row that ended", pass.Passes)
	}
	if pass.Failed != 1 {
		t.Errorf("Failed = %d, want 1: the last tick that ENDED was discarded, and the one after "+
			"it has not run — an unfinished row is not evidence the pass recovered", pass.Failed)
	}
}

// The same for a fan-out child kind: its last finished row discarded, and a
// newer unfinished one of the next pass over it.
func TestAPendingNextChildDoesNotMaskADiscardedShare(t *testing.T) {
	_, pool := migratedAppPool(t)
	ctx := t.Context()
	earlier, later := time.Now().Add(-2*time.Hour), time.Now().Add(-time.Hour)

	seedJob(ctx, t, pool, seed{
		Kind: "embed_drift", State: "discarded",
		Tags: []string{jobs.SweepTag}, CreatedAt: earlier,
	})
	seedJob(ctx, t, pool, seed{
		Kind: "embed_drift", State: "running",
		Tags: []string{jobs.SweepTag}, CreatedAt: later,
	})

	snap, err := jobs.Stats(ctx, pool)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	pass, ok := sweepFor(snap, "embed_drift")
	if !ok {
		t.Fatal("no sweep reported for a tagged fan-out kind")
	}
	if pass.Passes != 1 {
		t.Errorf("Passes = %d, want 1", pass.Passes)
	}
	if pass.Failed != 1 {
		t.Errorf("Failed = %d, want 1: the last finished share was discarded, and a "+
			"running retry is not yet evidence it recovered", pass.Failed)
	}
}

// A pass with nothing finished yet reports no outcome either way, which is what
// keeps the fix from inventing a failure out of silence.
func TestAPassWithNothingEndedYetReportsNoFailure(t *testing.T) {
	_, pool := migratedAppPool(t)
	ctx := t.Context()

	seedJob(ctx, t, pool, seed{
		Kind: "embed_drift_sweep", State: "running",
		Tags: []string{jobs.SweepTag},
	})

	snap, err := jobs.Stats(ctx, pool)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	pass, ok := sweepFor(snap, "embed_drift_sweep")
	if !ok {
		t.Fatal("no sweep reported for a tagged standalone Fleet kind — the counts below would " +
			"both read 0 from a missing row and look like agreement")
	}
	if pass.Passes != 1 || pass.Failed != 0 {
		t.Errorf("Passes = %d Failed = %d, want 1 and 0: a first tick still running has "+
			"neither succeeded nor failed", pass.Passes, pass.Failed)
	}
}

// The unit grain reads the last ENDED child too. statsBySweepUnit's doc says its
// rule is the SAME as statsBySweep's, and one of the two being fixed is how that
// sentence stops being true: a connection whose share was discarded kept reading
// healthy while its retry was merely pending.
func TestAPendingNextChildDoesNotMaskADiscardedUnit(t *testing.T) {
	_, pool := migratedAppPool(t)
	ctx := t.Context()
	ws, conn := ids.NewV7(), ids.NewV7()
	earlier, later := time.Now().Add(-2*time.Hour), time.Now().Add(-time.Hour)

	seedJob(ctx, t, pool, seed{
		Kind: "telegram_poll", State: "discarded", Workspace: ws, Connection: conn,
		Tags: []string{jobs.SweepTag}, CreatedAt: earlier,
	})
	seedJob(ctx, t, pool, seed{
		Kind: "telegram_poll", State: "running", Workspace: ws, Connection: conn,
		Tags: []string{jobs.SweepTag}, CreatedAt: later,
	})

	snap, err := jobs.Stats(ctx, pool)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	unit, ok := unitFor(snap, "telegram_poll")
	if !ok {
		t.Fatal("no per-unit sweep reported for a kind whose dispatcher fans out per connection")
	}
	if unit.Units != 1 {
		t.Errorf("Units = %d, want 1: one connection was reached, pending retry included", unit.Units)
	}
	if unit.Failed != 1 {
		t.Errorf("Failed = %d, want 1: this connection's last finished share was discarded, and a "+
			"running retry is not yet evidence it recovered", unit.Failed)
	}
}
