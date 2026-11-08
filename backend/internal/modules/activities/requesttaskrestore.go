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
func (s *Store) restoreRequestTask(ctx context.Context, tx pgx.Tx, task crmcontracts.Activity, pending bool, userID ids.UUID, in LogActivityInput) (crmcontracts.Activity, bool, error) {
	id := ids.UUID(task.Id)
	// Acceptance locks source then task. Ordinary archived reminders are
	// restorable; lockActivityForWrite deliberately admits only live or held rows.
	lock, err := storekit.LockRow(ctx, tx, "activity", id, storekit.IncludeArchived)
	if err != nil {
		return crmcontracts.Activity{}, false, err
	}
	current, err := readActivityContentAt(ctx, tx, ids.From[ids.ActivityKind](id), storekit.IncludeArchived, s.now())
	if err != nil {
		return crmcontracts.Activity{}, false, err
	}
	if current.AssigneeId == nil || ids.UUID(*current.AssigneeId) != userID {
		return crmcontracts.Activity{}, false, &requestHeldError{}
	}
	if current.IsDone != nil && *current.IsDone {
		return current, false, nil
	}
	var adopted bool
	if err := tx.QueryRow(ctx, "SELECT "+storekit.HumanWroteActivityClause("a")+" FROM activity a WHERE a.id=$1", id).Scan(&adopted); err != nil {
		return crmcontracts.Activity{}, false, err
	}
	if !pending && !adopted {
		return crmcontracts.Activity{}, false, &requestHeldError{}
	}
	if current.ArchivedAt == nil && adopted {
		return current, false, nil
	}
	if err := s.admitRequestRestoration(ctx, tx, id, in); err != nil {
		return crmcontracts.Activity{}, false, err
	}
	if err := recordRequestAcceptance(ctx, tx, lock, current, id); err != nil {
		return crmcontracts.Activity{}, false, err
	}
	out, err := readActivityContentAt(ctx, tx, ids.From[ids.ActivityKind](id), storekit.LiveOnly, s.now())
	return out, false, err
}

func (s *Store) admitRequestRestoration(ctx context.Context, tx pgx.Tx, id ids.UUID, in LogActivityInput) error {
	if err := auth.Require(ctx, "activity", principal.ActionUpdate); err != nil {
		return err
	}
	if err := auth.EnsureActivityWritableIn(ctx, tx, id, false); err != nil {
		return err
	}
	return s.checkTaskRecommendation(ctx, tx, &in)
}

func recordRequestAcceptance(ctx context.Context, tx pgx.Tx, lock storekit.RowLock, current crmcontracts.Activity, id ids.UUID) error {
	var err error
	evidence := map[string]any{"accepted_request": true}
	var audit ids.UUID
	if current.ArchivedAt != nil {
		patch := storekit.NewPatch()
		patch.Set("archived_at", current.ArchivedAt, nil)
		if err := patch.ApplyLocked(ctx, tx, lock); err != nil {
			return err
		}
		audit, err = storekit.AuditWithEvidence(ctx, tx, "update", "activity", id, patch.Before(), patch.After(), evidence)
	} else {
		audit, err = storekit.AuditEventWithEvidence(ctx, tx, "update", "activity", id, nil, evidence)
	}
	if err != nil {
		return err
	}
	fields := crmcontracts.PublicEventActivityChangedFields{}
	if current.ArchivedAt != nil {
		fields.Restored = new(true)
	}
	return storekit.EmitEvent(ctx, tx, audit, id, crmcontracts.PublicEventActivityUpdated{ChangedFields: fields})
}
