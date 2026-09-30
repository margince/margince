// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Sweep claims due work and publishes under the schedule owner’s current authority.
func (s *Service) Sweep(ctx context.Context) error {
	if err := auth.RequireSystem(ctx); err != nil {
		return err
	}
	ctx, cancelSweep := context.WithTimeout(ctx, 5*time.Minute)
	defer cancelSweep()
	if err := s.materializeDue(ctx); err != nil {
		return err
	}
	for i := 0; i < 20; i++ {
		if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) < time.Minute {
			return nil
		}
		claimed, found, err := s.claimExecution(ctx)
		if err != nil {
			return err
		}
		if !found {
			return nil
		}
		runCtx, cancel := context.WithTimeout(ctx, time.Minute)
		err = s.executeClaim(runCtx, claimed)
		cancel()
		if err != nil {
			slog.WarnContext(ctx, "report edition generation failed", "execution_id", claimed.ID, "error", err)
			if failure := s.failClaim(ctx, claimed, err); failure != nil {
				return failure
			}
		}
	}
	return nil
}

func (s *Service) materializeDue(ctx context.Context) error {
	return s.store.db.Tx(ctx, func(tx pgx.Tx) error {
		var b bindings
		rows, err := tx.Query(ctx, "SELECT "+scheduleColumns+" FROM report_schedule WHERE enabled AND next_due_at<="+b.add(s.now())+" ORDER BY next_due_at,id LIMIT "+b.add(100)+" FOR UPDATE SKIP LOCKED", b.values...)
		if err != nil {
			return err
		}
		schedules, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (crmcontracts.ReportingSchedule, error) { return scanSchedule(row) })
		if err != nil {
			return err
		}
		remaining := 100
		for _, schedule := range schedules {
			if remaining < 13 {
				break
			}
			count, err := s.materializeSchedule(ctx, tx, schedule)
			if err != nil {
				return err
			}
			remaining -= count
		}
		return nil
	})
}

func (s *Service) suspendSchedule(ctx context.Context, tx pgx.Tx, schedule crmcontracts.ReportingSchedule, reason string) error {
	before := schedule
	schedule.Definition.Enabled = false
	schedule.Version++
	schedule.LastStatus = &reason
	if err := s.writeSchedule(ctx, tx, schedule); err != nil {
		return err
	}
	return recordChange(ctx, tx, "report_schedule", ids.UUID(schedule.Id), "pause", &before, schedule)
}

type executionClaim struct {
	ID, Owner                 ids.UUID
	Fence, Attempt, RetryBase int64
}

func (s *Service) claimExecution(ctx context.Context) (executionClaim, bool, error) {
	var out executionClaim
	found := false
	err := s.store.db.Tx(ctx, func(tx pgx.Tx) error {
		var b bindings
		now := s.now()
		query := "SELECT id FROM report_execution WHERE ((status='pending' AND (retry_at IS NULL OR retry_at<=" + b.add(now) + ")) OR (status='running' AND lease_until<" + b.add(now) + ")) ORDER BY COALESCE((SELECT previous.id FROM report_execution previous WHERE previous.report_id=report_execution.report_id AND previous.status IN ('succeeded','partial','failed','suspended','skipped') ORDER BY previous.id DESC LIMIT 1), '00000000-0000-0000-0000-000000000000'::uuid),intended_due_at,id LIMIT 1 FOR UPDATE SKIP LOCKED"
		err := tx.QueryRow(ctx, query, b.values...).Scan(&out.ID)
		if err == pgx.ErrNoRows {
			return nil
		}
		if err != nil {
			return err
		}
		before, err := executionState(ctx, tx, out.ID)
		if err != nil {
			return err
		}
		if before.Status == "running" && before.Attempt-before.RetryBase >= 3 {
			return s.exhaustClaim(ctx, tx, out, before)
		}
		b = bindings{}
		err = tx.QueryRow(ctx, "UPDATE report_execution SET status='running',fence=fence+1,attempt=attempt+1,lease_until="+b.add(now.Add(90*time.Second))+" WHERE id="+b.add(out.ID)+" RETURNING owner_id,fence,attempt,retry_base_attempt", b.values...).Scan(&out.Owner, &out.Fence, &out.Attempt, &out.RetryBase)
		if err != nil {
			return err
		}
		found = true
		return recordExecutionChange(ctx, tx, out.ID, before)
	})
	return out, found, err
}

func (s *Service) exhaustClaim(ctx context.Context, tx pgx.Tx, claim executionClaim, before executionAudit) error {
	var b bindings
	reason := "Report generation stopped repeatedly before completing. Retry the run."
	if _, err := tx.Exec(ctx, "UPDATE report_execution SET status='failed',fence=fence+1,lease_until=NULL,reason="+b.add(reason)+" WHERE id="+b.add(claim.ID), b.values...); err != nil {
		return err
	}
	if err := recordExecutionChange(ctx, tx, claim.ID, before); err != nil {
		return err
	}
	return s.recordScheduleResult(ctx, tx, claim, "failed", reason)
}
