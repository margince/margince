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
	// Every key is the column's own name, which is the name the company update
	// path writes back: an Undo replays the image through that path.
	out := map[string]any{fieldDisplayName: displayName}
	for column, value := range map[string]*string{
		columnLegalName: legalName, columnIndustry: industry, columnDescription: description,
		columnSizeBand: sizeBand, columnCompanyPhone: phone, columnCompanyEmail: email,
		columnAddressLine1: address.Line1, columnAddressLine2: address.Line2,
		columnAddressCity: address.City, columnAddressRegion: address.Region,
		columnAddressPostalCode: address.PostalCode, columnAddressCountry: address.Country,
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
	out := map[string]any{fieldDisplayName: nil}
	for _, column := range []string{
		columnLegalName, columnIndustry, columnDescription, columnSizeBand, columnCompanyPhone, columnCompanyEmail,
	} {
		out[column] = nil
	}
	for _, column := range companyAddressColumns {
		out[column] = nil
	}
	return out
}

// changedColdStartColumns narrows two images to the columns that moved, and
// keeps the address whole. An Undo writes the address back as one object, and
// a part missing from the image would be cleared.
//
//craft:ignore naked-any same column-value contract as storekit.ChangedColumns
func changedColdStartColumns(before, after map[string]any) (map[string]any, map[string]any) {
	changedBefore, changedAfter := storekit.ChangedColumns(before, after)
	if !movedAddress(changedAfter) {
		return changedBefore, changedAfter
	}
	for _, column := range companyAddressColumns {
		changedBefore[column] = before[column]
		changedAfter[column] = after[column]
	}
	return changedBefore, changedAfter
}

// anchorSaveImages is the pair of images a company save records, narrowed to
// what moved so a re-sent form does not read as an edit. A save that minted the
// row has no before-image and no receipts, because its Undo archives the row.
func anchorSaveImages(ctx context.Context, tx pgx.Tx, target anchorTarget) (map[string]any, map[string]any, map[string]ProfileFieldReceipt, error) {
	after, err := readColdStartColumnImages(ctx, tx, target.id)
	if err != nil {
		return nil, nil, nil, err
	}
	if storekit.AbsentImage(target.before) {
		return nil, after, nil, nil
	}
	rows, err := readProfileFieldImages(ctx, tx, target.id)
	if err != nil {
		return nil, nil, nil, err
	}
	before, changed := changedColdStartColumns(target.before, after)
	return before, changed, profileFieldReceipts(target.rows, rows, changed), nil
}
