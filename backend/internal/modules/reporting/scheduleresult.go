// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func (s *Service) recordScheduleResult(ctx context.Context, tx pgx.Tx, claim executionClaim, status, reason string) error {
	var b bindings
	run := b.add(claim.ID)
	before, err := scanSchedule(tx.QueryRow(ctx, "SELECT "+scheduleColumns+" FROM report_schedule WHERE id=(SELECT schedule_id FROM report_execution WHERE id="+run+") AND version=(SELECT schedule_version FROM report_execution WHERE id="+run+") FOR UPDATE", b.values...))
	if errors.Is(err, apperrors.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	after := before
	result := status
	if reason != "" {
		result += " · " + reason
	}
	after.LastStatus = &result
	if status == reportingSuspended {
		after.Definition.Enabled = false
		after.Version++
	}
	if err := s.writeSchedule(ctx, tx, after); err != nil {
		return err
	}
	return recordChange(ctx, tx, "report_schedule", ids.UUID(after.Id), "update", &before, after)
}
