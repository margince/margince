// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// CapturedDealRefileArgs freezes the pass's scope and horizon without correspondence.
type CapturedDealRefileArgs struct {
	Workspace   ids.UUID                `json:"workspace_id" river:"unique"`
	Event       ids.UUID                `json:"event_id" river:"unique"`
	Correlation ids.UUID                `json:"correlation_id"`
	Scope       capture.DealRefileScope `json:"scope"`
	AsOf        time.Time               `json:"as_of"`
	Ceiling     ids.UUID                `json:"ceiling"`
}

// Kind selects the declared continuation queue.
func (CapturedDealRefileArgs) Kind() string { return "captured_deal_refile" }

// WorkspaceID binds the continuation to its installation.
func (a CapturedDealRefileArgs) WorkspaceID() ids.UUID { return a.Workspace }

// CapturedDealRefileSweepArgs recovers work whose original trigger did not finish.
type CapturedDealRefileSweepArgs struct{}

// Kind selects the declared recovery schedule.
func (CapturedDealRefileSweepArgs) Kind() string { return "captured_deal_refile_sweep" }

// FleetWide permits the scheduler to enumerate installations.
func (CapturedDealRefileSweepArgs) FleetWide() {}

type capturedDealRefileWorker struct {
	db      *database.DB
	repair  capture.DealRepair
	cohorts *contacts.Store
}

func newCapturedDealRefileWorker(pool *pgxpool.Pool) *capturedDealRefileWorker {
	db := InstallationDB(pool)
	return &capturedDealRefileWorker{db: db, repair: mailDealRepair(pool), cohorts: cohortRepairStore(contacts.NewStore(db))}
}

type dealRefileCursor struct {
	After       ids.UUID `json:"deal_refile_after"`
	CohortsDone bool     `json:"deal_refile_cohorts_done"`
}

func (w *capturedDealRefileWorker) Work(ctx context.Context, job *river.Job[CapturedDealRefileArgs]) error {
	passCtx, err := workspaceJobCtx(ctx, job.Args)
	if err != nil {
		return jobs.FaultContext(ctx, err)
	}
	passCtx = principal.WithCorrelationID(principal.SystemActing(passCtx, dealRefileActor), job.Args.Correlation)
	if !job.Args.Event.IsZero() {
		passCtx = principal.WithCausationEvent(passCtx, job.Args.Event)
	}
	if job.Args.AsOf.IsZero() || job.Args.Correlation.IsZero() {
		return jobs.FaultContext(ctx, apperrors.ErrInvalidArgument)
	}
	if job.Args.Ceiling.IsZero() {
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
	after, more, err := w.refilePage(passCtx, job.Args, cursor.After)
	if err != nil {
		return jobs.FaultContext(ctx, err)
	}
	if !more {
		return nil
	}
	// A crash before metadata is saved replays completed writes under their normal locks.
	if err := river.MetadataSet(ctx, "deal_refile_after", after); err != nil {
		return jobs.FaultContext(ctx, err)
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

func (w *capturedDealRefileWorker) refilePage(ctx context.Context, args CapturedDealRefileArgs, after ids.UUID) (ids.UUID, bool, error) {
	var scanned []ids.UUID
	err := w.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		scanned, err = w.repair.RefileCandidatesTx(ctx, tx, args.Scope, args.AsOf, after, args.Ceiling)
		return err
	})
	if err != nil {
		return after, false, err
	}
	for _, activity := range scanned {
		if err := w.db.Tx(ctx, func(tx pgx.Tx) error {
			_, err := w.repair.RefileTx(ctx, tx, ids.From[ids.ActivityKind](activity), args.AsOf)
			return err
		}); err != nil {
			if logErr := w.db.Tx(ctx, func(tx pgx.Tx) error {
				_, logErr := storekit.LogSystem(ctx, tx, "capture_deal_refile_fault", map[string]any{"activity_id": activity, "error": err.Error()})
				return logErr
			}); logErr != nil {
				return after, false, errors.Join(err, logErr)
			}
		}
	}
	if len(scanned) == 0 {
		return after, false, nil
	}
	return scanned[len(scanned)-1], len(scanned) == capture.DealRefileLimit, nil
}

type capturedDealRefileSweepWorker struct {
	pool *pgxpool.Pool
	now  func() time.Time
}

func (w *capturedDealRefileSweepWorker) Work(ctx context.Context, _ *river.Job[CapturedDealRefileSweepArgs]) error {
	return jobs.FaultContext(ctx, runPerWorkspace(ctx, w.pool, w.scheduleWorkspace))
}

func (w *capturedDealRefileSweepWorker) scheduleWorkspace(ctx context.Context, workspace ids.UUID) error {
	ctx = principal.SystemActing(principal.WithWorkspaceID(ctx, workspace), dealRefileActor)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	var args CapturedDealRefileArgs
	err := InstallationDB(w.pool).Tx(ctx, func(tx pgx.Tx) error {
		var err error
		args, err = dealRefilePassArgs(ctx, tx, capture.DealRefileScope{All: true}, w.now(), ids.Nil)
		return err
	})
	if err != nil {
		return err
	}
	return dispatchOne(ctx, args, nil)
}
