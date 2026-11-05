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

// applyUnclaimedCompanyColumn writes a read-back value onto a column nobody has
// claimed. For legal_name, industry and address that means the column is still
// empty, which each fill statement enforces. The description is the one column
// an automated read may also REPLACE, because the site is the authority on what
// a company sells — so for that one "unclaimed" means no contact has authored it,
// the check is below rather than in the statement, and the statement it then
// sends is the replacing one.
func applyUnclaimedCompanyColumn(ctx context.Context, tx pgx.Tx, companyID ids.CompanyID, column, value string) (bool, error) {
	write, ok := companyColumnWrites[column]
	if !ok {
		return false, fmt.Errorf("contacts: %q is not a coldstart-fillable column", column)
	}
	authority, value, err := unclaimedCompanyColumn(ctx, tx, companyID, column, value)
	if err != nil || value == "" {
		return false, err
	}
	if column == columnAddress {
		return writeCompanyAddress(ctx, tx, companyID, value, authority)
	}
	stored, unmapped := companyColumnArgument(column, value)
	if unmapped {
		return false, nil
	}
	tag, err := tx.Exec(ctx, write.statementFor(authority), companyID, stored)
	if err != nil {
		return false, fmt.Errorf("fill %s: %w", column, err)
	}
	return tag.RowsAffected() == 1, nil
}

func unclaimedCompanyColumn(ctx context.Context, tx pgx.Tx, companyID ids.CompanyID, column, value string) (companyWriteAuthority, string, error) {
	authority := fillUnclaimed
	if column == columnDescription {
		held, err := descriptionHeldByHuman(ctx, tx, companyID)
		if err != nil {
			return 0, "", err
		}
		if held {
			return 0, "", nil
		}
		authority = replaceStanding
		// The company header uses the same bounded text as the human form.
		value = headerLine(value)
	}
	// An empty string must not permanently claim an unanswered column.
	if value == "" {
		return 0, "", nil
	}
	return authority, value, nil
}
