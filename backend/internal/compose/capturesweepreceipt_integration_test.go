// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The receipts capture's repair passes leave, driven through the real workers.
//
// A receipt is what the capture-health page reads to say when a pass last
// succeeded, so each case here runs the job's own per-workspace turn and reads
// back the row it wrote: an outcome the page would misreport is a pass an
// administrator cannot see stalling.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type storedReceipt struct {
	outcome    string
	processed  int
	capHit     bool
	errorClass string
}

// receiptsOf reads one pass's receipts, newest first.
func receiptsOf(t *testing.T, e *integration.Env, sweep capture.Sweep) []storedReceipt {
	t.Helper()
	var out []storedReceipt
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(context.Background(), `
			SELECT outcome, processed, cap_hit, coalesce(error_class, '')
			  FROM capture_sweep_run WHERE sweep = $1
			 ORDER BY finished_at DESC, id DESC`, string(sweep))
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var r storedReceipt
			if err := rows.Scan(&r.outcome, &r.processed, &r.capHit, &r.errorClass); err != nil {
				return err
			}
			out = append(out, r)
		}
		return rows.Err()
	}); err != nil {
		t.Fatalf("reading the %s receipts: %v", sweep, err)
	}
	return out
}

func latestReceipt(t *testing.T, e *integration.Env, sweep capture.Sweep) storedReceipt {
	t.Helper()
	got := receiptsOf(t, e, sweep)
	if len(got) == 0 {
		t.Fatalf("the %s pass left no receipt", sweep)
	}
	return got[0]
}

func newTestLinkReconcileWorker(e *integration.Env) *linkReconcileWorker {
	return newLinkReconcileWorker(e.Pool,
		contacts.NewStore(InstallationDB(e.Pool)).WithAudienceRecompute(activities.RecomputeAudienceTx),
		slog.Default())
}

func newTestConfidentialityWorker(e *integration.Env) *confidentialityVerdictWorker {
	return &confidentialityVerdictWorker{
		pool:     e.Pool,
		engine:   NewConfidentialityVerdictEngine(e.Pool, nil, slog.Default()),
		receipts: newSweepRecorder(e.Pool),
	}
}

// seedFiledHeldMeetings writes n captured meetings filed under a contact and
// still held as naming nobody. Today's filing re-derives the audience as it
// links, so no writer produces this state any more; it is the legacy shape the
// pass exists to drain, written as it was left.
func seedFiledHeldMeetings(t *testing.T, e *integration.Env, n int) {
	t.Helper()
	attendee := e.SeedContact(t, "Filed Attendee", &e.Rep1)
	e.WsExec(t, `
		WITH a AS (
		  INSERT INTO activity (id, kind, subject, body, occurred_at, source, captured_by,
		                        source_system, source_id, audience, audience_reason)
		  SELECT uuidv7(), 'meeting', 'Board review: severance terms', 'confidential agenda',
		         now() - make_interval(mins => g), 'gcal:seed', 'connector:gcal',
		         'gcal', 'held-' || gen_random_uuid(), 'participants', $1
		    FROM generate_series(1, $2::int) g
		  RETURNING id),
		linked AS (
		  INSERT INTO activity_link (activity_id, entity_type, contact_id)
		  SELECT id, 'contact', $3 FROM a),
		attended AS (
		  INSERT INTO activity_participant (id, activity_id, address, role, contact_id)
		  SELECT uuidv7(), id, 'attendee@filed.test', 'attendee', $3 FROM a)
		INSERT INTO capture_import (activity_id, user_id, posture_at_import)
		SELECT id, $4, 'shared' FROM a`,
		activities.ReasonNoCounterparty, n, attendee, e.Rep1)
}

func readHealth(t *testing.T, e *integration.Env) crmcontracts.CaptureHealth {
	t.Helper()
	var health crmcontracts.CaptureHealth
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		var err error
		health, err = readCaptureHealth(context.Background(), tx, time.Now().UTC())
		return err
	}); err != nil {
		t.Fatalf("reading capture health: %v", err)
	}
	return health
}

func heldMeetingCount(t *testing.T, e *integration.Env) int {
	t.Helper()
	return readHealth(t, e).HeldMeetings.Count
}

