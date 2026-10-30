// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The input check over one changed deal. The freshness consumer queues it
// from the event the deal's writer staged, and its job runs it. Everything it
// clears is bounded to its deal.

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/jobs"
	kevents "github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// findingState is what a pass may change about one stored finding.
type findingState struct {
	ID       ids.UUID
	Status   string
	LastSeen time.Time
}

func (e *freshnessEnv) findingsOf(t *testing.T, deal ids.UUID) []findingState {
	t.Helper()
	var out []findingState
	if err := e.DB().Tx(e.Admin(), func(tx pgx.Tx) error {
		rows, err := tx.Query(e.Admin(), `SELECT id, status, last_seen_at FROM assurance_exception
			WHERE subject_id = $1 ORDER BY id`, deal)
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, pgx.RowToStructByPos[findingState])
		return err
	}); err != nil {
		t.Fatalf("reading the deal's findings: %v", err)
	}
	return out
}

func (e *freshnessEnv) rescan(t *testing.T, deal ids.UUID) error {
	t.Helper()
	worker := &assuranceDealRescanWorker{sweep: e.worker}
	return worker.Work(context.Background(), &river.Job[AssuranceDealRescanArgs]{
		Args: AssuranceDealRescanArgs{Workspace: e.WS, Deal: deal},
	})
}

func (e *freshnessEnv) queuedRescans(t *testing.T) int {
	t.Helper()
	return e.WsCount(t, `SELECT count(*) FROM river_job WHERE kind = 'assurance_deal_rescan' AND state = 'available'`)
}

