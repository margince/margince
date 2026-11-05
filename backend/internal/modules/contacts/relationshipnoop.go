// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"

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
	var out relationshipRow
	changed, err := storekit.Savepoint(ctx, tx, func(nested pgx.Tx) (bool, error) {
		var err error
		if out, err = demoteAndPatchRelationship(ctx, nested, current, id, in, capturedBy); err != nil {
			return false, err
		}
		_, moved := storekit.ChangedColumns(savedEdgeImage(current), savedEdgeImage(out))
		return len(moved) > 0, nil
	})
	if err != nil || !changed {
		return current, false, err
	}
	return out, true, nil
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

// savedEdgeImage is what a save can change on an edge: the audited fields and
// whose edge it is. A save by someone other than the author takes the edge over.
// That is how confirming a suggested seat works, so it is a change even when no
// field moves.
//
//craft:ignore naked-any the image is the schemaless column map storekit compares
func savedEdgeImage(rel relationshipRow) map[string]any {
	image := relationshipFieldImage(rel)
	image["captured_by"] = rel.CapturedBy
	return image
}
