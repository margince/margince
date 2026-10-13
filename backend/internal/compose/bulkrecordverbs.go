// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The verbs that write beside a record rather than on it: a tag put on or
// taken off, and one task filed under each record. Each row goes through the
// single-record writer — ApplyTagTx, RemoveTagTx, the createTask mapping and
// LogActivityTx — so a bulk tag or task is attributed, gated and audited
// exactly as one applied from the record page.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// bulkWriters are the stores one admitted change writes through beside the
// record's own module: only the one its verb needs is set.
type bulkWriters struct {
	lists *collections.Store
	tags  *collections.Store
	tasks *activities.Store
}

// admitVerb refuses a verb the caller may not use at all, sets the store its
// rows are written through, and answers what the verb asks of each record.
func (e *bulkEngine) admitVerb(ctx context.Context, change *bulkChange) (principal.Action, error) {
	switch {
	case change.verb == crmcontracts.BulkVerbArchive:
		return principal.ActionDelete, nil
	case isListVerb(change.verb):
		// A membership changes the list, not the record: reading the record
		// is what naming it on a list takes.
		change.writers.lists = e.lists
		return principal.ActionRead, e.admitListVerb(ctx, *change)
	case isTagVerb(change.verb):
		change.writers.tags = e.tags
		return principal.ActionUpdate, e.admitTagVerb(ctx, *change)
	case change.verb == crmcontracts.BulkVerbCreateTask:
		// A task is filed under the record, not written on it: reading the
		// record is what linking a task to it takes, as createTask asks.
		change.writers.tasks = e.tasks
		return principal.ActionRead, admitTaskVerb(ctx, *change)
	}
	return principal.ActionUpdate, nil
}

// isTagVerb says the verb puts a tag on the record or takes it off.
func isTagVerb(verb crmcontracts.BulkVerb) bool {
	return verb == crmcontracts.BulkVerbAddTag || verb == crmcontracts.BulkVerbRemoveTag
}

// admitTagVerb refuses a tag verb before any row is tried: a tag that does not
// exist, or a retired one to put on. An undo of remove_tag puts it back on.
func (e *bulkEngine) admitTagVerb(ctx context.Context, change bulkChange) error {
	applying := (change.verb == crmcontracts.BulkVerbAddTag) == (change.undo == nil)
	return e.tags.CheckTagChange(ctx, ids.From[ids.TagKind](*change.tagID), applying)
}

// admitTaskVerb asks for the activity grant the task writer asks of each row
// — create to file a task, delete to retire it on undo — once, up front.
func admitTaskVerb(ctx context.Context, change bulkChange) error {
	if change.undo != nil {
		return auth.Require(ctx, "activity", principal.ActionDelete)
	}
	return auth.Require(ctx, "activity", principal.ActionCreate)
}

// taskAssignee is the colleague create_task names as the owner of its tasks.
func taskAssignee(change bulkChange) *ids.UUID {
	if change.task == nil || change.task.AssigneeId == nil {
		return nil
	}
	id := ids.UUID(*change.task.AssigneeId)
	return &id
}

// archiverOf answers the record type's archive. admit refuses the archive verb
// for a type without one, so a miss here is a bug rather than a row's fault.
//
//nolint:ireturn // the archive share of a target is exactly the thing asked for
func archiverOf(target bulkTarget) (bulkArchiver, error) {
	archiver, ok := target.(bulkArchiver)
	if !ok {
		return nil, fmt.Errorf("compose: a %T has no archive", target)
	}
	return archiver, nil
}

// reassignerOf answers the record type's reassignment. admit refuses
// reassign_owner for a type without one, so a miss here is a bug.
//
//nolint:ireturn // the reassignment share of a target is exactly the thing asked for
func reassignerOf(target bulkTarget) (bulkReassigner, error) {
	reassigner, ok := target.(bulkReassigner)
	if !ok {
		return nil, fmt.Errorf("compose: a %T has no owner to hand on", target)
	}
	return reassigner, nil
}

