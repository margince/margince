// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// Writing an accepted read-back value onto the ORGANIZATION column behind it.
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

func writeOrgColumn(ctx context.Context, tx pgx.Tx, orgID ids.OrganizationID, column, value string, overwrite bool) (bool, error) {
	if !overwrite {
		return applyUnclaimedOrgColumn(ctx, tx, orgID, column, value)
	}
	queries := map[string]string{
		columnLegalName: `UPDATE organization SET legal_name = $2, updated_at = now()
			WHERE id = $1 AND legal_name IS DISTINCT FROM $2`,
		columnIndustry: `UPDATE organization SET industry = $2, updated_at = now()
			WHERE id = $1 AND industry IS DISTINCT FROM $2`,
		columnAddress: `UPDATE organization SET address_line1 = $2, updated_at = now()
			WHERE id = $1 AND address_line1 IS DISTINCT FROM $2`,
		columnDescription: `UPDATE organization SET description = $2, updated_at = now()
			WHERE id = $1 AND description IS DISTINCT FROM $2
			AND ($2::text IS NULL OR length($2) <= 500)`,
	}
	query, ok := queries[column]
	if !ok {
		return false, fmt.Errorf("people: %q is not a coldstart-writable column", column)
	}
	// An empty value clears the column to NULL, not to "". The fill arm above
	// matches on IS NULL, so a column cleared to the empty string could never
	// be filled again by any later read — the record would look answered while
	// holding nothing, and no enrichment would ever correct it. The human
	// company form clears to NULL for the same reason (setCompanyColumn).
	tag, err := tx.Exec(ctx, query, orgID, emptyToNil(value))
	if err != nil {
		return false, fmt.Errorf("replace %s: %w", column, err)
	}
	return tag.RowsAffected() == 1, nil
}

// applyUnclaimedOrgColumn writes a read-back value onto a column nobody has
// claimed. For legal_name, industry and address that means the column is still
// empty, which each statement enforces with IS NULL. The description is the one
// column an automated read may also REPLACE, because the site is the authority
// on what a company sells — so for that one "unclaimed" means no person has
// authored it, and the check is below rather than in the statement.
func applyUnclaimedOrgColumn(ctx context.Context, tx pgx.Tx, orgID ids.OrganizationID, column, value string) (bool, error) {
	query, ok := coldStartColumns[column]
	if !ok {
		return false, fmt.Errorf("people: %q is not a coldstart-fillable column", column)
	}
	if column == descriptionField {
		held, err := descriptionHeldByHuman(ctx, tx, orgID)
		if err != nil {
			return false, err
		}
		if held {
			return false, nil
		}
	}
	// Nothing to fill. Writing "" here would satisfy this arm's own WHERE
	// legal_name IS NULL once and never again: the column would read as
	// answered while holding nothing, and no later read could correct it.
	if value == "" {
		return false, nil
	}
	tag, err := tx.Exec(ctx, query, orgID, value)
	if err != nil {
		return false, fmt.Errorf("fill %s: %w", column, err)
	}
	return tag.RowsAffected() == 1, nil
}

// carriesOrgName reports whether this apply could write a name column, and so
// whether it owes the organization-name lock.
//
// Presence of the field is the test, because that is what the loop in
// applyEvidenceFieldsWithOverwrite acts on: writeOrgColumn's overwrite arm
// matches on IS DISTINCT FROM, so an apply that clears legal_name to "" still
// writes the row — taking its row lock — and still reaches the re-check, which
// wants the name lock. Anything narrower than presence lets that apply take the
// two in the order that deadlocks against a human rename.
