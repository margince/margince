// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Undoing a bulk change: a compensating bulk change of its own, run through
// the same engine, previewed and confirmed the same way.
//
// The undo's items are the records the change altered, each pinned to the
// version the change left it at. A record that has moved since is skipped as
// changed_since_batch rather than overwritten; a reassignment goes back to the
// owner it had; an archive is un-archived by its owning module, which also
// refuses a record merged, erased or whose email or domain another live record
// holds now. The undo gets its own batch id and bulk_operation row, linked to
// the change it reversed, and a change is undone once.

import (
	"context"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// bulkUndoPlan is what an undo puts back.
type bulkUndoPlan struct {
	// batchID is the change being undone.
	batchID ids.UUID
	// ownersBefore is each record's owner before the change.
	ownersBefore map[openapi_types.UUID]*openapi_types.UUID
	// tasks is the task create_task filed under each record.
	tasks map[openapi_types.UUID]bulkCreatedTask
	// taggings is the tag assignment add_tag made on each record.
	taggings map[openapi_types.UUID]openapi_types.UUID
	// members is each record's list membership as remove_from_list took it.
	members map[openapi_types.UUID]collections.RemovedMember
}

// membershipOf answers the list membership remove_from_list took from a
// record; a forward change has no plan and knows none.
func (p *bulkUndoPlan) membershipOf(id openapi_types.UUID) (collections.RemovedMember, bool) {
	if p == nil {
		return collections.RemovedMember{}, false
	}
	was, known := p.members[id]
	return was, known
}

// bulkCreatedTask is one task create_task filed, at the version it left it.
type bulkCreatedTask struct {
	id      openapi_types.UUID
	version int64
}

var (
	errBulkAlreadyUndone = &httperr.DetailedError{
		Status: http.StatusConflict, Code: "bulk_already_undone",
		Detail: "this bulk change was already undone; a change is undone once",
	}
	errBulkUndoOfUndo = &httperr.DetailedError{
		Status: http.StatusConflict, Code: "bulk_undo_not_undoable",
		Detail: "this is an undo; it is not undone in turn. Make the change again instead",
	}
	errBulkNothingToUndo = &httperr.DetailedError{
		Status: http.StatusConflict, Code: "bulk_nothing_to_undo",
		Detail: "this bulk change changed no record the undo can name, so there is nothing to put back",
	}
)

// PreviewUndo answers what undoing the change batchID would do.
func (e *bulkEngine) PreviewUndo(ctx context.Context, batchID ids.UUID) (crmcontracts.BulkChangePreview, error) {
	change, err := e.undoChange(ctx, batchID)
	if err != nil {
		return crmcontracts.BulkChangePreview{}, err
	}
	return e.Preview(ctx, change)
}

// Undo puts back what the change batchID did, under confirmToken when the
// change altered more than bulkConfirmAbove records.
func (e *bulkEngine) Undo(ctx context.Context, batchID ids.UUID, confirmToken string) (crmcontracts.BulkChangeResult, error) {
	change, err := e.undoChange(ctx, batchID)
	if err != nil {
		return crmcontracts.BulkChangeResult{}, err
	}
	change.confirmToken = confirmToken
	return e.Execute(ctx, change)
}

// undoChange reads the change batchID and answers the change that reverses it.
// The once-only rule is asked here for a fast answer and again when the undo
// records itself, under the unique index, where two racing undos meet.
func (e *bulkEngine) undoChange(ctx context.Context, batchID ids.UUID) (bulkChange, error) {
	var op bulkOperation
	err := e.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		op, err = readBulkOperation(ctx, tx, batchID)
		return err
	})
	switch {
	case err != nil:
		return bulkChange{}, err
	case op.undoOf != nil:
		return bulkChange{}, errBulkUndoOfUndo
	case op.undoneBy != nil:
		return bulkChange{}, errBulkAlreadyUndone
	case len(op.result.Changed) == 0:
		return bulkChange{}, errBulkNothingToUndo
	}
	plan := &bulkUndoPlan{
		batchID:      batchID,
		ownersBefore: make(map[openapi_types.UUID]*openapi_types.UUID, len(op.result.Changed)),
		tasks:        map[openapi_types.UUID]bulkCreatedTask{},
		taggings:     map[openapi_types.UUID]openapi_types.UUID{},
		members:      map[openapi_types.UUID]collections.RemovedMember{},
	}
	items := make([]crmcontracts.BulkItem, len(op.result.Changed))
	for i, outcome := range op.result.Changed {
		items[i] = crmcontracts.BulkItem{Id: outcome.ID, Version: outcome.Version}
		plan.ownersBefore[outcome.ID] = outcome.OwnerBefore
		if outcome.TaskID != nil {
			plan.tasks[outcome.ID] = bulkCreatedTask{id: *outcome.TaskID, version: outcome.TaskVersion}
		}
		if outcome.MemberAddedAt != nil {
			plan.members[outcome.ID] = collections.RemovedMember{Note: outcome.MemberNote, AddedAt: *outcome.MemberAddedAt}
		}
		if outcome.TaggableID != nil {
			plan.taggings[outcome.ID] = *outcome.TaggableID
		}
	}
	return bulkChange{
		recordType: op.recordType, verb: op.verb, items: items, listID: op.listID, tagID: op.tagID, undo: plan,
	}, nil
}

