// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package jobfanout

// The Surface-B agent scheduler is one job row per live tenant. Its failure
// mode is the opposite of the sweeps beside it: a tenant whose occurrence
// seeding or due-job claim hit a database error used to abandon the pass for
// EVERY LATER TENANT TOO, so one workspace's transient fault meant the morning
// brief never ran anywhere behind it in the fleet order — silently, on every
// tick, until the fault cleared.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/integration/jobtest"
	"github.com/margince/margince/backend/internal/modules/agents/runner"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// schedulerPassCtx is the scope the scheduler's workspace worker binds before
// it ticks: the tenant, and nothing else. Each claimed job resolves its own
// passport and mints its own correlation id inside the pass, so a suite that
// bound an actor here would be exercising provenance production never writes.
func schedulerPassCtx(ws ids.UUID) context.Context {
	return principal.WithWorkspaceID(context.Background(), ws)
}

// afterEveryDueHour is a reading late enough on its own UTC day that every
// catalog occurrence has fallen due. It is pinned rather than read off the wall
// clock because the catalog's due hours are fixed UTC hours: a suite running
// before the earliest of them would seed nothing, and its seeding assertions
// would hold vacuously for those hours of the day.
func afterEveryDueHour() time.Time {
	d := time.Now().UTC()
	return time.Date(d.Year(), d.Month(), d.Day(), 23, 0, 0, 0, time.UTC)
}

// failRunnerJobWritesFor makes every runner_job write inside ONE tenant raise,
// leaving every other tenant's writes untouched. Both halves of a scheduling
// pass write that table — seeding INSERTs an occurrence, claiming UPDATEs it to
// running — so a BEFORE INSERT OR UPDATE trigger covers the pass whatever the
// clock says is due.
//
// It is dropped in cleanup: the integration lane resets rows between tests but
// keeps the schema, so a surviving trigger would break every later suite that
// schedules a run in this workspace.
func failRunnerJobWrites(t *testing.T, owner *pgx.Conn) {
	t.Helper()
	ctx := context.Background()
	if _, err := owner.Exec(ctx, `
		CREATE OR REPLACE FUNCTION runner_job_write_fault() RETURNS trigger
		LANGUAGE plpgsql AS $$
		BEGIN
		  RAISE EXCEPTION 'runner job write fault injection';
		END $$`); err != nil {
		t.Fatalf("creating the fault-injection function: %v", err)
	}
	// Registered before the trigger is armed, not after both: a failure to arm
	// would otherwise leave the function behind, which is the leak this cleanup
	// exists to prevent. Cleanups run LIFO, so the trigger still drops first.
	t.Cleanup(func() {
		if _, err := owner.Exec(context.Background(),
			`DROP FUNCTION runner_job_write_fault()`); err != nil {
			t.Errorf("dropping the fault-injection function: %v", err)
		}
	})
	// Every write, not one tenant's: the table carries no workspace to key a
	// WHEN clause on (ADR-0091 §8 phase D), and the pass under test is the
	// installation's only one. The trigger drops in cleanup, so its blast
	// radius is this test.
	if _, err := owner.Exec(ctx, `
		CREATE TRIGGER runner_job_write_fault_trigger
		BEFORE INSERT OR UPDATE ON runner_job
		FOR EACH ROW EXECUTE FUNCTION runner_job_write_fault()`); err != nil {
		t.Fatalf("arming the fault-injection trigger: %v", err)
	}
	t.Cleanup(func() {
		if _, err := owner.Exec(context.Background(),
			`DROP TRIGGER runner_job_write_fault_trigger ON runner_job`); err != nil {
			t.Errorf("dropping the fault-injection trigger: %v", err)
		}
	})
}

