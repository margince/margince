// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Each writer names this catalog directly so mutation gates can attribute its SQL
// to the entry point that must enforce authorization and concurrency.

import (
	"strings"

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
	// A scraped address cannot replace an existing structured address on the fill path.
	columnAddress: {
		fillUnclaimed: `UPDATE company SET address_line1 = $2 WHERE id = $1 AND archived_at IS NULL
		       AND address_line1 IS NULL AND address_city IS NULL AND address_postal_code IS NULL`,
		replaceStanding: `UPDATE company SET address_line1 = $2 WHERE id = $1 AND archived_at IS NULL
		          AND address_line1 IS DISTINCT FROM $2`,
	},
	// Clearing is permitted only by replacement; filling with NULL would leave no answer.
	columnDescription: {
		fillUnclaimed: `UPDATE company SET description = $2 WHERE id = $1 AND archived_at IS NULL
		       AND description IS NULL AND $2::text IS NOT NULL`,
		replaceStanding: `UPDATE company SET description = $2 WHERE id = $1 AND archived_at IS NULL
		          AND description IS DISTINCT FROM $2`,
	},
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