// refuseUpdates makes every UPDATE of table that matches `when` fail, for the
// rest of the test: the real statement failing the way a lock or a constraint
// would, rather than a double standing in for the store.
func refuseUpdates(t *testing.T, table, when string) {
	t.Helper()
	owner := integration.OwnerConn(t)
	name := "refuse_" + table
	ctx := context.Background()
	if _, err := owner.Exec(ctx, fmt.Sprintf(`
		CREATE FUNCTION %[1]s() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN RAISE EXCEPTION 'refused by the test'; END $$;
		CREATE TRIGGER %[1]s BEFORE UPDATE ON %[2]s FOR EACH ROW WHEN (%[3]s)
		EXECUTE FUNCTION %[1]s();`, name, table, when)); err != nil {
		t.Fatalf("installing the refusal on %s: %v", table, err)
	}
	t.Cleanup(func() {
		if _, err := owner.Exec(ctx, fmt.Sprintf(
			`DROP TRIGGER %[1]s ON %[2]s; DROP FUNCTION %[1]s();`, name, table)); err != nil {
			t.Errorf("removing the refusal on %s: %v", table, err)
		}
	})
}

func TestEveryPassLeavesAnOKReceiptForEachWorkspace(t *testing.T) {
	e := integration.Setup(t)
	seedFiledHeldMeetings(t, e, 1)
	ctx := context.Background()

	if err := newTestConfidentialityWorker(e).Work(ctx, &river.Job[ConfidentialityVerdictArgs]{}); err != nil {
		t.Fatalf("the confidentiality pass: %v", err)
	}
	if err := newTestLinkReconcileWorker(e).Work(ctx, &river.Job[LinkReconcileArgs]{}); err != nil {
		t.Fatalf("the link reconcile: %v", err)
	}

	workspaces, err := enumerateWorkspaces(ctx, e.Pool)
	if err != nil {
		t.Fatal(err)
	}
	for _, sweep := range capture.Sweeps() {
		got := receiptsOf(t, e, sweep)
		if len(got) != len(workspaces) {
			t.Fatalf("%s left %d receipts over %d workspaces, want one per workspace turn",
				sweep, len(got), len(workspaces))
		}
		if got[0].outcome != string(capture.SweepOK) || got[0].errorClass != "" || got[0].capHit {
			t.Errorf("%s receipt = %+v, want a clean ok", sweep, got[0])
		}
	}
	if got := latestReceipt(t, e, capture.SweepFiledMeetingHolds).processed; got != 1 {
		t.Errorf("the meeting pass reports %d lifted, want the one it lifted", got)
	}
}

func TestAFailedPassRecordsItsClassAndWhatItCommitted(t *testing.T) {
	e := integration.Setup(t)
	seedFiledHeldMeetings(t, e, 2)
	// The pass lifts newest first; the older meeting's write is refused, so
	// one lift commits before the failure.
	refuseUpdates(t, "activity",
		`OLD.kind = 'meeting' AND OLD.occurred_at < now() - interval '90 seconds' AND OLD.audience_reason = 'no_counterparty'`)

	err := newTestLinkReconcileWorker(e).reconcileLinksForWorkspace(context.Background(), e.WS)
	if err == nil {
		t.Fatal("the turn reported success over a refused lift")
	}
	got := latestReceipt(t, e, capture.SweepFiledMeetingHolds)
	if got.outcome != string(capture.SweepFailed) || got.errorClass != unclassifiedSweepFailure {
		t.Fatalf("receipt = %+v, want failed with the unclassified class — a database refusal is "+
			"not in the job fault vocabulary, and its text must not be stored", got)
	}
	if got.processed != 1 {
		t.Errorf("receipt processed = %d, want the 1 lift committed before the failure", got.processed)
	}
	// The stages are independent: the next one still ran and says so.
	if next := latestReceipt(t, e, capture.SweepStrandedContacts); next.outcome != string(capture.SweepOK) {
		t.Errorf("the stranded-contact pass after a failed meeting pass = %+v, want ok", next)
	}
}

