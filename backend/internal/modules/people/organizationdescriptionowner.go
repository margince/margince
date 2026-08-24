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
// The answer lives in field_provenance, the same layer the logo asks
// (logoHeldByHuman), so "a human owns this field" has one answer in the
// product rather than one per column.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/gradionhq/margince/backend/internal/platform/database/storekit"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
)

// descriptionField is the field_provenance name both helpers key on. It is the
// COLUMN's name, which is what the stamp in applyEvidenceFields records for
// every column-backed field.
const descriptionField = "description"

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

// descriptionHeldByHuman reports whether a person authored this organization's
// description.
//
// No provenance row means nobody has claimed the column, and an automated
// writer may replace what another automated writer put there: every human
// writer of the description stamps, so the absence is informative rather than
// merely unknown.
func descriptionHeldByHuman(ctx context.Context, tx pgx.Tx, orgID ids.OrganizationID) (bool, error) {
	var human bool
	err := tx.QueryRow(ctx, `
		SELECT captured_by LIKE 'human:%'
		FROM field_provenance
		WHERE object_type = 'organization' AND object_id = $1 AND field_name = $2
		ORDER BY captured_at DESC, id DESC
		LIMIT 1`, orgID, descriptionField).Scan(&human)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read organization description provenance: %w", err)
	}
	return human, nil
}
