// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package weeklyplan

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// PreviewCommitmentCarryTx reads commitments through each plan's normal visibility gate.
func (s *Store) PreviewCommitmentCarryTx(ctx context.Context, tx pgx.Tx, kind string, source ids.UUID) ([]crmcontracts.WeeklyPlanCommitment, error) {
	rows, err := tx.Query(ctx, `SELECT DISTINCT p.owner_id, p.local_week_start FROM weekly_plan p
 JOIN weekly_plan_commitment c ON c.plan_id=p.id
 WHERE c.linked_record_type=@kind AND c.linked_record_id=@source
 ORDER BY p.owner_id, p.local_week_start LIMIT 501`, pgx.StrictNamedArgs{"kind": kind, "source": source})
	if err != nil {
		return nil, err
	}
	type selectedPlan struct {
		Owner ids.UUID
		Week  time.Time
	}
	plans, err := pgx.CollectRows(rows, pgx.RowToStructByPos[selectedPlan])
	if err != nil {
		return nil, err
	}
	if len(plans) > 500 {
		return nil, apperrors.ErrInvalidArgument
	}
	out := make([]crmcontracts.WeeklyPlanCommitment, 0)
	for _, selected := range plans {
		if err := s.requirePlanReader(ctx, selected.Owner); err != nil {
			return nil, err
		}
		plan, err := readPlan(ctx, tx, selected.Owner, selected.Week)
		if err != nil {
			return nil, err
		}
		for _, commitment := range plan.Commitments {
			if commitment.LinkedRecordType == kind && commitment.LinkedRecordID == source {
				out = append(out, commitmentToWire(commitment))
			}
		}
		if len(out) > 500 {
			return nil, apperrors.ErrInvalidArgument
		}
	}
	return out, nil
}
