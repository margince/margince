// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// CapturedDealRefileArgs freezes the pass's scope and horizon without correspondence.
type CapturedDealRefileArgs struct {
	Event       ids.UUID                `json:"event_id" river:"unique"`
	Correlation ids.UUID                `json:"correlation_id"`
	Scope       capture.DealRefileScope `json:"scope"`
	AsOf        time.Time               `json:"as_of"`
	Ceiling     ids.UUID                `json:"ceiling"`
}

// Kind selects the declared continuation queue.
func (CapturedDealRefileArgs) Kind() string { return "captured_deal_refile" }

// CapturedDealRefileSweepArgs recovers work whose original trigger did not finish.
type CapturedDealRefileSweepArgs struct{}

// Kind selects the declared recovery schedule.
func (CapturedDealRefileSweepArgs) Kind() string { return "captured_deal_refile_sweep" }

// FleetWide permits the scheduler to enumerate installations.
func (CapturedDealRefileSweepArgs) FleetWide() {}

// dealTopicCallsPerPass bounds the model calls one pass makes before it
// snoozes, so a pass stays inside its job timeout and the model's rate.
const dealTopicCallsPerPass = 20

type capturedDealRefileWorker struct {
	pool    *pgxpool.Pool
	db      *database.DB
	repair  capture.DealRepair
	cohorts *contacts.Store
	// closes reads and offers back the closes for silence the backfill finds
	// premature; closer notes the offer on the deal's timeline.
	closes *deals.Store
	closer silentDealCloser
	now    func() time.Time
	// page is how many candidates one pass inspects before it snoozes.
	page int
	// calls is how many topic checks one pass asks before it snoozes.
	calls int
}

// newCapturedDealRefileWorker builds the pass over the deal topic lane; a nil
// brain files no suggested deal, and a later pass with a model files it.
func newCapturedDealRefileWorker(pool *pgxpool.Pool, topicBrain completer) *capturedDealRefileWorker {
	db := InstallationDB(pool)
	repair := mailDealRepair(pool)
	repair.Check = NewDealTopicCheck(pool, topicBrain)
	return &capturedDealRefileWorker{
		pool: pool, db: db, repair: repair, cohorts: cohortRepairStore(contacts.NewStore(db)),
		closes: deals.NewStore(db, DealsInstallation()), closer: newSilentDealCloser(pool, slog.Default()),
		now:  func() time.Time { return time.Now().UTC() },
		page: capture.DealRefileLimit, calls: dealTopicCallsPerPass,
	}
}

type dealRefileCursor struct {
	After        ids.UUID `json:"deal_refile_after"`
	CohortsDone  bool     `json:"deal_refile_cohorts_done"`
	RefileDone   bool     `json:"deal_refile_done"`
	RecheckAfter ids.UUID `json:"deal_recheck_after"`
	RecheckDone  bool     `json:"deal_recheck_done"`
}

func (w *capturedDealRefileWorker) Work(ctx context.Context, job *river.Job[CapturedDealRefileArgs]) error {
	passCtx := ctx
	passCtx = principal.WithCorrelationID(principal.SystemActing(passCtx, dealRefileActor), job.Args.Correlation)
	if !job.Args.Event.IsZero() {
		passCtx = principal.WithCausationEvent(passCtx, job.Args.Event)
	}
	if job.Args.AsOf.IsZero() || job.Args.Correlation.IsZero() {
		return jobs.FaultContext(ctx, apperrors.ErrInvalidArgument)
	}
	// A workspace with no live activity has a zero ceiling and nothing to
	// refile, but the backfill must still run its phases to finish.
	if job.Args.Ceiling.IsZero() && !job.Args.Scope.Backfill {
		return nil
	}
	var cursor dealRefileCursor
	if len(job.Metadata) > 0 {
		if err := json.Unmarshal(job.Metadata, &cursor); err != nil {
			return jobs.FaultContext(ctx, err)
		}
	}
	if !cursor.CohortsDone {
		changed, err := w.repairCohorts(passCtx, job.Args.Scope.Contacts)
		if err != nil {
			return jobs.FaultContext(ctx, err)
		}
		if changed {
			return river.JobSnooze(0)
		}
		if err := river.MetadataSet(ctx, "deal_refile_cohorts_done", true); err != nil {
			return jobs.FaultContext(ctx, err)
		}
	}
	if cursor.RecheckDone {
		more, err := w.backfill(passCtx, job.Args)
		if err != nil {
			return jobs.FaultContext(ctx, err)
		}
		if more {
			return river.JobSnooze(0)
		}
		return nil
	}
	// The refile pages run first, then, for the recovery sweep, the recheck
	// pages. Each phase keeps its own cursor across snoozes.
	page, afterKey, after := w.refilePage, "deal_refile_after", cursor.After
	if cursor.RefileDone {
		page, afterKey, after = w.recheckPage, "deal_recheck_after", cursor.RecheckAfter
	}
	next, more, err := page(passCtx, job.Args, after)
	if err != nil {
		return jobs.FaultContext(ctx, err)
	}
	backfillDone := false
	switch {
	case more:
		// A crash before metadata is saved replays completed writes under their normal locks.
		err = river.MetadataSet(ctx, afterKey, next)
	case !cursor.RefileDone && job.Args.Scope.Recheck:
		err = river.MetadataSet(ctx, "deal_refile_done", true)
	default:
		if job.Args.Scope.Backfill {
			if err := river.MetadataSet(ctx, "deal_recheck_done", true); err != nil {
				return jobs.FaultContext(ctx, err)
			}
		}
		var left bool
		left, err = w.backfill(passCtx, job.Args)
		backfillDone = !left
	}
	if err != nil {
		return jobs.FaultContext(ctx, err)
	}
	if backfillDone {
		return nil
	}
	return river.JobSnooze(0)
}