// undoOne reverses the change on one record.
func undoOne(
	ctx context.Context, tx pgx.Tx, target bulkTarget, change bulkChange, item crmcontracts.BulkItem,
) (bulkApplied, crmcontracts.BulkSkip, error) {
	var sample crmcontracts.BulkSampleRow
	var leftBehind []storekit.LeftBehind
	var restored storekit.RestoreReport
	var err error
	switch {
	case isListVerb(change.verb):
		// The inverse membership change, through the same writer.
		return applyMembership(ctx, tx, target, change, item, change.verb == crmcontracts.BulkVerbRemoveFromList)
	case isTagVerb(change.verb):
		return applyTagging(ctx, tx, target, change, item, change.verb == crmcontracts.BulkVerbRemoveTag)
	case change.verb == crmcontracts.BulkVerbCreateTask:
		return undoTask(ctx, tx, target, change, item)
	}
	switch change.verb {
	case crmcontracts.BulkVerbReassignOwner:
		var skip crmcontracts.BulkSkip
		sample, skip, err = handBack(ctx, tx, target, change.undo, item)
		if err == nil && skip.Reason != "" {
			return bulkApplied{}, skip, nil
		}
	case crmcontracts.BulkVerbArchive:
		var report storekit.RestoreReport
		var archiver bulkArchiver
		if archiver, err = archiverOf(target); err == nil {
			report, err = archiver.restore(ctx, tx, ids.UUID(item.Id), item.Version, change.pendingLinks)
		}
		sample = crmcontracts.BulkSampleRow{
			Id: item.Id, Label: report.Label,
			Before: crmcontracts.BulkRecordState{Archived: true},
		}
		leftBehind = report.LeftBehind
		restored = report
	}
	if err != nil {
		skip, classifyErr := undoSkipFor(err)
		return bulkApplied{}, skip, classifyErr
	}
	applied, skip, err := appliedAt(ctx, tx, change.recordType, sample)
	applied.restored = restored
	for _, left := range leftBehind {
		applied.leftBehind = append(applied.leftBehind, crmcontracts.BulkLeftBehind{
			Id: item.Id, Kind: crmcontracts.BulkLeftBehindKind(left.Kind), RefId: openapi_types.UUID(left.ID),
		})
	}
	return applied, skip, err
}

