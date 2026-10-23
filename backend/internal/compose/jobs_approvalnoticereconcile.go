// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const (
	approvalNoticePageSize       = 200
	approvalNoticeReconcileActor = "system:approval-notice-reconcile"
)

// ApprovalNoticeReconcileArgs resumes a bounded sweep through job metadata.
type ApprovalNoticeReconcileArgs struct{}

// Kind selects the declared reconciliation queue policy.
func (ApprovalNoticeReconcileArgs) Kind() string { return "approval_notice_reconcile" }

// InsertOpts keeps one scan active while its pages yield.
func (ApprovalNoticeReconcileArgs) InsertOpts() river.InsertOpts {
	return *jobs.QueuedAs[ApprovalNoticeReconcileArgs](&river.InsertOpts{
		MaxAttempts: 3,
		UniqueOpts:  river.UniqueOpts{ByState: activeSweepStates},
	})
}

type approvalNoticeReconcileWorker struct {
	db        *database.DB
	identity  *identity.Service
	approvals *approvals.Service
	notices   *notices.Store
	retract   *ApprovalNoticeRetract
}

func newApprovalNoticeReconcileWorker(pool *pgxpool.Pool) *approvalNoticeReconcileWorker {
	db := InstallationDB(pool)
	ident := identity.NewService(pool)
	store := notices.NewStore(db)
	return &approvalNoticeReconcileWorker{
		db: db, identity: ident,
		approvals: NewApprovalsService(db), notices: store, retract: NewApprovalNoticeRetract(pool, store, ident, slog.Default()),
	}
}

func (w *approvalNoticeReconcileWorker) Work(ctx context.Context, job *river.Job[ApprovalNoticeReconcileArgs]) error {
	passCtx, err := installationJobCtx(ctx, w.identity)
	if err != nil {
		return jobs.FaultContext(ctx, err)
	}
	passCtx = principal.SystemActing(passCtx, approvalNoticeReconcileActor)
	var cursor struct {
		After ids.UUID `json:"approval_notice_after"`
	}
	if len(job.Metadata) > 0 {
		if err := json.Unmarshal(job.Metadata, &cursor); err != nil {
			return jobs.FaultContext(ctx, err)
		}
	}
	after, _, err := w.reconcilePage(passCtx, cursor.After)
	if err != nil {
		return jobs.FaultContext(ctx, err)
	}
	if after.IsZero() {
		return nil
	}
	// Persist after commit: a crash may replay a page, but cannot skip its retractions.
	if err := river.MetadataSet(ctx, "approval_notice_after", after); err != nil {
		return jobs.FaultContext(ctx, err)
	}
	return river.JobSnooze(time.Second)
}

// reconcilePage retracts every copy of a settled decision. It also retracts a
// copy of a pending one sent to a member it is not asked of.
func (w *approvalNoticeReconcileWorker) reconcilePage(ctx context.Context, after ids.UUID) (ids.UUID, int64, error) {
	var page notices.OutstandingApprovalPage
	var pending map[ids.ApprovalID]bool
	var asked map[approvals.AskedPair]bool
	err := w.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		page, err = w.notices.OutstandingApprovalNotices(ctx, tx, after, approvalNoticePageSize)
		if err != nil {
			return err
		}
		if pending, err = w.approvals.StillPending(ctx, tx, page.ApprovalIDs); err != nil {
			return err
		}
		asked, err = w.approvals.StillAskedOf(ctx, tx, askedPairs(page.Notices))
		return err
	})
	if err != nil {
		return ids.Nil, 0, err
	}
	var retracted int64
	for id, remains := range pending {
		if remains {
			continue
		}
		count, err := w.retract.retract(ctx, id.UUID)
		if err != nil {
			return ids.Nil, retracted, err
		}
		retracted += count
	}
	for _, held := range page.Notices {
		if !pending[held.Approval] || asked[approvals.AskedPair{Approval: held.Approval, Recipient: held.Recipient}] {
			continue
		}
		count, err := w.retractUnasked(ctx, held)
		if err != nil {
			return ids.Nil, retracted, err
		}
		retracted += count
	}
	return page.AfterID, retracted, nil
}

// retractUnasked takes a pending approval's copy back from its recipient, asked
// again under the approval's lock in the same transaction. A request handed back
// to this recipient since the page was read keeps its copy.
func (w *approvalNoticeReconcileWorker) retractUnasked(ctx context.Context, held notices.OutstandingApprovalNotice) (int64, error) {
	pair := approvals.AskedPair{Approval: held.Approval, Recipient: held.Recipient}
	var taken int64
	err := w.db.Tx(ctx, func(tx pgx.Tx) error {
		asked, err := w.approvals.StillAskedOf(ctx, tx, []approvals.AskedPair{pair})
		if err != nil || asked[pair] {
			return err
		}
		taken, err = w.notices.RetractCopy(ctx, tx, held.ID)
		return err
	})
	if err != nil {
		return 0, fmt.Errorf("approval notice reconcile: %w", err)
	}
	return taken, nil
}

func askedPairs(copies []notices.OutstandingApprovalNotice) []approvals.AskedPair {
	pairs := make([]approvals.AskedPair, 0, len(copies))
	for _, held := range copies {
		pairs = append(pairs, approvals.AskedPair{Approval: held.Approval, Recipient: held.Recipient})
	}
	return pairs
}
