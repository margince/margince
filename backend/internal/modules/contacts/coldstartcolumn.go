// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Writing an accepted read-back value onto the COMPANY column behind it.
//
// The evidence row lands for every accepted field; a column moves only where
// one exists and only under the claim rules below, which is the whole reason
// this is its own concern rather than a branch inside the apply loop.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func writeCompanyColumn(ctx context.Context, tx pgx.Tx, companyID ids.CompanyID, column, value string, overwrite bool) (bool, error) {
	if !overwrite {
		return applyUnclaimedCompanyColumn(ctx, tx, companyID, column, value)
	}
	if column == columnAddress {
		return writeCompanyAddress(ctx, tx, companyID, value, replaceStanding)
	}
	write, ok := companyColumnWrites[column]
	if !ok {
		return false, fmt.Errorf("contacts: %q is not a coldstart-writable column", column)
	}
	// An empty value clears the column to NULL, not to "". The fill arm below
	// matches on IS NULL, so a column cleared to the empty string could never
	// be filled again by any later read — the record would look answered while
	// holding nothing, and no enrichment would ever correct it. The human
	// company form clears to NULL for the same reason (setCompanyColumn).
	stored, unmapped := companyColumnArgument(column, value)
	if unmapped {
		return false, nil
	}
	tag, err := tx.Exec(ctx, write.statementFor(replaceStanding), companyID, stored)
	if err != nil {
		return false, fmt.Errorf("replace %s: %w", column, err)
	}
	return tag.RowsAffected() == 1, nil
}

// Read-back fields fill unclaimed columns without replacing standing values.
func applyUnclaimedCompanyColumn(ctx context.Context, tx pgx.Tx, companyID ids.CompanyID, column, value string) (bool, error) {
	write, ok := companyColumnWrites[column]
	if !ok {
		return false, fmt.Errorf("contacts: %q is not a coldstart-fillable column", column)
	}
	if value == "" {
		return false, nil
	}
	if column == columnAddress {
		return writeCompanyAddress(ctx, tx, companyID, value, fillUnclaimed)
	}
	stored, unmapped := companyColumnArgument(column, value)
	if unmapped {
		return false, nil
	}
	tag, err := tx.Exec(ctx, write.statementFor(fillUnclaimed), companyID, stored)
	if err != nil {
		return false, fmt.Errorf("fill %s: %w", column, err)
	}
	return tag.RowsAffected() == 1, nil
}
