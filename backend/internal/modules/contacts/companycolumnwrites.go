// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Each writer names this catalog directly so mutation gates can attribute its SQL
// to the entry point that must enforce authorization and concurrency.

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/textcut"
)

// companyWriteAuthority says what a writer may do to a column somebody has already
// answered. It has no zero value on purpose — a writer states which it holds.
type companyWriteAuthority uint8

const (
	// fillUnclaimed writes only where the column is still unanswered.
	fillUnclaimed companyWriteAuthority = iota + 1
	// replaceStanding writes over a value already there.
	replaceStanding
)

// companyColumnWrite keys each statement by the authority needed to apply it.
type companyColumnWrite map[companyWriteAuthority]string

func (w companyColumnWrite) statementFor(authority companyWriteAuthority) string {
	return w[authority]
}

// companyColumnWrites is the one place a company's editable columns are
// written. The identifier is never a bind parameter: the statement is fixed
// here and only values bind.
var companyColumnWrites = map[string]companyColumnWrite{
	columnLegalName: {
		fillUnclaimed:   `UPDATE company SET legal_name = $2 WHERE id = $1 AND archived_at IS NULL AND legal_name IS NULL`,
		replaceStanding: `UPDATE company SET legal_name = $2 WHERE id = $1 AND archived_at IS NULL AND legal_name IS DISTINCT FROM $2`,
	},
	columnIndustry: {
		fillUnclaimed:   `UPDATE company SET industry = $2 WHERE id = $1 AND archived_at IS NULL AND industry IS NULL`,
		replaceStanding: `UPDATE company SET industry = $2 WHERE id = $1 AND archived_at IS NULL AND industry IS DISTINCT FROM $2`,
	},
	// Address parts move together. Filling requires every part to be empty;
	// replacement clears old parts that the new address does not contain.
	columnAddress: {
		fillUnclaimed: `UPDATE company SET address_line1 = $2, address_line2 = $3, address_region = $4,
		       address_postal_code = $5, address_city = $6, address_country = $7
		 WHERE id = $1 AND archived_at IS NULL
		   AND address_line1 IS NULL AND address_line2 IS NULL AND address_region IS NULL
		   AND address_postal_code IS NULL AND address_city IS NULL AND address_country IS NULL`,
		replaceStanding: `UPDATE company SET address_line1 = $2, address_line2 = $3, address_region = $4,
		       address_postal_code = $5, address_city = $6, address_country = $7
		 WHERE id = $1 AND archived_at IS NULL
		   AND (address_line1, address_line2, address_region, address_postal_code, address_city, address_country)
		       IS DISTINCT FROM ($2::text, $3::text, $4::text, $5::text, $6::text, $7::text)`,
	},
	// Clearing is permitted only by replacement; filling with NULL would leave no answer.
	columnDescription: {
		fillUnclaimed: `UPDATE company SET description = $2 WHERE id = $1 AND archived_at IS NULL
		       AND description IS NULL AND $2::text IS NOT NULL`,
		replaceStanding: `UPDATE company SET description = $2 WHERE id = $1 AND archived_at IS NULL
		          AND description IS DISTINCT FROM $2`,
	},
}

// A refused split leaves existing structured fields untouched during automatic fill.
// An explicit replacement preserves the whole text in the profile field.
func writeCompanyAddress(ctx context.Context, tx pgx.Tx, companyID ids.CompanyID, raw string, authority companyWriteAuthority) (bool, error) {
	address, split := SplitAddressText(raw)
	if !split && authority == fillUnclaimed {
		return false, nil
	}
	tag, err := tx.Exec(ctx, companyColumnWrites[columnAddress].statementFor(authority), addressWriteArguments(companyID, address)...)
	if err != nil {
		return false, fmt.Errorf("write company address: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

// addressWriteArguments binds the address statements, in their placeholder order.
func addressWriteArguments(companyID ids.CompanyID, a crmcontracts.Address) []any {
	return []any{companyID, a.Line1, a.Line2, a.Region, a.PostalCode, a.City, a.Country}
}

// CompanyDescriptionMax is what company_description_length admits, in
// characters. The column refuses a longer write, so every writer bounds or
// refuses the value before it gets there.
const CompanyDescriptionMax = 500

// The source can exceed its display column. Trim on rune and word boundaries
// without changing the source or inventing punctuation in the display value.
func headerLine(value string) string {
	cut := textcut.Runes(value, CompanyDescriptionMax)
	if cut == value {
		return value
	}
	if at := strings.LastIndexAny(cut, " \t\n"); at > 0 {
		return strings.TrimRight(cut[:at], " \t\n")
	}
	return cut
}

// companyColumnArgument is the value a company column write binds. An empty
// value binds NULL, so an unfilled field never reads as the empty answer.
// The industry column binds the code the text names. Unmapped reports text that
// names no code: the human form clears the column, read-back paths write nothing.
func companyColumnArgument(column, value string) (stored *string, unmapped bool) {
	if column == columnIndustry && value != "" {
		write := resolveIndustryWrite(&value)
		return write.code, write.code == nil
	}
	return emptyToNil(value), false
}
