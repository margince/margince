// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Marking Worklist tasks and promises done in one change. A worklist_item is
// named by the id its Worklist row carries: a task's activity id or a promise's
// claim id. Each row is ticked by its owning module's own single-record write
// — the task through the activity update a PATCH of is_done makes, the promise
// through the settlement POST /claims/{id}/settle makes — so each gets its own
// audit row and event, and the undo opens each again through the same write.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// worklistBulkTarget is the Worklist's share of a bulk change.
type worklistBulkTarget struct {
	tasks  *activities.Store
	claims *contacts.Store
}

// worklistItem is one task or promise as the change found it under its lock.
type worklistItem struct {
	row     bulkRow
	promise bool
	done    bool
}

// lockItem locks the task the id names, or failing that the promise. The ids
// are UUIDs from two tables, so at most one of the two finds a row.
func (t worklistBulkTarget) lockItem(ctx context.Context, tx pgx.Tx, id ids.UUID) (worklistItem, error) {
	task, err := t.tasks.LockTaskForBulkTx(ctx, tx, ids.From[ids.ActivityKind](id))
	if err == nil {
		return worklistItem{row: bulkRow{label: task.Subject, version: task.Version}, done: task.Done}, nil
	}
	if !errors.Is(err, apperrors.ErrNotFound) {
		return worklistItem{}, err
	}
	claim, err := t.claims.LockClaimForBulkTx(ctx, tx, id)
	if err != nil {
		return worklistItem{}, err
	}
	return worklistItem{row: bulkRow{label: claim.Body, version: claim.Version}, promise: true, done: claim.Done}, nil
}

func (t worklistBulkTarget) lock(ctx context.Context, tx pgx.Tx, id ids.UUID) (bulkRow, error) {
	item, err := t.lockItem(ctx, tx, id)
	return item.row, err
}

// setDone ticks one locked item done or open, and answers the version the
// write left it at.
func (t worklistBulkTarget) setDone(ctx context.Context, tx pgx.Tx, id ids.UUID, item worklistItem, done bool) (int64, error) {
	if item.promise {
		return t.claims.SetClaimDoneTx(ctx, tx, id, done)
	}
	return t.tasks.SetTaskDoneTx(ctx, tx, ids.From[ids.ActivityKind](id), done, item.row.version)
}

// applyCompletion marks one item done, or for an undo open again. An item
// already in the state asked for is left alone as no_change; one whose version
// moved since the caller saw it, or since the change being undone, is skipped.
func applyCompletion(
	ctx context.Context, tx pgx.Tx, target bulkTarget, change bulkChange, item crmcontracts.BulkItem,
) (bulkApplied, crmcontracts.BulkSkip, error) {
	worklist, ok := target.(worklistBulkTarget)
	if !ok {
		return bulkApplied{}, crmcontracts.BulkSkip{}, fmt.Errorf("compose: a %T has nothing to complete", target)
	}
	forward := change.undo == nil
	classify, moved := bulkSkipFor, crmcontracts.BulkSkipReasonChangedSincePreview
	if !forward {
		classify, moved = undoSkipFor, crmcontracts.BulkSkipReasonChangedSinceBatch
	}
	id := ids.UUID(item.Id)
	found, err := worklist.lockItem(ctx, tx, id)
	if err != nil {
		skip, classifyErr := classify(err)
		return bulkApplied{}, skip, classifyErr
	}
	if found.row.version != item.Version {
		return bulkApplied{}, skipped(moved), nil
	}
	if found.done == forward {
		return bulkApplied{}, skipped(crmcontracts.BulkSkipReasonNoChange), nil
	}
	version, err := worklist.setDone(ctx, tx, id, found, forward)
	if err != nil {
		skip, classifyErr := classify(err)
		return bulkApplied{}, skip, classifyErr
	}
	before, after := !forward, forward
	sample := crmcontracts.BulkSampleRow{
		Id: item.Id, Label: found.row.label,
		Before: crmcontracts.BulkRecordState{Done: &before},
		After:  crmcontracts.BulkRecordState{Done: &after},
	}
	return bulkApplied{sample: sample, outcome: bulkOutcome{ID: item.Id, Version: version}}, crmcontracts.BulkSkip{}, nil
}

// visibleRecords answers which of the named records of recordType the reader
// may see now.
func visibleRecords(
	ctx context.Context, tx pgx.Tx, recordType crmcontracts.BulkRecordType, named []ids.UUID,
) (map[ids.UUID]bool, error) {
	if recordType != crmcontracts.BulkRecordTypeWorklistItem {
		return auth.VisibleSubset(ctx, tx, string(recordType), named)
	}
	seen, err := contacts.VisibleClaims(ctx, tx, named)
	if err != nil {
		return nil, err
	}
	tasksRead, err := readGranted(ctx, "activity")
	if err != nil || !tasksRead {
		return seen, err
	}
	for _, id := range named {
		if seen[id] {
			continue
		}
		err := auth.EnsureActivityContentVisible(ctx, tx, id)
		switch {
		case err == nil:
			seen[id] = true
		case !errors.Is(err, apperrors.ErrNotFound):
			return nil, err
		}
	}
	return seen, nil
}