// A one-deal pass clears that deal's finding and nothing of another deal's,
// and its run is not read as a reading of the pipeline.
func TestAOneDealPassCannotClearAnotherDealsFindings(t *testing.T) {
	e := setupFreshness(t)
	if _, err := e.Deals.UpdateDeal(e.Admin(), ids.From[ids.DealKind](e.healthy), deals.UpdateDealInput{
		ForecastCategory: new("commit"),
	}); err != nil {
		t.Fatalf("committing the other deal with no next step: %v", err)
	}
	if err := e.run(t); err != nil {
		t.Fatalf("the pass over every deal: %v", err)
	}
	before := e.findingsOf(t, e.healthy)
	if len(before) == 0 || len(e.findingsOf(t, e.faulted)) == 0 {
		t.Fatal("the fixture needs open findings on both deals")
	}
	latest, err := e.store.LatestRun(e.Admin())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Deals.UpdateDeal(e.Admin(), ids.From[ids.DealKind](e.faulted), deals.UpdateDealInput{
		ExpectedClose: new(e.at.AddDate(0, 0, 20)),
	}); err != nil {
		t.Fatalf("moving the close date: %v", err)
	}
	e.deliver(t, e.lastEnvelope(t, "deal.updated", e.faulted), e.editDeal(t, e.healthy, 4300000))

	if err := e.rescan(t, e.faulted); err != nil {
		t.Fatalf("the one-deal pass: %v", err)
	}

	if cleared := e.WsCount(t, `SELECT count(*) FROM assurance_exception
		WHERE subject_id = $1 AND type = $2 AND status = $3`,
		e.faulted, assurance.TypeClosePast, assurance.ExceptionConditionCleared); cleared != 1 {
		t.Fatalf("the pass over the faulted deal left its past close date open")
	}
	after := e.findingsOf(t, e.healthy)
	if len(after) != len(before) {
		t.Fatalf("the other deal had %d findings and now has %d", len(before), len(after))
	}
	for i := range before {
		if after[i] != before[i] {
			t.Errorf("a pass over another deal changed this one's finding: %+v became %+v", before[i], after[i])
		}
	}
	if f := e.freshness(t); f.ChangedDeals != 1 {
		t.Fatalf("freshness = %+v, want the other deal still out of date and this one read", f)
	}
	if now, err := e.store.LatestRun(e.Admin()); err != nil || now.ID != latest.ID {
		t.Fatalf("the latest reading of the pipeline is now %v (%v), want it unchanged", now.ID, err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM assurance_run WHERE subject_deal_id = $1 AND eligible_deals = 1`,
		e.faulted); n != 1 {
		t.Fatalf("%d runs name the deal they read, want 1", n)
	}
}

// A delivery seen twice queues one job, and a job run twice raises no second
// finding and records no second run.
func TestADuplicateDeliveryQueuesOneJobAndNoSecondFinding(t *testing.T) {
	e := setupFreshness(t)
	inserter, err := jobs.NewInserter(e.Pool, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	e.marker = e.marker.WithRescans(inserter)
	edit := e.editDeal(t, e.faulted, 3900000)
	e.deliver(t, edit)
	e.deliver(t, edit)
	if n := e.queuedRescans(t); n != 1 {
		t.Fatalf("two deliveries of one edit queued %d jobs, want 1", n)
	}

	for range 2 {
		if err := e.rescan(t, e.faulted); err != nil {
			t.Fatalf("the one-deal pass: %v", err)
		}
	}
	if n := e.WsCount(t, `SELECT count(*) FROM assurance_run WHERE subject_deal_id = $1`, e.faulted); n != 1 {
		t.Fatalf("a job run twice recorded %d runs, want 1", n)
	}
	if n := e.WsCount(t, `SELECT count(*) - count(DISTINCT logical_key) FROM assurance_exception`); n != 0 {
		t.Fatalf("%d findings were stored twice", n)
	}
}

// A change committed while the pass reads its deal is not the pass's to
// clear: nothing commits, and the retry reads the newest generation.
func TestAChangeDuringAOneDealPassIsReadByTheRetry(t *testing.T) {
	e := setupFreshness(t)
	e.deliver(t, e.editDeal(t, e.faulted, 4000000))
	runs := e.WsCount(t, `SELECT count(*) FROM assurance_run`)
	duringRead := func(ctx context.Context, tx pgx.Tx, asOf time.Time, coverage []assurance.SourceCoverage, deal ids.UUID) ([]assurance.Subject, error) {
		subjects, err := assuranceDealSubjects(ctx, tx, asOf, coverage, deal)
		e.deliver(t, e.editDeal(t, e.faulted, 4100000))
		return subjects, err
	}
	scanner := assuranceScanner(e.Pool, e.store).WithDealSubjects(duringRead)
	if _, err := scanner.ScanDeal(e.passCtx(), e.at, e.faulted); !errors.Is(err, assurance.ErrInputsMovedDuringPass) {
		t.Fatalf("a pass whose deal changed under it answered %v, want ErrInputsMovedDuringPass", err)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM assurance_run`); got != runs {
		t.Fatalf("the abandoned pass committed a run (%d runs, was %d)", got, runs)
	}

	if err := e.rescan(t, e.faulted); err != nil {
		t.Fatalf("the retry: %v", err)
	}
	if f := e.freshness(t); f.ChangedDeals != 0 {
		t.Fatalf("the retry left the deal out of date: %+v", f)
	}
}

// A burst of edits to one deal is one job, and a burst across many deals
// queues at most assuranceRescanBatchLimit jobs per read.
func TestABurstOfEditsQueuesBoundedJobs(t *testing.T) {
	e := setupFreshness(t)
	inserter, err := jobs.NewInserter(e.Pool, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	e.marker = e.marker.WithRescans(inserter)
	burst := make([]kevents.Envelope, 0, 20)
	for i := range 20 {
		burst = append(burst, e.editDeal(t, e.faulted, int64(4200000+i)))
	}
	e.deliver(t, burst...)
	if n := e.queuedRescans(t); n != 1 {
		t.Fatalf("twenty edits to one deal queued %d jobs, want 1", n)
	}

	wide := make([]kevents.Envelope, 0, assuranceRescanBatchLimit+10)
	for range assuranceRescanBatchLimit + 10 {
		env := burst[0]
		env.Entity.ID = ids.NewV7()
		wide = append(wide, env)
	}
	e.deliver(t, wide...)
	if n := e.queuedRescans(t); n != 1+assuranceRescanBatchLimit {
		t.Fatalf("a burst across %d deals queued %d jobs, want %d", len(wide), n-1, assuranceRescanBatchLimit)
	}
}

// A finding whose deal row is gone closes as departed. The tree has no writer
// that deletes a deal yet, so the row is removed directly, as erasure would.
func TestAFindingWhoseDealIsGoneCloses(t *testing.T) {
	e := setupFreshness(t)
	if len(e.findingsOf(t, e.faulted)) == 0 {
		t.Fatal("the fixture needs findings on the faulted deal")
	}
	if _, err := integration.OwnerConn(t).Exec(context.Background(), `
		DELETE FROM deal WHERE id = $1`, e.faulted); err != nil {
		t.Fatalf("removing the deal: %v", err)
	}
	if err := e.run(t); err != nil {
		t.Fatalf("the pass: %v", err)
	}
	for _, f := range e.findingsOf(t, e.faulted) {
		if f.Status != assurance.ExceptionSubjectDeparted {
			t.Errorf("finding %s of a deal that is gone is %q, want departed", f.ID, f.Status)
		}
	}
}

// A change committed after the pass cleared its marker, while the job is
// still finishing, is deduplicated against the running job. The job sees the
// marker again and asks to run again rather than leave it to the night.
func TestAChangeAsTheJobFinishesRunsItAgain(t *testing.T) {
	e := setupFreshness(t)
	e.deliver(t, e.editDeal(t, e.faulted, 4400000))
	worker := &assuranceDealRescanWorker{
		sweep: e.worker,
		scan: func(ctx context.Context, store *assurance.Store, now time.Time, deal ids.UUID) (assurance.Result, error) {
			result, err := e.worker.scanDeal(ctx, store, now, deal)
			e.deliver(t, e.editDeal(t, e.faulted, 4500000))
			return result, err
		},
	}
	err := worker.Work(context.Background(), &river.Job[AssuranceDealRescanArgs]{
		Args: AssuranceDealRescanArgs{Workspace: e.WS, Deal: e.faulted},
	})
	if _, snoozed := errors.AsType[*rivertype.JobSnoozeError](err); !snoozed {
		t.Fatalf("a job whose deal changed as it finished answered %v, want a snooze", err)
	}
	if err := e.rescan(t, e.faulted); err != nil {
		t.Fatalf("the rerun: %v", err)
	}
	if f := e.freshness(t); f.ChangedDeals != 0 {
		t.Fatalf("the rerun left the deal out of date: %+v", f)
	}
}

// Sustained edits stop queueing once a workspace has
// assuranceRescanBacklogLimit checks waiting; the nightly pass reads the rest.
func TestTheWaitingDealChecksHaveACeiling(t *testing.T) {
	e := setupFreshness(t)
	inserter, err := jobs.NewInserter(e.Pool, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	e.marker = e.marker.WithRescans(inserter)
	template := e.editDeal(t, e.faulted, 4600000)
	for range assuranceRescanBacklogLimit/assuranceRescanBatchLimit + 2 {
		wide := make([]kevents.Envelope, 0, assuranceRescanBatchLimit)
		for range assuranceRescanBatchLimit {
			env := template
			env.Entity.ID = ids.NewV7()
			wide = append(wide, env)
		}
		e.deliver(t, wide...)
	}
	if n := e.queuedRescans(t); n != assuranceRescanBacklogLimit {
		t.Fatalf("sustained edits queued %d checks, want the ceiling of %d", n, assuranceRescanBacklogLimit)
	}
}