// seedDueRunnerJob plants an already-due, passport-less occurrence in ws. It
// gives a tenant real work for a pass to claim: a workspace with nothing due
// never updates a row, so neither the fault below nor the claim above it would
// have anything to act on.
//
// Passport-less is deliberate — executing it needs no model and no bound
// identity, and the loud "no passport bound" failure it lands on the row is the
// cheapest honest evidence that the claim ran and the job was executed. The
// spec is a real catalog name for the same reason: an unknown one fails one
// step earlier, before the claim's result has been read at all.
func seedDueRunnerJob(t *testing.T, owner *pgx.Conn, trigger string) {
	t.Helper()
	if _, err := owner.Exec(context.Background(), `
		INSERT INTO runner_job (agent_spec, trigger_ref, due_at)
		VALUES ('morning_brief', $1, now() - interval '1 minute')`, trigger); err != nil {
		t.Fatalf("seeding a due runner job: %v", err)
	}
}

// runnerJobOutcome reads one seeded occurrence's status and failure reason.
func runnerJobOutcome(t *testing.T, owner *pgx.Conn, trigger string) (status, lastError string) {
	t.Helper()
	var reason *string
	if err := owner.QueryRow(context.Background(),
		`SELECT status, last_error FROM runner_job WHERE trigger_ref = $1`,
		trigger).Scan(&status, &reason); err != nil {
		t.Fatalf("reading the runner job %s: %v", trigger, err)
	}
	if reason != nil {
		lastError = *reason
	}
	return status, lastError
}

// assertSpecSeededFor fails unless this seat holds the given spec's occurrence
// for the day. This is the half of a pass that has no other trace: an unseeded
// occurrence is simply a brief that never happens, with no row anywhere to
// notice its absence.
//
// It asks about ONE spec rather than the whole catalog, because a grant is per
// agent. A rep who said yes to the morning brief has said nothing about the
// at-risk sweep, and a helper that demanded both would be asserting a
// consent-everywhere model the product deliberately does not have.
func assertSpecSeededFor(t *testing.T, owner *pgx.Conn, spec runner.AgentSpec, now time.Time, seat ids.UserID) {
	t.Helper()
	var seeded bool
	if err := owner.QueryRow(context.Background(), `
		SELECT EXISTS (SELECT 1 FROM runner_job WHERE trigger_ref = $1)`,
		spec.TriggerRef(now, seat)).Scan(&seeded); err != nil {
		t.Fatalf("reading the seeded occurrences of %s: %v", spec.Name, err)
	}
	if !seeded {
		t.Errorf("no %s occurrence exists for %s on seat %s — that rep's agent simply never runs, and no row records it",
			spec.Name, now.Format(time.DateOnly), seat)
	}
}

// TestAgentSchedulerReportsAPassThatCouldNotWrite is the characterization: a
// scheduling pass that cannot write must reach its caller as an error. A pass
// that reported success over it leaves the briefs unseeded and unclaimed with
// no row saying so.
func TestAgentSchedulerReportsAPassThatCouldNotWrite(t *testing.T) {
	re := setupRunner(t)
	owner := integration.OwnerConn(t)

	const trigger = "morning_brief:write-fault"
	// Real due work, seeded before the fault is armed, so the claim has a row
	// to update and trip the trigger.
	seedDueRunnerJob(t, owner, trigger)
	failRunnerJobWrites(t, owner)

	now := afterEveryDueHour()
	if err := re.svc.Tick(schedulerPassCtx(re.wsID), now); err == nil {
		t.Fatal("a scheduling pass that could not write reported success — nothing records that the briefs were never seeded or claimed")
	}
	if status, _ := runnerJobOutcome(t, owner, trigger); status != "queued" {
		t.Fatalf("the seeded job is %s, want it untouched at queued — the fault injection did not hold, so this test proves nothing", status)
	}
}

