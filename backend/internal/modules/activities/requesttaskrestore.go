// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Only explicit acceptance restores an archived reminder. Completion is final
// until somebody edits the task; neither capture nor replay reopens done work.
func restoreRequestTask(ctx context.Context, tx pgx.Tx, task crmcontracts.Activity) (crmcontracts.Activity, bool, error) {
	if task.ArchivedAt == nil || (task.IsDone != nil && *task.IsDone) {
		return task, false, nil
	}
	if err := auth.Require(ctx, "activity", principal.ActionUpdate); err != nil {
		return crmcontracts.Activity{}, false, err
	}
	id := ids.UUID(task.Id)
	// Acceptance locks source then task. Ordinary archived reminders are
	// restorable; lockActivityForWrite deliberately admits only live or held rows.
	lock, err := storekit.LockRow(ctx, tx, "activity", id, storekit.IncludeArchived)
	if err != nil {
		return crmcontracts.Activity{}, false, err
	}
	current, err := readActivityContent(ctx, tx, ids.From[ids.ActivityKind](id), storekit.IncludeArchived)
	if err != nil {
		return crmcontracts.Activity{}, false, err
	}
	if current.IsDone != nil && *current.IsDone {
		return current, false, nil
	}
	if err := auth.EnsureActivityWritableIn(ctx, tx, id, false); err != nil {
		return crmcontracts.Activity{}, false, err
	}
	patch := storekit.NewPatch()
	patch.Set("archived_at", current.ArchivedAt, nil)
	if err := patch.ApplyLocked(ctx, tx, lock); err != nil {
		return crmcontracts.Activity{}, false, err
	}
	audit, err := storekit.Audit(ctx, tx, "update", "activity", id, map[string]any{"archived_at": current.ArchivedAt}, map[string]any{"archived_at": nil})
	if err != nil {
		return crmcontracts.Activity{}, false, err
	}
	restored := true
	if err := storekit.EmitEvent(ctx, tx, audit, id, crmcontracts.PublicEventActivityUpdated{ChangedFields: crmcontracts.PublicEventActivityChangedFields{Restored: &restored}}); err != nil {
		return crmcontracts.Activity{}, false, err
	}
	out, err := readActivityContent(ctx, tx, ids.From[ids.ActivityKind](id), storekit.LiveOnly)
	return out, false, err
}
