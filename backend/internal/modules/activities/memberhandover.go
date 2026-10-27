// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const memberTaskSource = "from"

// ConsolidateMemberTasksTx changes responsibility without disclosing task content.
// It is available only inside an admitted unused-identity consolidation.
func (s *Store) ConsolidateMemberTasksTx(ctx context.Context, tx pgx.Tx, from, to ids.UserID) (int, error) {
	if !auth.ConsolidatingMember(ctx, from.UUID, to.UUID) {
		return 0, apperrors.ErrPermissionDenied
	}
	for _, object := range []string{"user_admin", linkEntityActivity} {
		if err := auth.Require(ctx, object, principal.ActionUpdate); err != nil {
			return 0, err
		}
	}
	if err := auth.Require(ctx, "user_admin", principal.ActionDelete); err != nil {
		return 0, err
	}
	if err := auth.EnsureAssignee(ctx, tx, to.UUID); err != nil {
		return 0, err
	}
	rows, err := tx.Query(ctx, `SELECT id,version FROM activity WHERE assignee_id=@from AND kind='task' AND NOT is_done AND archived_at IS NULL ORDER BY id FOR UPDATE`, pgx.StrictNamedArgs{memberTaskSource: from})
	if err != nil {
		return 0, err
	}
	tasks, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (struct {
		ID      ids.UUID
		Version int64
	}, error,
	) {
		var task struct {
			ID      ids.UUID
			Version int64
		}
		err := row.Scan(&task.ID, &task.Version)
		return task, err
	})
	if err != nil {
		return 0, err
	}
	transferred := 0
	for _, task := range tasks {
		patch := storekit.NewPatch()
		patch.Set("assignee_id", from.UUID, to.UUID)
		if patch.Empty() {
			continue
		}
		if err := patch.ApplyGuarded(ctx, tx, linkEntityActivity, task.ID, &task.Version); err != nil {
			return 0, err
		}
		audit, err := storekit.Audit(ctx, tx, "update", linkEntityActivity, task.ID, patch.Before(), patch.After())
		if err != nil {
			return 0, err
		}
		assignee := openapi_types.UUID(to.UUID)
		if err := storekit.EmitEvent(ctx, tx, audit, task.ID, crmcontracts.PublicEventActivityUpdated{ChangedFields: crmcontracts.PublicEventActivityChangedFields{AssigneeId: &assignee}}); err != nil {
			return 0, err
		}
		transferred++
	}
	return transferred, nil
}
