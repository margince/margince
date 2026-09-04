// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// Who owns an organization's description.
//
// The column is written by two kinds of author. A person types it into the
// company header or the company form; a site read fills it from the company's
// own website. A site read may replace what another automated writer put there
// — that is what lets a crawl correct a description an agent wrote from a
// meeting transcript — and must never replace a person's sentence.
//
// The header's summary is meant to say what the company SELLS, and the site is
// what knows that. An agent creating a record while holding a meeting
// transcript writes a summary of the MEETING there instead: true, about the
// wrong subject, and under the older fill-where-blank rule it permanently
// blocked the read that could have corrected it, because whoever wrote first
// won. Replacing an unclaimed value is what removes that trap.
//
// The answer lives in field_provenance, the same layer the logo asks
// (logoHeldByHuman), so "a human owns this field" has one answer in the
// product rather than one per column.

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

// describedDifferently reports whether an organization patch actually moved the
// description, as opposed to merely naming it.
//
// storekit.Patch records an assignment without comparing, so a request that
// re-sends the value already stored still puts the key in `after`. Treating
// that as authorship is what would let an agent re-sending a person's own
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

// stampDescriptionAuthor records who authored this organization's description,
// so a later site read can tell whether it may replace the sentence. Both edit
// paths call it — the header's inline edit and the company form — rather than
// each spelling the stamp its own way.
//
// WHO is carried by `by`, not by the source: it is the authenticated principal,
// so an agent editing through update_record stamps agent:<id> and does not hold
// the column, while a person stamps human:<id> and does. The source names the
// CHANNEL, which for both callers is a person's form on this surface — the
// agent case is a governed write against the same endpoint, and reporting it as
// a different channel would say something about the request that is not true.
func stampDescriptionAuthor(ctx context.Context, tx pgx.Tx, orgID ids.OrganizationID, by string) error {
	return storekit.StampFields(ctx, tx, "organization", orgID.UUID, companySourceHuman, by,
		[]storekit.FieldStamp{{Field: descriptionField}})
}

// carryDescriptionAuthor moves the description's author from a merged-away
// organization to the survivor that just inherited its sentence, so the value
// and the claim on it stay together. Nothing is written when the retired record
// had no provenance row: the survivor then holds an unclaimed description,
// which is the truth about it.
func carryDescriptionAuthor(ctx context.Context, tx pgx.Tx, from, to ids.OrganizationID) error {
	var source, by string
	err := tx.QueryRow(ctx, `
		SELECT source, captured_by
		FROM field_provenance
		WHERE object_type = 'organization' AND object_id = $1 AND field_name = $2
		ORDER BY captured_at DESC, id DESC
		LIMIT 1`, from, descriptionField).Scan(&source, &by)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read merged-away description provenance: %w", err)
	}
	return storekit.StampFields(ctx, tx, "organization", to.UUID, source, by,
		[]storekit.FieldStamp{{Field: descriptionField}})
}

// descriptionHeldByHuman reports whether a person authored this organization's
// description. The column and its provenance move together — a human clearing
// the description stamps it too — so the field's own answer is the whole test,
// which is where it differs from the logo one screen over.
func descriptionHeldByHuman(ctx context.Context, tx pgx.Tx, orgID ids.OrganizationID) (bool, error) {
	return orgFieldHeldByHuman(ctx, tx, orgID, descriptionField)
}
