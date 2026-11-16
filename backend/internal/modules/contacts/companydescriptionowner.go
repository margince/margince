// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Description provenance keeps user-authored text protected during corrections.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// descriptionField is the field_provenance name both helpers key on. It is the
// COLUMN's name, which is what the stamp in applyEvidenceFields records for
// every column-backed field.
const descriptionField = "description"

// describedDifferently reports whether a company patch actually moved the
// description, as opposed to merely naming it.
//
// storekit.Patch records an assignment without comparing, so a request that
// re-sends the value already stored still puts the key in `after`. Treating
// that as authorship is what would let an agent re-sending a contact's own
// sentence take the column away from them, so the values are compared here.
//
// The two images hold different types by construction: `before` carries the
// contract's *string as read from the row, `after` the string the request
// supplied. A nil before is a column that held nothing, which any non-empty
// value differs from.
func describedDifferently(before, after map[string]any) bool {
	next, sent := after[descriptionField].(string)
	if !sent {
		return false
	}
	switch prev := before[descriptionField].(type) {
	case *string:
		if prev == nil {
			return next != ""
		}
		return *prev != next
	case string:
		return prev != next
	default:
		// No before image for the column: the patch named it, and nothing says
		// it already held this value.
		return true
	}
}

// stampDescriptionAuthor records who authored this company's description,
// so a later site read can tell whether it may replace the sentence. Both edit
// paths call it — the header's inline edit and the company form — rather than
// each spelling the stamp its own way.
//
// A user and their agent both hold the column. The source says which of them
// wrote it, as every company fact does.
func stampDescriptionAuthor(ctx context.Context, tx pgx.Tx, companyID ids.CompanyID, by string) error {
	return storekit.StampFields(ctx, tx, "company", companyID.UUID, companyAuthorSource(ctx), by,
		[]storekit.FieldStamp{{Field: descriptionField}})
}

// provenanceCarriedOnMerge are the survivorship-filled fields whose author
// follows the value onto the survivor.
var provenanceCarriedOnMerge = []string{descriptionField, fieldLegalName, fieldIndustry, linkedInField}

// carryFieldAuthors moves the author of each field the survivor just inherited
// from a merged-away company, so the value and the claim on it stay together.
// A field the retired record had no provenance for stays unclaimed, which is
// the truth about it.
func carryFieldAuthors(ctx context.Context, tx pgx.Tx, from, to ids.CompanyID, filled map[string]any) error {
	for _, field := range provenanceCarriedOnMerge {
		if _, inherited := filled[field]; !inherited {
			continue
		}
		var source, by string
		err := tx.QueryRow(ctx, `
			SELECT source, captured_by
			FROM field_provenance
			WHERE object_type = 'company' AND object_id = $1 AND field_name = $2
			ORDER BY captured_at DESC, id DESC
			LIMIT 1`, from, field).Scan(&source, &by)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return fmt.Errorf("read merged-away %s provenance: %w", field, err)
		}
		if err := storekit.StampFields(ctx, tx, "company", to.UUID, source, by,
			[]storekit.FieldStamp{{Field: field}}); err != nil {
			return err
		}
	}
	return nil
}

// descriptionHeldByHuman reports whether a contact authored this company's
// description. The column and its provenance move together — a human clearing
// the description stamps it too — so the field's own answer is the whole test,
// which is where it differs from the logo one screen over.
func descriptionHeldByHuman(ctx context.Context, tx pgx.Tx, companyID ids.CompanyID) (bool, error) {
	return companyFieldHeldByAUser(ctx, tx, companyID, descriptionField)
}
