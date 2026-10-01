// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type executionAudit struct {
	RetryBase  int64      `json:"retry_base_attempt"`
	Status     string     `json:"status"`
	Attempt    int64      `json:"attempt"`
	Fence      int64      `json:"fence"`
	EditionID  *ids.UUID  `json:"edition_id"`
	Reason     *string    `json:"reason"`
	LeaseUntil *time.Time `json:"lease_until"`
	RetryAt    *time.Time `json:"retry_at"`
}

func executionState(ctx context.Context, tx pgx.Tx, id ids.UUID) (executionAudit, error) {
	var out executionAudit
	var b bindings
	err := tx.QueryRow(ctx, "SELECT status,attempt,fence,edition_id,reason,lease_until,retry_at,retry_base_attempt FROM report_execution WHERE id="+b.add(id)+" FOR UPDATE", b.values...).Scan(&out.Status, &out.Attempt, &out.Fence, &out.EditionID, &out.Reason, &out.LeaseUntil, &out.RetryAt, &out.RetryBase)
	return out, storedError(err)
}

func recordExecutionChange(ctx context.Context, tx pgx.Tx, id ids.UUID, before executionAudit) error {
	after, err := executionState(ctx, tx, id)
	if err != nil {
		return err
	}
	return recordChange(ctx, tx, "report_edition", id, "update", &before, after)
}