// TestAgentSchedulerSeedsAndClaimsWhatIsDue is what survives the fan-out.
//
// It used to enqueue one row per live workspace, exclude archived tenants, and
// prove that the tenant whose writes failed was the ONLY row that failed. A
// pass is one row now (ADR-0103 §1), so what is worth pinning is the half that
// never depended on there being several: a pass seeds every occurrence due at
// its instant and claims the work already waiting — the two halves that have no
// other trace, since an unseeded occurrence is simply a brief that never
// happens.
func TestAgentSchedulerSeedsAndClaimsWhatIsDue(t *testing.T) {
	re := setupRunner(t)
	owner := integration.OwnerConn(t)

	const trigger = "morning_brief:seed-and-claim"
	seedDueRunnerJob(t, owner, trigger)

	// The pass seeds for the reps who granted, so a seat that said yes is what
	// gives the seeding half anything to prove.
	me := re.sessionUser(t)
	if err := re.recordDecision(t, me, runner.GrantStateGranted, &re.passportID); err != nil {
		t.Fatalf("record the grant: %v", err)
	}

	now := afterEveryDueHour()
	if err := re.svc.Tick(schedulerPassCtx(re.wsID), now); err != nil {
		t.Fatalf("the scheduling pass: %v", err)
	}
	assertSpecSeededFor(t, owner, grantedSpec(t), now, me)

	status, lastError := runnerJobOutcome(t, owner, trigger)
	if status == "queued" {
		t.Fatal("the due job is still queued — the pass reported success without claiming anything")
	}
	// Passport-less by design: the loud "no passport bound" failure on the row
	// is the cheapest honest evidence that the claim ran and the job executed.
	if status != "failed" || lastError == "" {
		t.Fatalf("the passport-less job is %s (%q), want a loud failure — the claim reached it but the execution did not", status, lastError)
	}
}

// TestAgentSchedulerDispatchRepeatsOnItsConfiguredInterval pins the half of the
// schedule a boot pass hides. RunOnStart fires once whatever the cadence is, so
// a dispatcher wired to a constant instead of the operator's --runner-interval
// looks identical at boot and then never runs again — every workspace's due
// occurrences unseeded from that moment on, with every gate green. Two
// dispatches less than jobtest.DispatchGapBound apart can only happen if a
// cadence far shorter than that flag's own default is what River is scheduling
// on.
func TestAgentSchedulerDispatchRepeatsOnItsConfiguredInterval(t *testing.T) {
	re := setupRunner(t)

	_, completed, _ := jobtest.StartTestJobRunner(t, re.pool, compose.JobRunnerConfig{
		CloseDateInterval: time.Hour,
		ReconcileInterval: time.Hour,
		TimeScanInterval:  time.Hour,
		AgentScheduler: compose.AgentSchedulerConfig{
			Interval: jobtest.DispatchInterval, Service: re.svc,
		},
	})
	// Generous compared with the gap bound: a run this slow is a sick machine,
	// and the assertion that decides the outcome is the gap below, not this.
	waitCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	kind := compose.AgentSchedulerArgs{}.Kind()
	first, second := jobtest.AwaitTwoDispatchArrivals(waitCtx, t, completed, kind)
	if gap := second.Sub(first); gap > jobtest.DispatchGapBound {
		t.Fatalf("the two %s dispatches were %s apart, over the %s bound — the schedule is not the configured %s interval but some larger constant, and --runner-interval's own 30s default is the one that would look exactly like this",
			kind, gap, jobtest.DispatchGapBound, jobtest.DispatchInterval)
	}
}

