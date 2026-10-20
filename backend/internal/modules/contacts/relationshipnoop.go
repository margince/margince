// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// patchRelationshipUnlessUnchanged lands a relationship patch and reports
// whether it changed the edge.
//
// The statement coalesces absent fields and derives the primary flag in SQL, so
// only the row it returns shows whether anything moved. By then the version
// trigger has run. The demotion and the patch therefore run in a savepoint that
// is rolled back when the result equals the edge the caller already had.
func patchRelationshipUnlessUnchanged(
	ctx context.Context, tx pgx.Tx, current relationshipRow, id ids.UUID, in UpdateRelationshipInput, capturedBy string,
) (relationshipRow, bool, error) {
	nested, err := tx.Begin(ctx)
	if err != nil {
		return relationshipRow{}, false, fmt.Errorf("open the relationship patch: %w", err)
	}
	out, err := demoteAndPatchRelationship(ctx, nested, current, id, in, capturedBy)
	if err != nil {
		if rbErr := nested.Rollback(ctx); rbErr != nil {
			return relationshipRow{}, false, errors.Join(err, rbErr)
		}
		return relationshipRow{}, false, err
	}
	_, moved := storekit.ChangedColumns(relationshipFieldImage(current), relationshipFieldImage(out))
	if len(moved) > 0 {
		if err := nested.Commit(ctx); err != nil {
			return relationshipRow{}, false, fmt.Errorf("commit the relationship patch: %w", err)
		}
		return out, true, nil
	}
	if err := nested.Rollback(ctx); err != nil {
		return relationshipRow{}, false, fmt.Errorf("withdraw the relationship patch that changed nothing: %w", err)
	}
	return current, false, nil
}

// demoteAndPatchRelationship is the two statements of a relationship edit. Its
// errors take the constraint mapping the insert uses, so a patch that breaks
// rel_dates gets the same named refusal as a create.
func demoteAndPatchRelationship(
	ctx context.Context, tx pgx.Tx, current relationshipRow, id ids.UUID, in UpdateRelationshipInput, capturedBy string,
) (relationshipRow, error) {
	if err := demoteForRelationshipPatch(ctx, tx, current, id, in); err != nil {
		return relationshipRow{}, err
	}
	out, err := patchRelationshipRow(ctx, tx, id, in, capturedBy)
	if err != nil {
		return relationshipRow{}, mapRelationshipConstraint(err, current.Kind)
	}
	return out, nil
}
