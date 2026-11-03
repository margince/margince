// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The company COLUMN images an accepted cold-start read-back diffs
// against. They live apart from the apply that uses them because field
// history is their whole reason: the write path reports only THAT a column
// changed, so what it WAS has to be read and carried, and the rules for
// reading it — the row lock, the field-not-column keying, and the empty
// image a freshly created record diffs against — are one concern.

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// columnSizeBand is the size band's column, which is also its field-history key.
const columnSizeBand = "size_band"

// readColdStartColumnImages reads the columns an apply may touch, so the audit
// row can carry what each one WAS. The column writes report only whether they
// changed something, never what they replaced, so the before image has to be
// read: field history projects per field from before/after, and an image-less
// audit row gives it nothing to diff.
func readColdStartColumnImages(ctx context.Context, tx pgx.Tx, companyID ids.CompanyID) (map[string]any, error) {
	var displayName string
	var legalName, industry, description, sizeBand, phone, email *string
	var address crmcontracts.Address
	// FOR UPDATE, and it is the audit that needs it: the before image and the
	// apply that follows must describe ONE transaction's work. Read unlocked,
	// a concurrent update landing between them lands inside this diff, and
	// field history attributes somebody else's edit to this acceptance.
	//
	// CALLER OBLIGATION: hold lockCompanyNameWrites before calling this. Every
	// writer of a company name takes that workspace-wide lock first and
	// the row lock second (see its own doc); a caller that reaches this row
	// lock without it inverts the pair and deadlocks against a human rename.
	if err := tx.QueryRow(ctx,
		`SELECT display_name, legal_name, industry, description, size_band, phone, email,
		        address_line1, address_line2, address_region, address_postal_code, address_city, address_country
		   FROM company WHERE id = $1 FOR UPDATE`,
		companyID).Scan(&displayName, &legalName, &industry, &description, &sizeBand, &phone, &email,
		&address.Line1, &address.Line2, &address.Region, &address.PostalCode, &address.City, &address.Country); err != nil {
		return nil, fmt.Errorf("read company column images: %w", err)
	}
	// Field history uses registered_address for all six address columns.
	// Their combined image keeps a city-only edit visible in that history.
	out := map[string]any{fieldDisplayName: displayName}
	var addressLine *string
	if line := formatAddressLine(address); line != "" {
		addressLine = &line
	}
	// size_band has no profile-field twin: the column IS the surface, so its
	// own name is the field-history key.
	for column, value := range map[string]*string{
		fieldLegalName: legalName, fieldIndustry: industry,
		fieldRegisteredAddress: addressLine, fieldOfferSummary: description,
		columnSizeBand: sizeBand, columnCompanyPhone: phone, columnCompanyEmail: email,
	} {
		if value == nil {
			// An empty column reads as an explicit null in the image, never as
			// an absent key: "it had nothing" and "nobody looked" are different
			// answers, and field history renders them differently.
			out[column] = nil
			continue
		}
		out[column] = *value
	}
	return out, nil
}

// emptyColdStartColumnImages is the before-image of a record that did not
// exist: every field explicitly null, so a create diffs against nothing rather
// than against itself.
func emptyColdStartColumnImages() map[string]any {
	return map[string]any{
		fieldDisplayName:       nil,
		fieldLegalName:         nil,
		fieldIndustry:          nil,
		fieldRegisteredAddress: nil,
		fieldOfferSummary:      nil,
		columnSizeBand:         nil,
		columnCompanyPhone:     nil,
		columnCompanyEmail:     nil,
	}
}

// anchorSaveImages is the pair of images a company save records: the columns as
// the resolve found them, against the row as the save leaves it, narrowed to
// what actually moved so a re-sent form does not present every field it echoed
// back as an edit. A save that MINTED the row has no before-image, and its
// after image stays whole — every column the create wrote is a change from
// nothing.
func anchorSaveImages(ctx context.Context, tx pgx.Tx, target anchorTarget) (map[string]any, map[string]any, error) {
	after, err := readColdStartColumnImages(ctx, tx, target.id)
	if err != nil {
		return nil, nil, err
	}
	if storekit.AbsentImage(target.before) {
		return nil, after, nil
	}
	before, changed := storekit.ChangedColumns(target.before, after)
	return before, changed, nil
}

// formatAddressLine spells the address columns as one line, in the order a
// letter prints them, skipping the empty ones.
func formatAddressLine(a crmcontracts.Address) string {
	var parts []string
	for _, part := range []string{
		pointerValue(a.Line1), pointerValue(a.Line2),
		strings.TrimSpace(pointerValue(a.PostalCode) + " " + pointerValue(a.City)),
		pointerValue(a.Region), pointerValue(a.Country),
	} {
		if part != "" {
			parts = append(parts, part)
		}
	}
	return strings.Join(parts, ", ")
}