// TestAgentSchedulerWithoutAnIntervalSchedulesNothingButStillWorksAQueuedRow
// pins the omission. River accepts PeriodicInterval(0) and turns it into a
// schedule whose next run time never advances, so a runner assembled by a
// caller that never meant to schedule agents would fan the whole fleet out as
// fast as Postgres accepts an insert. Registering no schedule is the honest
// reading; the WORKERS still register, so a row an earlier boot queued is still
// worked rather than stranded.
func TestAgentSchedulerWithoutAnIntervalSchedulesNothingButStillWorksAQueuedRow(t *testing.T) {
	re := setupRunner(t)

	jobRunner, completed, _ := jobtest.StartTestJobRunner(t, re.pool, compose.JobRunnerConfig{
		CloseDateInterval: time.Hour,
		ReconcileInterval: time.Hour,
		TimeScanInterval:  time.Hour,
		AgentScheduler:    compose.AgentSchedulerConfig{Interval: 0, Service: re.svc},
	})
	if err := jobRunner.Enqueue(context.Background(), compose.AgentSchedulerArgs{}, nil); err != nil {
		t.Fatalf("enqueueing the pass an earlier boot would have left: %v", err)
	}

	// The close-date sweep is the FENCE, and it has to be: River inserts every
	// RunOnStart periodic job in one round after Start returns, so a run that
	// only waited on the hand-queued row could read the count before that round
	// had happened at all — and would then report zero however the schedule was
	// wired. Waiting for a sibling RunOnStart dispatcher to complete puts the
	// round provably in the past. The scheduling pass is waited on for the other
	// half of the claim: a queued row is still worked with no schedule present.
	waitCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	jobtest.AwaitKindsCompleted(waitCtx, t, completed,
		compose.CloseDateSweepArgs{}.Kind(), compose.AgentSchedulerArgs{}.Kind())

	var dispatched int
	if err := re.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM river_job WHERE kind = $1`,
		compose.AgentSchedulerArgs{}.Kind()).Scan(&dispatched); err != nil {
		t.Fatalf("counting the dispatched scheduling passes: %v", err)
	}
	// Exactly the one row this test queued by hand. The count used to be zero
	// because the schedule and the work were different kinds; one kind does both
	// now, so a spinning schedule shows as rows arriving ABOVE this floor.
	if dispatched != 1 {
		t.Errorf("%d %s rows exist after a runner was given no scheduler interval, want only the one queued here — a zero duration is not a cadence, and River spins on it rather than refusing it",
			dispatched, compose.AgentSchedulerArgs{}.Kind())
	}
}

// TestEverySeatThatGrantedGetsItsOwnNightlyOccurrence is the regression this
// whole change exists for.
//
// Before the trigger ref carried a seat, both uniqueness rules made the night
// workspace-wide: the first rep seeded took the row and every other rep's
// insert conflicted away. NOTHING FAILED WHEN THAT HAPPENED — ON CONFLICT DO
// NOTHING is the intended re-seed path, the pass returned nil, and the only
// symptom was reps quietly not getting briefs. So the assertion is a count
// across seats, not the success of the pass.
func TestEverySeatThatGrantedGetsItsOwnNightlyOccurrence(t *testing.T) {
	re := setupRunner(t)
	owner := integration.OwnerConn(t)

	me := re.sessionUser(t)
	colleague := seedColleague(t, owner)
	theirs := re.mintPassportForColleague(t, colleague)
	if err := re.recordDecision(t, me, runner.GrantStateGranted, &re.passportID); err != nil {
		t.Fatalf("record the grant for %s: %v", me, err)
	}
	if err := re.recordDecision(t, colleague, runner.GrantStateGranted, &theirs); err != nil {
		t.Fatalf("record the grant for %s: %v", colleague, err)
	}

	now := afterEveryDueHour()
	if err := re.svc.Tick(schedulerPassCtx(re.wsID), now); err != nil {
		t.Fatalf("the scheduling pass: %v", err)
	}

	// Count the DISTINCT rows first. Asking per seat whether "a row for this
	// ref exists" is satisfied by one shared row answering for both seats —
	// which is precisely the workspace-wide behaviour this test exists to
	// catch, so that form of the assertion passes exactly when the bug is
	// present. The count is what cannot be satisfied by one row.
	spec := grantedSpec(t)
	var rows int
	if err := owner.QueryRow(context.Background(),
		`SELECT count(*) FROM runner_job WHERE agent_spec = $1`, grantSpec).Scan(&rows); err != nil {
		t.Fatalf("counting the seeded occurrences: %v", err)
	}
	if rows != 2 {
		t.Fatalf("%d occurrence(s) were seeded for 2 granting seats: the trigger ref does not "+
			"distinguish reps, so the uniqueness constraints admit one run for the whole "+
			"workspace and every seat after the first silently gets nothing", rows)
	}
	for _, seat := range []ids.UserID{me, colleague} {
		var passport *string
		if err := owner.QueryRow(context.Background(),
			`SELECT passport_id::text FROM runner_job WHERE agent_spec = $1 AND trigger_ref = $2`,
			grantSpec, spec.TriggerRef(now, seat)).Scan(&passport); err != nil {
			t.Fatalf("seat %s has no occurrence of its own — the night ran for somebody else and this rep silently got nothing: %v", seat, err)
		}
		if passport == nil {
			t.Fatalf("seat %s was seeded with no passport: the run can only fail at execution, which is a broken night rather than an honest refusal", seat)
		}
	}
}

// TestASeatIsSeededWithItsOwnCredentialAndNobodyElses pins WHOSE authority the
// night carries.
//
// A fan-out that seeded the right number of rows against one shared passport
// would satisfy the count above while acting for every rep as one person —
// which is the exact thing the standing grant exists to prevent.
func TestASeatIsSeededWithItsOwnCredentialAndNobodyElses(t *testing.T) {
	re := setupRunner(t)
	owner := integration.OwnerConn(t)

	me := re.sessionUser(t)
	colleague := seedColleague(t, owner)
	theirs := re.mintPassportForColleague(t, colleague)
	if err := re.recordDecision(t, me, runner.GrantStateGranted, &re.passportID); err != nil {
		t.Fatalf("record the grant for %s: %v", me, err)
	}
	if err := re.recordDecision(t, colleague, runner.GrantStateGranted, &theirs); err != nil {
		t.Fatalf("record the grant for %s: %v", colleague, err)
	}

	now := afterEveryDueHour()
	if err := re.svc.Tick(schedulerPassCtx(re.wsID), now); err != nil {
		t.Fatalf("the scheduling pass: %v", err)
	}

	spec := grantedSpec(t)
	for seat, want := range map[ids.UserID]ids.PassportID{me: re.passportID, colleague: theirs} {
		var got string
		if err := owner.QueryRow(context.Background(),
			`SELECT passport_id::text FROM runner_job WHERE trigger_ref = $1`,
			spec.TriggerRef(now, seat)).Scan(&got); err != nil {
			t.Fatalf("reading seat %s's occurrence: %v", seat, err)
		}
		if got != want.String() {
			t.Errorf("seat %s is seeded with passport %s, want its own %s — the night would act for this rep under somebody else's authority", seat, got, want)
		}
	}
}

// TestARepWhoDeclinedIsNotSeeded holds the other direction: the standing grant
// is a real gate on the fan-out, not a row the seeder reads past.
//
// The declining rep is the case that makes the feature honest. A pass that
// seeded them anyway would run an agent overnight for somebody who said no,
// and the only reason it would not act is that they hold no passport — which
// is a failure at 2am rather than a decision respected at seeding time.
func TestARepWhoDeclinedIsNotSeeded(t *testing.T) {
	re := setupRunner(t)
	owner := integration.OwnerConn(t)

	me := re.sessionUser(t)
	if err := re.recordDecision(t, me, runner.GrantStateGranted, &re.passportID); err != nil {
		t.Fatalf("record the grant for %s: %v", me, err)
	}

	colleague := seedColleague(t, owner)
	if err := re.recordDecision(t, colleague, runner.GrantStateDeclined, nil); err != nil {
		t.Fatalf("recording the decline: %v", err)
	}

	now := afterEveryDueHour()
	if err := re.svc.Tick(schedulerPassCtx(re.wsID), now); err != nil {
		t.Fatalf("the scheduling pass: %v", err)
	}

	spec := grantedSpec(t)
	var seeded bool
	if err := owner.QueryRow(context.Background(),
		`SELECT EXISTS (SELECT 1 FROM runner_job WHERE trigger_ref = $1)`,
		spec.TriggerRef(now, colleague)).Scan(&seeded); err != nil {
		t.Fatalf("reading the declining rep's occurrence: %v", err)
	}
	if seeded {
		t.Error("a rep who declined was seeded an overnight run: the grant is not gating the fan-out, and the agent works for somebody who said no")
	}
	// The granting rep still got theirs, or the assertion above passes for the
	// wrong reason — a pass that seeded nobody satisfies it too.
	assertSpecSeededFor(t, owner, grantedSpec(t), now, me)
}

// TestAWorkspaceWhereNobodyGrantedSeedsNothing is the empty case, and it is a
// PASS rather than a fault: no live grant means nobody has said yes yet.
func TestAWorkspaceWhereNobodyGrantedSeedsNothing(t *testing.T) {
	re := setupRunner(t)
	owner := integration.OwnerConn(t)

	now := afterEveryDueHour()
	if err := re.svc.Tick(schedulerPassCtx(re.wsID), now); err != nil {
		t.Fatalf("a pass over a workspace where nobody granted must succeed, not fault: %v", err)
	}

	var jobs int
	if err := owner.QueryRow(context.Background(),
		`SELECT count(*) FROM runner_job WHERE agent_spec = $1`, grantSpec).Scan(&jobs); err != nil {
		t.Fatalf("counting the seeded occurrences: %v", err)
	}
	if jobs != 0 {
		t.Errorf("%d occurrence(s) were seeded with no live grant behind them — a run nothing can authorize, which fails in the small hours with nobody watching", jobs)
	}
}

// TestASecondPassSeedsNoSecondRunForTheSameSeat keeps the idempotency the seat
// segment could have broken. Uniqueness is per seat per day now, and a ref that
// varied per pass would queue a rep the same brief every tick.
func TestASecondPassSeedsNoSecondRunForTheSameSeat(t *testing.T) {
	re := setupRunner(t)
	owner := integration.OwnerConn(t)

	me := re.sessionUser(t)
	if err := re.recordDecision(t, me, runner.GrantStateGranted, &re.passportID); err != nil {
		t.Fatalf("record the grant for %s: %v", me, err)
	}

	now := afterEveryDueHour()
	for pass := 0; pass < 2; pass++ {
		if err := re.svc.Tick(schedulerPassCtx(re.wsID), now); err != nil {
			t.Fatalf("scheduling pass %d: %v", pass+1, err)
		}
	}

	var jobs int
	if err := owner.QueryRow(context.Background(),
		`SELECT count(*) FROM runner_job WHERE agent_spec = $1 AND trigger_ref = $2`,
		grantSpec, grantedSpec(t).TriggerRef(now, me)).Scan(&jobs); err != nil {
		t.Fatalf("counting this seat's occurrences: %v", err)
	}
	if jobs != 1 {
		t.Errorf("this seat holds %d occurrences of one day's brief, want exactly 1 — a rep queued the same brief every tick", jobs)
	}
}

// seedColleague adds a second real seat to the workspace.
func seedColleague(t *testing.T, owner *pgx.Conn) ids.UserID {
	t.Helper()
	var raw string
	if err := owner.QueryRow(context.Background(), `
		INSERT INTO app_user (email, display_name, status)
		VALUES ($1, 'Second Seat', 'active')
		RETURNING id::text`, "seat-"+ids.NewV7().String()+"@fable.test").Scan(&raw); err != nil {
		t.Fatalf("seeding a second seat: %v", err)
	}
	id, err := ids.ParseAs[ids.UserKind](raw)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// grantedSpec resolves the catalog entry these tests grant, so a test builds
// its trigger ref the way production does rather than formatting a second copy
// of the shape.
func grantedSpec(t *testing.T) runner.AgentSpec {
	t.Helper()
	for _, spec := range runner.Catalog() {
		if spec.Name == grantSpec {
			return spec
		}
	}
	t.Fatalf("no catalog entry named %s", grantSpec)
	return runner.AgentSpec{}
}
