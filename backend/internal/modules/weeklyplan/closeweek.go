// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package weeklyplan

// Closing a week: what the plan came to, settled once and then frozen.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Outcome is what a closed week's plan came to.
//
// RETURNED rather than written into weekly_review, because this module does not
// own that table — compose/weekly does, and a second writer of one table is how
// two answers to one question appear. The weekly job takes these and freezes
// them alongside its own counts.
type Outcome struct {
	// Due counts every commitment the rep stood by: dropped ones are not owed,
	// because dropping is a decision rather than a failure to act.
	Due int
	// Kept counts the ones done. Never greater than Due, which the review's own
	// CHECK also holds.
	Kept int
}

// CloseWeek settles the caller's plan for the week containing now and reports
// what it came to.
//
// IDEMPOTENT, and that is the whole design. The weekly dispatcher ticks more
// than once inside a week so a worker that was down still backfills, so this
// runs repeatedly over the same plan. A second call finds the plan already
// closed and answers the SAME counts without moving a row — if it re-settled,
// a commitment the rep completed after the first close would flip from missed
// to done and the frozen review would stop matching the plan beside it.
func (s *Store) CloseWeek(ctx context.Context, now time.Time) (Outcome, error) {
	if err := auth.Require(ctx, "weekly_plan", principal.ActionUpdate); err != nil {
		return Outcome{}, err
	}
	owner, err := planUser(ctx)
	if err != nil {
		return Outcome{}, err
	}
	var out Outcome
	err = database.WithWorkspaceTx(ctx, s.db.Pool(), func(tx pgx.Tx) error {
		week, err := s.weekStart(ctx, tx, now)
		if err != nil {
			return err
		}
		// The week that CLOSED, not the one running: the same window the
		// review covers, so a plan and the retrospective beside it are about
		// the same seven days.
		closing := week.AddDate(0, 0, -7)
		plan, err := readPlan(ctx, tx, owner, closing)
		if errors.Is(err, apperrors.ErrNotFound) {
			// A rep who planned nothing owes nothing. Zero, and no plan row
			// invented to say so.
			return nil
		}
		if err != nil {
			return err
		}
		if plan.Status != "open" {
			// The STAMPED answer, not a fresh count. A commitment that moved
			// after the first close — a late-landing write, a repair — must not
			// change what the review already froze.
			if plan.Outcome != nil {
				out = *plan.Outcome
			}
			return nil
		}
		// Everything still open when the week ran out was not done. `missed` is
		// written here and nowhere else: it is the week's verdict, not
		// something a person declares about themselves.
		// The plan row is locked FIRST, and every write below runs under it —
		// including the sweep, which is by plan_id rather than by commitment.
		// Without the lock a rep marking a commitment done in the same instant
		// races the sweep marking it missed, and which one lands is a coin
		// toss the review then freezes.
		lock, err := storekit.LockRow(ctx, tx, "weekly_plan", plan.ID,
			storekit.NoArchiveColumn)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE weekly_plan_commitment
			   SET state = $2, version = version + 1, updated_at = now()
			 WHERE plan_id = $1 AND state = $3`,
			plan.ID, StateMissed, StateOpen); err != nil {
			return fmt.Errorf("weeklyplan: settling the week: %w", err)
		}
		// Counted from the SETTLED rows, then stamped. Reading before the sweep
		// would count the commitments the sweep is about to mark missed as
		// still open, which is the same number by luck and the wrong reason.
		settled, err := readCommitments(ctx, tx, plan.ID)
		if err != nil {
			return err
		}
		out = countOutcome(settled)
		patch := storekit.NewPatch()
		patch.Set("status", plan.Status, "closed")
		patch.Set("commitments_due", nil, out.Due)
		patch.Set("commitments_kept", nil, out.Kept)
		// The same lock taken above: one FOR UPDATE covers the sweep and the
		// close, so nothing can settle a commitment between them.
		if err := patch.ApplyLocked(ctx, tx, lock); err != nil {
			return err
		}
		auditID, err := storekit.Audit(ctx, tx, "update", "weekly_plan", plan.ID,
			patch.Before(), patch.After())
		if err != nil {
			return err
		}
		return storekit.EmitEvent(ctx, tx, auditID, owner, crmcontracts.PublicEventWeeklyPlanUpdated{
			PlanId: openapi_types.UUID(plan.ID), OwnerUserId: openapi_types.UUID(owner),
			ChangedFields: []string{"status"},
		})
	})
	if err != nil {
		return Outcome{}, err
	}
	return out, nil
}

// countOutcome tallies a settled week.
//
// Dropped commitments count in neither: a rep who decided on Wednesday that a
// thing was not worth doing did not fail to do it, and counting it against them
// teaches them to leave dead commitments open instead of saying so.
func countOutcome(commitments []Commitment) Outcome {
	var out Outcome
	for _, c := range commitments {
		if c.State == StateDropped {
			continue
		}
		out.Due++
		if c.State == StateDone {
			out.Kept++
		}
	}
	return out
}