func (w *capturedDealRefileWorker) repairCohorts(ctx context.Context, contactIDs []ids.UUID) (bool, error) {
	changed := false
	for _, contact := range contactIDs {
		err := w.db.Tx(ctx, func(tx pgx.Tx) error {
			done, err := w.cohorts.PromoteContactCohortTx(ctx, tx, ids.From[ids.ContactKind](contact))
			changed = changed || done.Linked > 0 || done.Promoted > 0
			return err
		})
		if errors.Is(err, apperrors.ErrNotFound) {
			continue
		}
		if err != nil {
			return false, err
		}
	}
	return changed, nil
}

// repairAt is the repair with its "Which deal?" question judged at the pass's
// frozen instant, the same horizon the pass files by.
func (w *capturedDealRefileWorker) repairAt(asOf time.Time) capture.DealRepair {
	repair := w.repair
	repair.Ask = newWhichDealAsker(w.pool, func() time.Time { return asOf }).Ask
	return repair
}

func (w *capturedDealRefileWorker) refilePage(ctx context.Context, args CapturedDealRefileArgs, after ids.UUID) (ids.UUID, bool, error) {
	var scanned []ids.UUID
	err := w.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		scanned, err = w.repair.RefileCandidatesTx(ctx, tx, args.Scope, args.AsOf, after, args.Ceiling, w.page)
		return err
	})
	if err != nil {
		return after, false, err
	}
	return w.walk(ctx, scanned, after, "capture_deal_refile_fault", func(activity ids.ActivityID) (bool, error) {
		return w.repairAt(args.AsOf).Refile(ctx, w.db, activity, args.AsOf)
	})
}

// recheckPage asks the topic check about one page of the rule's older links
// from the last capture.DealRecheckWindowDays days.
func (w *capturedDealRefileWorker) recheckPage(ctx context.Context, args CapturedDealRefileArgs, after ids.UUID) (ids.UUID, bool, error) {
	var scanned []ids.UUID
	err := w.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		scanned, err = w.repair.RecheckCandidatesTx(ctx, tx, args.AsOf, after, args.Ceiling, w.page)
		return err
	})
	if err != nil {
		return after, false, err
	}
	return w.walk(ctx, scanned, after, "capture_deal_recheck_fault", func(activity ids.ActivityID) (bool, error) {
		return w.repair.Recheck(ctx, w.db, activity)
	})
}

// walk files each scanned activity, logging a row fault and going on. It
// stops early once the pass has asked its share of topic checks, and answers
// where to resume and whether work remains.
func (w *capturedDealRefileWorker) walk(
	ctx context.Context, scanned []ids.UUID, after ids.UUID, fault string, file func(ids.ActivityID) (bool, error),
) (ids.UUID, bool, error) {
	asked := 0
	for _, activity := range scanned {
		called, err := file(ids.From[ids.ActivityKind](activity))
		if err != nil {
			if logErr := w.db.Tx(ctx, func(tx pgx.Tx) error {
				_, logErr := storekit.LogSystem(ctx, tx, fault, map[string]any{"activity_id": activity, "error": err.Error()})
				return logErr
			}); logErr != nil {
				return after, false, errors.Join(err, logErr)
			}
		}
		if called {
			asked++
		}
		if asked >= w.calls {
			return activity, true, nil
		}
	}
	if len(scanned) == 0 {
		return after, false, nil
	}
	return scanned[len(scanned)-1], len(scanned) == w.page, nil
}

type capturedDealRefileSweepWorker struct {
	pool *pgxpool.Pool
	now  func() time.Time
}

func (w *capturedDealRefileSweepWorker) Work(ctx context.Context, _ *river.Job[CapturedDealRefileSweepArgs]) error {
	return jobs.FaultContext(ctx, runInstallationPass(ctx, w.pool, w.scheduleWorkspace))
}

func (w *capturedDealRefileSweepWorker) scheduleWorkspace(ctx context.Context, workspace ids.UUID) error {
	ctx = principal.SystemActing(principal.WithWorkspaceID(ctx, workspace), dealRefileActor)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	var args CapturedDealRefileArgs
	err := InstallationDB(w.pool).Tx(ctx, func(tx pgx.Tx) error {
		var err error
		done, err := capture.StakeholderBackfillDoneTx(ctx, tx)
		if err != nil {
			return err
		}
		scope := capture.DealRefileScope{All: true, Recheck: true, Backfill: !done}
		args, err = dealRefilePassArgs(ctx, tx, scope, w.now(), ids.Nil)
		return err
	})
	if err != nil {
		return err
	}
	return dispatchOne(ctx, w.pool, args, nil)
}
