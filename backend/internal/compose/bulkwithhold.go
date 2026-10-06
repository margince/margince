// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// A batch's stored result names records by id, with why each was left alone
// or what did not come back with it. The requester was shown every one of them
// when the change ran, but authority moves: a record the reader can no longer
// see is withheld from every later answer about the batch, whatever the stored
// row says.

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// withholdUnseen keeps the skips and left-behind entries whose record the
// reader may see now — the object grant and row scope of recordType — and, for
// a list, tag or link left behind, whose reference they may see too.
func withholdUnseen(
	ctx context.Context, tx pgx.Tx, recordType crmcontracts.BulkRecordType,
	skipped []crmcontracts.BulkSkip, left []crmcontracts.BulkLeftBehind,
) ([]crmcontracts.BulkSkip, []crmcontracts.BulkLeftBehind, error) {
	named := make([]ids.UUID, 0, len(skipped)+len(left))
	var lists []ids.UUID
	for _, skip := range skipped {
		named = append(named, ids.UUID(skip.Id))
	}
	for _, entry := range left {
		named = append(named, ids.UUID(entry.Id))
		if entry.Kind == crmcontracts.BulkLeftBehindKindList {
			lists = append(lists, ids.UUID(entry.RefId))
		}
	}
	visible, err := visibleRecords(ctx, tx, recordType, named)
	if err != nil {
		return nil, nil, err
	}
	seen := func(id openapi_types.UUID) bool { return visible[ids.UUID(id)] }
	outSkipped := make([]crmcontracts.BulkSkip, 0, len(skipped))
	for _, skip := range skipped {
		if seen(skip.Id) {
			outSkipped = append(outSkipped, skip)
		}
	}
	refSeen, err := referencesSeen(ctx, tx, lists)
	if err != nil {
		return nil, nil, err
	}
	outLeft := make([]crmcontracts.BulkLeftBehind, 0, len(left))
	for _, entry := range left {
		if seen(entry.Id) && refSeen(entry) {
			outLeft = append(outLeft, entry)
		}
	}
	return outSkipped, outLeft, nil
}

// referencesSeen answers whether the reader may see what one left-behind entry
// refers to. A child row of the record (an email, a domain) is the record's
// own and needs nothing more; a list is row-scoped; a tag or a link needs its
// object's read grant.
func referencesSeen(ctx context.Context, tx pgx.Tx, lists []ids.UUID) (func(crmcontracts.BulkLeftBehind) bool, error) {
	visibleLists, err := auth.VisibleSubset(ctx, tx, "list", lists)
	if err != nil {
		return nil, err
	}
	tags, err := readGranted(ctx, "tag")
	if err != nil {
		return nil, err
	}
	links, err := readGranted(ctx, "relationship")
	if err != nil {
		return nil, err
	}
	return func(entry crmcontracts.BulkLeftBehind) bool {
		switch entry.Kind {
		case crmcontracts.BulkLeftBehindKindList:
			return visibleLists[ids.UUID(entry.RefId)]
		case crmcontracts.BulkLeftBehindKindTag:
			return tags
		case crmcontracts.BulkLeftBehindKindRelationship:
			return links
		}
		return true
	}, nil
}

// readGranted reduces the object read gate to a verdict; only a denial is one.
func readGranted(ctx context.Context, object string) (bool, error) {
	err := auth.Require(ctx, object, principal.ActionRead)
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		return false, nil
	}
	return err == nil, err
}

// bulkBatchStillSeen answers whether a replayed bulk answer may be handed back:
// the caller may still read the batch, and every record its stored result
// names is one they may still see. A replay is refused whole rather than
// edited, because an edited body is one the product never produced.
func bulkBatchStillSeen(ctx context.Context, db *database.DB, batchID ids.UUID) error {
	return db.Tx(ctx, func(tx pgx.Tx) error {
		op, err := readBulkOperation(ctx, tx, batchID)
		if err != nil {
			return err
		}
		skipped, left, err := withholdUnseen(ctx, tx, op.recordType, op.result.Skipped, op.result.LeftBehind)
		if err != nil {
			return err
		}
		if len(skipped) != len(op.result.Skipped) || len(left) != len(op.result.LeftBehind) {
			return apperrors.ErrNotFound
		}
		return nil
	})
}