func TestAPassBehindAFailedStageIsRecordedSkipped(t *testing.T) {
	e := integration.Setup(t)
	// A thread that spent its attempts, so the retiring stage has a row to
	// write — and the write is refused.
	e.WsExec(t, `
		INSERT INTO capture_thread_verdict (thread_key, user_id, status, attempts, updated_at)
		VALUES ('thread:spent', $1, 'pending', $2, now() - interval '1 hour')`,
		e.Rep1, capture.ThreadVerdictMaxAttempts)
	refuseUpdates(t, "capture_thread_verdict", "true")

	w := newTestConfidentialityWorker(e)
	if err := w.judgeWorkspace(context.Background(), e.WS); err == nil {
		t.Fatal("the turn reported success over a refused retirement")
	}
	got := latestReceipt(t, e, capture.SweepSettledThreadVerdicts)
	if got.outcome != string(capture.SweepSkipped) || got.processed != 0 || got.errorClass != "" {
		t.Errorf("receipt = %+v, want skipped with nothing processed", got)
	}
}

// A turn cut off before its first stage records both passes behind it as
// skipped — written after the cancellation, because a pass that ran out of
// time is the one an administrator most needs to see.
func TestATurnCancelledBeforeItsFirstStageRecordsItsPassesSkipped(t *testing.T) {
	e := integration.Setup(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := newTestLinkReconcileWorker(e).reconcileLinksForWorkspace(ctx, e.WS); err == nil {
		t.Fatal("a cancelled turn reported success")
	}
	for _, sweep := range []capture.Sweep{capture.SweepFiledMeetingHolds, capture.SweepStrandedContacts} {
		if got := latestReceipt(t, e, sweep); got.outcome != string(capture.SweepSkipped) {
			t.Errorf("%s receipt = %+v, want skipped", sweep, got)
		}
	}
}

func TestAPassStoppedAtItsBoundIsPartialAndTheCountSeesPastIt(t *testing.T) {
	e := integration.Setup(t)
	seedFiledHeldMeetings(t, e, liftFiledMeetingHoldsPerTick+1)

	if got := heldMeetingCount(t, e); got != liftFiledMeetingHoldsPerTick+1 {
		t.Fatalf("held meetings = %d, want %d — the page counts without the pass's bound",
			got, liftFiledMeetingHoldsPerTick+1)
	}
	w := newTestLinkReconcileWorker(e)
	if err := w.reconcileLinksForWorkspace(context.Background(), e.WS); err != nil {
		t.Fatalf("first turn: %v", err)
	}
	first := latestReceipt(t, e, capture.SweepFiledMeetingHolds)
	if first.outcome != string(capture.SweepPartial) || !first.capHit ||
		first.processed != liftFiledMeetingHoldsPerTick {
		t.Fatalf("first receipt = %+v, want partial at the bound with %d lifted",
			first, liftFiledMeetingHoldsPerTick)
	}
	if got := heldMeetingCount(t, e); got != 1 {
		t.Fatalf("held meetings after one turn = %d, want the 1 past the bound", got)
	}

	if err := w.reconcileLinksForWorkspace(context.Background(), e.WS); err != nil {
		t.Fatalf("second turn: %v", err)
	}
	if got := latestReceipt(t, e, capture.SweepFiledMeetingHolds); got.outcome != string(capture.SweepOK) {
		t.Errorf("second receipt = %+v, want ok once the backlog is gone", got)
	}
	if got := heldMeetingCount(t, e); got != 0 {
		t.Errorf("held meetings after the drain = %d, want 0", got)
	}
}

// refusingFirstReceipt refuses the first receipt write and passes the rest to
// the real ledger.
type refusingFirstReceipt struct {
	real  sweepLedger
	calls int
}

var errReceiptRefused = errors.New("receipt refused")

func (l *refusingFirstReceipt) RecordSweep(ctx context.Context, r capture.SweepReceipt) error {
	l.calls++
	if l.calls == 1 {
		return errReceiptRefused
	}
	return l.real.RecordSweep(ctx, r)
}

func TestAReceiptThatCannotBeWrittenIsReportedAndTheNextWorkspaceRuns(t *testing.T) {
	e := integration.Setup(t)
	w := newTestConfidentialityWorker(e)
	w.receipts.ledger = &refusingFirstReceipt{real: capture.NewSweepLedger(InstallationDB(e.Pool))}

	// Two turns through the fleet walk the job uses. An installation holds one
	// workspace, so the same one stands in for the second.
	err := runEach(context.Background(), []ids.UUID{e.WS, e.WS}, w.judgeWorkspace)
	if !errors.Is(err, errReceiptRefused) {
		t.Fatalf("the walk returned %v, want the refused receipt reported", err)
	}
	if got := receiptsOf(t, e, capture.SweepSettledThreadVerdicts); len(got) != 1 ||
		got[0].outcome != string(capture.SweepOK) {
		t.Errorf("receipts = %+v, want the second turn's ok — one refused receipt must not stop the walk", got)
	}
}

func TestPruningKeepsTheLastSuccessThroughAnyRunOfFailures(t *testing.T) {
	e := integration.Setup(t)
	ledger := capture.NewSweepLedger(InstallationDB(e.Pool))
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	start := time.Now().Add(-time.Hour)
	record := func(i int, outcome capture.SweepOutcome, class string) {
		at := start.Add(time.Duration(i) * time.Second)
		if err := ledger.RecordSweep(ctx, capture.SweepReceipt{
			Sweep: capture.SweepStrandedContacts, StartedAt: at, FinishedAt: at,
			Outcome: outcome, ErrorClass: class,
		}); err != nil {
			t.Fatalf("recording receipt %d: %v", i, err)
		}
	}
	record(0, capture.SweepOK, "")
	failures := capture.SweepReceiptsKept + 20
	for i := 1; i <= failures; i++ {
		record(i, capture.SweepFailed, "write_conflict")
	}

	got := receiptsOf(t, e, capture.SweepStrandedContacts)
	if len(got) != capture.SweepReceiptsKept+1 {
		t.Errorf("history holds %d receipts, want the %d newest plus the last success",
			len(got), capture.SweepReceiptsKept)
	}
	if last := got[len(got)-1]; last.outcome != string(capture.SweepOK) {
		t.Errorf("oldest kept receipt = %+v, want the last success", last)
	}
	var state capture.SweepState
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		states, err := capture.SweepStatesTx(context.Background(), tx)
		for _, s := range states {
			if s.Sweep == capture.SweepStrandedContacts {
				state = s
			}
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if state.LastSucceededAt == nil || state.Latest == nil || state.Latest.Outcome != capture.SweepFailed {
		t.Errorf("state = %+v, want the latest failure and the last success both reported", state)
	}
}

func TestEverySweepAndOutcomeIsAcceptedByTheTable(t *testing.T) {
	e := integration.Setup(t)
	ledger := capture.NewSweepLedger(InstallationDB(e.Pool))
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	now := time.Now()
	for _, sweep := range capture.Sweeps() {
		for _, receipt := range []capture.SweepReceipt{
			sweepReceiptFor(sweep, now, now, sweepTally{processed: 3}, nil),
			sweepReceiptFor(sweep, now, now, sweepTally{processed: 3, capHit: true}, nil),
			sweepReceiptFor(sweep, now, now, sweepTally{processed: 1}, errors.New("boom")),
			{Sweep: sweep, StartedAt: now, FinishedAt: now, Outcome: capture.SweepSkipped},
		} {
			if err := ledger.RecordSweep(ctx, receipt); err != nil {
				t.Errorf("the table refused %s/%s: %v", sweep, receipt.Outcome, err)
			}
			if !crmcontracts.CaptureSweepRunOutcome(receipt.Outcome).Valid() {
				t.Errorf("%s is not in the contract's outcome enum", receipt.Outcome)
			}
		}
		if !crmcontracts.CaptureSweepHealthSweep(sweep).Valid() {
			t.Errorf("%s is not in the contract's sweep enum", sweep)
		}
	}
}

// A pass that panics has committed what it committed. The receipt says it
// failed and how far it got, and the panic still reaches River.
func TestAPanickingPassLeavesAFailedReceiptAndStillPanics(t *testing.T) {
	e := integration.Setup(t)
	recorder := newSweepRecorder(e.Pool)
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)

	func() {
		defer func() {
			if recover() == nil {
				t.Error("the panic was absorbed; River must still see it")
			}
		}()
		_, _ = recorder.run(ctx, capture.SweepFiledMeetingHolds, func(tally *sweepTally) error {
			tally.processed = 2
			panic("a pass fell over")
		})
	}()

	got := latestReceipt(t, e, capture.SweepFiledMeetingHolds)
	if got.outcome != string(capture.SweepFailed) || got.errorClass != panickedSweepFailure ||
		got.processed != 2 {
		t.Errorf("receipt = %+v, want failed/panicked with the 2 committed before the panic", got)
	}
}
