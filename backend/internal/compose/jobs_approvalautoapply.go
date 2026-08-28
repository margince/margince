// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The tick that applies what a rep already answered.
//
// A proposal of a reversible kind, on a record whose owner has put that kind on
// automatic, is a question the product was told not to ask. This is the clock
// that stops asking it: the decision itself belongs to the approvals module —
// status, the audit row, the event, the registered effect — and this file
// supplies only the schedule.
//
// It is a sweep rather than a hook at staging time for two reasons. The answer
// can change after the proposal is made, so a rep turning automatic on today
// should see yesterday's suggestions applied without re-staging them. And
// staging happens at nineteen call sites, where a rule spread across all of
// them is a rule one of them will forget.

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ApprovalAutoApplyArgs applies proposals their owner has put on automatic.
type ApprovalAutoApplyArgs struct{}

// Kind is the stable job identifier River persists in river_job.
func (ApprovalAutoApplyArgs) Kind() string { return "approval_auto_apply" }

// InsertOpts carries the attempt cap the declaration publishes, because the
// periodic insert supplies uniqueness and no attempt policy of its own.
//
// One attempt: the retry is the next tick a minute away, and a row this pass
// did not apply is still pending then. A row that did not apply because its
// owner has gone is not a fault to retry at all.
func (ApprovalAutoApplyArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue:       river.QueueDefault,
		MaxAttempts: 1,
		UniqueOpts:  river.UniqueOpts{ByState: activeSweepStates},
	}
}

type approvalAutoApplyWorker struct {
	pool     *pgxpool.Pool
	identity *identity.Service
	log      *slog.Logger
}

// Work applies what is due.
//
// The pass binds no actor of its own, and that is the point. Each apply runs
// under the OWNER's authority — an agent principal carrying their grants, teams
// and seat — resolved inside autoApplier at the moment it applies, so the write
// is bounded by exactly what that rep could do by hand. A pass-wide system
// principal would bypass object RBAC and row scope for every row it touched.
//
// The correlation id groups one tick's applies as the single pass they are,
// which is what lets a reader follow an unexpected batch back to its cause.
func (w *approvalAutoApplyWorker) Work(ctx context.Context, _ *river.Job[ApprovalAutoApplyArgs]) error {
	passCtx, err := installationJobCtx(ctx, w.identity)
	if err != nil {
		return jobs.FaultContext(ctx, err)
	}
	passCtx = principal.WithCorrelationID(passCtx, ids.NewV7())
	applied, err := SweepAutoApply(passCtx, w.pool)
	if err != nil {
		return jobs.FaultContext(ctx, err)
	}
	if applied > 0 {
		// Worth a line: these are writes nobody was asked about, so a run of
		// them is what a reader wants to be able to find after the fact.
		w.logger().InfoContext(ctx, "approval auto-apply: proposals applied under standing policy",
			"count", applied)
	}
	return nil
}

func (w *approvalAutoApplyWorker) logger() *slog.Logger {
	if w.log == nil {
		return slog.Default()
	}
	return w.log
}