// lockUnmoved locks one record for a forward change, holding it to the version
// the caller was shown; an undo holds it to nothing, because a tag, a
// membership or a task never moved the record's version.
func lockUnmoved(
	ctx context.Context, tx pgx.Tx, target bulkTarget, change bulkChange, item crmcontracts.BulkItem,
) (bulkRow, crmcontracts.BulkSkip, error) {
	row, err := target.lock(ctx, tx, ids.UUID(item.Id))
	if err != nil {
		skip, classifyErr := bulkSkipFor(err)
		return bulkRow{}, skip, classifyErr
	}
	if change.undo == nil && row.version != item.Version {
		return bulkRow{}, skipped(crmcontracts.BulkSkipReasonChangedSincePreview), nil
	}
	return row, crmcontracts.BulkSkip{}, nil
}

// applyTagging puts the change's tag on one record or takes it off.
func applyTagging(
	ctx context.Context, tx pgx.Tx, target bulkTarget, change bulkChange, item crmcontracts.BulkItem, add bool,
) (bulkApplied, crmcontracts.BulkSkip, error) {
	row, skip, err := lockUnmoved(ctx, tx, target, change, item)
	if err != nil || skip.Reason != "" {
		return bulkApplied{}, skip, err
	}
	outcome, skip, err := tagOne(ctx, tx, change, item, add)
	if err != nil || skip.Reason != "" {
		return bulkApplied{}, skip, err
	}
	before, after := !add, add
	sample := crmcontracts.BulkSampleRow{
		Id: item.Id, Label: row.label,
		Before: crmcontracts.BulkRecordState{OwnerId: wireOwner(row.ownerID), Tagged: &before},
		After:  crmcontracts.BulkRecordState{OwnerId: wireOwner(row.ownerID), Tagged: &after},
	}
	outcome.ID, outcome.Version = item.Id, row.version
	return bulkApplied{sample: sample, outcome: outcome}, crmcontracts.BulkSkip{}, nil
}

// tagOne is one row's tag write, answering what its undo needs. Putting the
// tag on answers the assignment it made, which is all its undo may take off,
// so a tag somebody took off and put back since is skipped as
// changed_since_batch. Taking it off answers the removal its undo restores.
func tagOne(
	ctx context.Context, tx pgx.Tx, change bulkChange, item crmcontracts.BulkItem, add bool,
) (bulkOutcome, crmcontracts.BulkSkip, error) {
	tag, entity, id := ids.From[ids.TagKind](*change.tagID), string(change.recordType), ids.UUID(item.Id)
	tags := change.writers.tags
	var out bulkOutcome
	var err error
	unmoved := crmcontracts.BulkSkipReasonNoChange
	switch {
	case add && change.undo != nil:
		var removal ids.UUID
		if removal, err = change.undo.removalOf(item.Id); err == nil {
			_, err = tags.RestoreBatchTagRemovalTx(ctx, tx, tag, removal, change.undo.batchID)
		}
	case add:
		row, applyErr := tags.ApplyTagTx(ctx, tx, tag, entity, id)
		err = applyErr
		assignment := openapi_types.UUID(row.ID)
		out.TaggableID = &assignment
	case change.undo != nil && change.verb == crmcontracts.BulkVerbAddTag:
		made, known := change.undo.taggings[item.Id]
		if !known {
			return bulkOutcome{}, skipped(crmcontracts.BulkSkipReasonNoChange), nil
		}
		unmoved = crmcontracts.BulkSkipReasonChangedSinceBatch
		var removed bool
		removed, err = tags.RemoveTagAssignmentTx(ctx, tx, ids.UUID(made), tag, entity, id)
		if err == nil && !removed {
			return bulkOutcome{}, skipped(unmoved), nil
		}
	default:
		var removal ids.UUID
		removal, err = tags.RemoveTagTx(ctx, tx, tag, entity, id)
		if err == nil && removal == ids.Nil {
			return bulkOutcome{}, skipped(unmoved), nil
		}
		kept := openapi_types.UUID(removal)
		out.RemovalID = &kept
	}
	switch {
	case errors.Is(err, collections.ErrRemovalMovedOn):
		return bulkOutcome{}, skipped(crmcontracts.BulkSkipReasonChangedSinceBatch), nil
	case add && change.undo == nil && errors.Is(err, apperrors.ErrConflict):
		return bulkOutcome{}, skipped(crmcontracts.BulkSkipReasonNoChange), nil
	case err != nil:
		skip, classifyErr := bulkSkipFor(err)
		return bulkOutcome{}, skip, classifyErr
	}
	return out, crmcontracts.BulkSkip{}, nil
}