// handBack gives one record back to the owner it had before the change.
func handBack(
	ctx context.Context, tx pgx.Tx, target bulkTarget, plan *bulkUndoPlan, item crmcontracts.BulkItem,
) (crmcontracts.BulkSampleRow, crmcontracts.BulkSkip, error) {
	id := ids.UUID(item.Id)
	row, err := target.lock(ctx, tx, id)
	if err != nil {
		return crmcontracts.BulkSampleRow{}, crmcontracts.BulkSkip{}, err
	}
	if row.version != item.Version {
		return crmcontracts.BulkSampleRow{}, skipped(crmcontracts.BulkSkipReasonChangedSinceBatch), nil
	}
	before := plan.ownersBefore[item.Id]
	if before == nil {
		return crmcontracts.BulkSampleRow{}, skipped(crmcontracts.BulkSkipReasonNoPreviousOwner), nil
	}
	owner := ids.UUID(*before)
	if err := auth.EnsureAssignee(ctx, tx, owner); err != nil {
		return crmcontracts.BulkSampleRow{}, crmcontracts.BulkSkip{}, err
	}
	sample := crmcontracts.BulkSampleRow{
		Id: item.Id, Label: row.label,
		Before: crmcontracts.BulkRecordState{OwnerId: wireOwner(row.ownerID)},
		After:  crmcontracts.BulkRecordState{OwnerId: before},
	}
	reassigner, err := reassignerOf(target)
	if err != nil {
		return crmcontracts.BulkSampleRow{}, crmcontracts.BulkSkip{}, err
	}
	return sample, crmcontracts.BulkSkip{}, reassigner.reassign(ctx, tx, id, ids.From[ids.UserKind](owner), item.Version)
}

// undoSkipFor is bulkSkipFor for an undo, where a version that moved means the
// record changed after the batch rather than after a preview.
func undoSkipFor(err error) (crmcontracts.BulkSkip, error) {
	if errors.Is(err, apperrors.ErrVersionSkew) {
		return skipped(crmcontracts.BulkSkipReasonChangedSinceBatch), nil
	}
	return bulkSkipFor(err)
}

// restoreSkip turns an un-archive's refusal into why the record was left alone.
func restoreSkip(refusal *storekit.RestoreRefusal) crmcontracts.BulkSkip {
	switch refusal.Reason {
	case storekit.RestoreMerged:
		return skipped(crmcontracts.BulkSkipReasonMerged)
	case storekit.RestoreErased:
		return skipped(crmcontracts.BulkSkipReasonErased)
	case storekit.RestoreValueTaken:
		skip := skipped(crmcontracts.BulkSkipReasonValueTaken)
		skip.Code = &refusal.Detail
		return skip
	case storekit.RestoreNotArchived:
		return skipped(crmcontracts.BulkSkipReasonNoChange)
	}
	code, message := string(refusal.Reason), refusal.Error()
	return crmcontracts.BulkSkip{Reason: crmcontracts.BulkSkipReasonRefused, Code: &code, Message: &message}
}

// bulkLinks carries the links an undo's restores left behind because the
// record at their other end was still archived. Rows are restored in id order,
// so when two records of one change were linked, the earlier restore finds the
// later one archived; the later restore tries the link again once it is live.
type bulkLinks struct {
	pending  []storekit.LeftBehind
	relinked map[ids.UUID]bool
}

// settle takes one committed restore: what it brought back leaves the pending
// set, and the links it left behind join it.
func (l *bulkLinks) settle(report storekit.RestoreReport) {
	if l.relinked == nil {
		l.relinked = map[ids.UUID]bool{}
	}
	for _, id := range report.Relinked {
		l.relinked[id] = true
	}
	pending := l.pending[:0]
	for _, link := range l.pending {
		if !l.relinked[link.ID] {
			pending = append(pending, link)
		}
	}
	for _, left := range report.LeftBehind {
		if left.Kind == string(crmcontracts.BulkLeftBehindKindRelationship) {
			pending = append(pending, left)
		}
	}
	l.pending = pending
}

// stillBehind drops from left what a later restore brought back.
func (l *bulkLinks) stillBehind(left []crmcontracts.BulkLeftBehind) []crmcontracts.BulkLeftBehind {
	out := left[:0]
	for _, entry := range left {
		if !l.relinked[ids.UUID(entry.RefId)] {
			out = append(out, entry)
		}
	}
	return out
}
