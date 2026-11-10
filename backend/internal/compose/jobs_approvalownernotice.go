// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ApprovalOwnerNoticeArgs identifies the request and recipient to recheck.
type ApprovalOwnerNoticeArgs struct {
	Workspace  ids.UUID `json:"workspace_id"`
	ApprovalID ids.UUID `json:"approval_id"`
	DeciderID  ids.UUID `json:"decider_id"`
}

// Kind identifies persisted owner-announcement jobs.
func (ApprovalOwnerNoticeArgs) Kind() string { return "approval_owner_notice" }

// WorkspaceID binds the announcement to the workspace named at handover.
func (a ApprovalOwnerNoticeArgs) WorkspaceID() ids.UUID { return a.Workspace }

// InsertOpts keeps each handover distinct while an earlier announcement completes.
func (ApprovalOwnerNoticeArgs) InsertOpts() river.InsertOpts {
	return *jobs.QueuedAs[ApprovalOwnerNoticeArgs](nil)
}

type approvalOwnerNoticeWorker struct {
	pool *pgxpool.Pool
	now  func() time.Time
}

func (w *approvalOwnerNoticeWorker) Work(ctx context.Context, job *river.Job[ApprovalOwnerNoticeArgs]) error {
	wsCtx, err := workspaceJobCtx(ctx, job.Args)
	if err != nil {
		return river.JobCancel(jobs.FaultContext(ctx, err))
	}
	if job.Args.ApprovalID.IsZero() || job.Args.DeciderID.IsZero() {
		return river.JobCancel(jobs.FaultContext(ctx, fmt.Errorf("owner announcement requires a request and recipient: %w", apperrors.ErrInvalidArgument)))
	}
	ctx = principal.SystemActing(wsCtx, "system:approval-owner-notice")
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	db, err := workspaceJobDB(w.pool, job.Args)
	if err != nil {
		return jobs.FaultContext(ctx, err)
	}
	err = db.Tx(ctx, func(tx pgx.Tx) error {
		return tellNewDecider(ctx, tx, notices.NewStore(db), job.Args.DeciderID, []ids.ApprovalID{ids.From[ids.ApprovalKind](job.Args.ApprovalID)}, w.now)
	})
	return jobs.FaultContext(ctx, err)
}