// applyTask files the change's task under one record, through the mapping the
// createTask door uses.
func applyTask(
	ctx context.Context, tx pgx.Tx, target bulkTarget, change bulkChange, item crmcontracts.BulkItem,
) (bulkApplied, crmcontracts.BulkSkip, error) {
	row, skip, err := lockUnmoved(ctx, tx, target, change, item)
	if err != nil || skip.Reason != "" {
		return bulkApplied{}, skip, err
	}
	in, err := activities.TaskInputFrom(taskRequestFor(change, item.Id))
	if err != nil {
		return bulkApplied{}, crmcontracts.BulkSkip{}, err
	}
	task, _, err := change.writers.tasks.LogActivityTx(ctx, tx, in)
	if err != nil {
		skip, classifyErr := bulkSkipFor(err)
		return bulkApplied{}, skip, classifyErr
	}
	taskID := task.Id
	var taskVersion int64
	if task.Version != nil {
		taskVersion = *task.Version
	}
	sample := crmcontracts.BulkSampleRow{
		Id: item.Id, Label: row.label,
		Before: crmcontracts.BulkRecordState{OwnerId: wireOwner(row.ownerID)},
		After:  crmcontracts.BulkRecordState{OwnerId: wireOwner(row.ownerID), TaskId: &taskID},
	}
	outcome := bulkOutcome{ID: item.Id, Version: row.version, TaskID: &taskID, TaskVersion: taskVersion}
	return bulkApplied{sample: sample, outcome: outcome}, crmcontracts.BulkSkip{}, nil
}

// taskRequestFor is the createTask request one row of create_task makes: the
// change's task, linked to that record.
func taskRequestFor(change bulkChange, record openapi_types.UUID) crmcontracts.CreateTaskRequest {
	links := []struct {
		EntityId   openapi_types.UUID                            `json:"entity_id"` //nolint:staticcheck // the generated link element spells it so, and this literal must match it
		EntityType crmcontracts.CreateTaskRequestLinksEntityType `json:"entity_type"`
	}{{EntityId: record, EntityType: crmcontracts.CreateTaskRequestLinksEntityType(change.recordType)}}
	return crmcontracts.CreateTaskRequest{
		Subject: change.task.Subject, DueAt: change.task.DueAt, AssigneeId: change.task.AssigneeId,
		Links: &links, Source: "manual",
	}
}

// undoTask retires the task the change filed under one record, held to the
// version the change left it at: a task completed or edited since is someone's
// work now, and is skipped as changed_since_batch rather than archived.
func undoTask(
	ctx context.Context, tx pgx.Tx, target bulkTarget, change bulkChange, item crmcontracts.BulkItem,
) (bulkApplied, crmcontracts.BulkSkip, error) {
	created, ok := change.undo.tasks[item.Id]
	if !ok {
		return bulkApplied{}, skipped(crmcontracts.BulkSkipReasonNoChange), nil
	}
	row, skip, err := lockUnmoved(ctx, tx, target, change, item)
	if err != nil || skip.Reason != "" {
		return bulkApplied{}, skip, err
	}
	version := created.version
	err = change.writers.tasks.ArchiveActivityTx(ctx, tx, ids.From[ids.ActivityKind](ids.UUID(created.id)), &version)
	if err != nil {
		skip, classifyErr := undoSkipFor(err)
		return bulkApplied{}, skip, classifyErr
	}
	taskID := created.id
	sample := crmcontracts.BulkSampleRow{
		Id: item.Id, Label: row.label,
		Before: crmcontracts.BulkRecordState{OwnerId: wireOwner(row.ownerID), TaskId: &taskID},
		After:  crmcontracts.BulkRecordState{OwnerId: wireOwner(row.ownerID)},
	}
	return bulkApplied{sample: sample, outcome: bulkOutcome{ID: item.Id, Version: row.version}}, crmcontracts.BulkSkip{}, nil
}
