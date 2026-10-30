// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// checkTaskAssignee holds a task filed for a colleague to the rule a person
// assigning one meets. A task already filed under this key is a replay. Its
// assignee was judged once and may have left since, and that is no reason to
// refuse the unit's retry.
func (s *Sink) checkTaskAssignee(ctx context.Context, tx pgx.Tx, rec connector.NormalizedRecord, fields ActivityFields) error {
	if fields.AssigneeID.IsZero() {
		return nil
	}
	var filed bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM activity WHERE source_system = $1 AND source_id = $2)`,
		rec.NaturalKey.SourceSystem, rec.NaturalKey.SourceID).Scan(&filed); err != nil {
		return fmt.Errorf("capture: task replay lookup: %w", err)
	}
	if filed {
		return nil
	}
	if s.taskAssignee == nil {
		return errors.New("capture: this role files no task for a colleague")
	}
	return s.taskAssignee(ctx, tx, ids.From[ids.UserKind](fields.AssigneeID))
}
