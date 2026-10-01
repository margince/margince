// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The two list verbs of a bulk change: add a selection to a Shortlist, or take
// it off one. Each row goes through the Shortlist's one membership writer, the
// same one a record page or the list page uses, so a bulk change attributes,
// notes and records each membership exactly as a single add or remove does.

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// isListVerb says the verb changes a Shortlist's membership rather than the
// record: it needs list_id, and it needs no write authority over the record.
func isListVerb(verb crmcontracts.BulkVerb) bool {
	return verb == crmcontracts.BulkVerbAddToList || verb == crmcontracts.BulkVerbRemoveFromList
}

// withLists lets the engine run the list verbs; without it, while lists are
// switched off, a list verb names nothing and answers not found.
func (e *bulkEngine) withLists(store *collections.Store) *bulkEngine {
	e.lists = store
	return e
}

// admitListVerb refuses a list verb before any row is tried: lists off, or a
// list the caller may not change for this record type.
func (e *bulkEngine) admitListVerb(ctx context.Context, change bulkChange) error {
	if e.lists == nil {
		return apperrors.ErrNotFound
	}
	return e.lists.CheckShortlistChange(ctx, ids.From[ids.ListKind](*change.listID), string(change.recordType))
}

// applyMembership adds one record to the Shortlist or takes it off. A forward
// change holds the record to the version the caller was shown; an undo does
// not, because a membership never moved the record's version.
func applyMembership(
	ctx context.Context, tx pgx.Tx, target bulkTarget, change bulkChange, item crmcontracts.BulkItem, add bool,
) (bulkApplied, crmcontracts.BulkSkip, error) {
	id := ids.UUID(item.Id)
	row, err := target.lock(ctx, tx, id)
	if err != nil {
		skip, classifyErr := bulkSkipFor(err)
		return bulkApplied{}, skip, classifyErr
	}
	if change.undo == nil && row.version != item.Version {
		return bulkApplied{}, skipped(crmcontracts.BulkSkipReasonChangedSincePreview), nil
	}
	list := ids.From[ids.ListKind](*change.listID)
	member := collections.MemberChange{
		EntityType: string(change.recordType), EntityID: id, Note: change.note, Reason: collections.ReasonBulk,
	}
	if add {
		_, err = change.writers.lists.AddMemberTx(ctx, tx, list, member)
	} else {
		err = change.writers.lists.RemoveMemberTx(ctx, tx, list, member)
	}
	if errors.Is(err, collections.ErrAlreadyMember) || errors.Is(err, collections.ErrNotMember) {
		return bulkApplied{}, skipped(crmcontracts.BulkSkipReasonNoChange), nil
	}
	if err != nil {
		skip, classifyErr := bulkSkipFor(err)
		return bulkApplied{}, skip, classifyErr
	}
	before, after := !add, add
	sample := crmcontracts.BulkSampleRow{
		Id: item.Id, Label: row.label,
		Before: crmcontracts.BulkRecordState{OwnerId: wireOwner(row.ownerID), Listed: &before},
		After:  crmcontracts.BulkRecordState{OwnerId: wireOwner(row.ownerID), Listed: &after},
	}
	return bulkApplied{sample: sample, outcome: bulkOutcome{ID: item.Id, Version: row.version}}, crmcontracts.BulkSkip{}, nil
}

// withListsIf runs the list verbs over the installation's collections store
// when lists are switched on.
func (e *bulkEngine) withListsIf(on bool) *bulkEngine {
	if !on {
		return e
	}
	return e.withLists(NewCollectionsStore(e.db.Pool()))
}
