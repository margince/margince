// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Which values on a contact a purchase put there and still owns.
//
// Two readers ask it: the contact read, which marks those values as bought, and
// the revert behind "Delete bought data", which takes them back. Both read the
// same per-table predicate below, so the page cannot mark a value the revert
// would leave standing because it no longer counts as the purchase's. The
// revert adds its own conditions on top (deletability); it never loosens these.
//
// Every predicate relates a target row `t` to its provider_applied_field row `f`.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const (
	// boughtTitleSQL: a column has no row of its own, so the value is the proof.
	boughtTitleSQL = `t.id = f.contact_id AND f.target_field = '` + fieldTitle + `'
		AND t.title = f.applied_value`
	// boughtSocialSQL: by row id, because the form save replaces every social
	// row and a re-saved handle carrying the same string is somebody else's.
	boughtSocialSQL = `t.id = f.target_row_id AND t.contact_id = f.contact_id`
	// boughtChildRowSQL: a re-typed address or number carries the typist's source.
	boughtChildRowSQL = `t.id = f.target_row_id AND t.contact_id = f.contact_id
		AND t.source = f.provider AND t.archived_at IS NULL`
	// boughtEmploymentSQL: an import may link a purchase to an edge a colleague
	// recorded; that edge is supported by the purchase, not bought.
	boughtEmploymentSQL = `t.id = f.target_row_id AND t.contact_id = f.contact_id
		AND t.archived_at IS NULL AND t.captured_by = 'connector:' || t.source`
)

// boughtPredicates is the predicate per target table, as the tests walk it.
var boughtPredicates = map[string]string{
	entityContact:      boughtTitleSQL,
	tableContactSocial: boughtSocialSQL,
	tableContactEmail:  boughtChildRowSQL,
	tableContactPhone:  boughtChildRowSQL,
	tableRelationship:  boughtEmploymentSQL,
}

// boughtFieldsSQL lists one contact's ledger rows whose target still holds the
// bought value, newest first, formatted with the contact's placeholder and the employment
// visibility clause.
const boughtFieldsSQL = `
	SELECT f.target_table, f.target_field, f.target_row_id, f.provider, f.applied_at
	  FROM provider_applied_field f
	 WHERE f.contact_id = $%d
	   AND ((f.target_table = 'contact'
	         AND EXISTS (SELECT 1 FROM contact t WHERE ` + boughtTitleSQL + `))
	     OR (f.target_table = 'contact_social'
	         AND EXISTS (SELECT 1 FROM contact_social t WHERE ` + boughtSocialSQL + `))
	     OR (f.target_table = 'contact_email'
	         AND EXISTS (SELECT 1 FROM contact_email t WHERE ` + boughtChildRowSQL + `))
	     OR (f.target_table = 'contact_phone'
	         AND EXISTS (SELECT 1 FROM contact_phone t WHERE ` + boughtChildRowSQL + `))
	     OR (f.target_table = 'relationship'
	         AND EXISTS (SELECT 1 FROM relationship t
	                       JOIN company company ON company.id = t.company_id
	                      WHERE ` + boughtEmploymentSQL + ` AND %s)))
	 ORDER BY f.applied_at DESC, f.id`

// attachBoughtFields stamps which of the contact's values a purchase still owns.
//
// Contact-read authority covers every target but the employment: an edge names
// a company as a pair, so it is listed only under the same edge and company
// gates the contact's employer is read under.
func attachBoughtFields(ctx context.Context, tx pgx.Tx, c *crmcontracts.Contact) error {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	contact := arg(ids.UUID(c.Id))
	employmentVisible, visible, err := employerScope(ctx, "t", arg)
	if err != nil {
		return err
	}
	if !visible {
		// A refusal part-way through may have bound the gates it did ask.
		employmentVisible, args = "FALSE", args[:contact]
	}
	rows, err := tx.Query(ctx, storekit.SQLf(boughtFieldsSQL, contact, employmentVisible), args...)
	if err != nil {
		return fmt.Errorf("contacts: reading which values were bought: %w", err)
	}
	defer rows.Close()
	bought := []crmcontracts.BoughtField{}
	seen := map[string]bool{}
	for rows.Next() {
		var table, field, provider string
		var rowID *ids.UUID
		var at time.Time
		if err := rows.Scan(&table, &field, &rowID, &provider, &at); err != nil {
			return fmt.Errorf("contacts: reading which values were bought: %w", err)
		}
		target := boughtTarget(table, field, rowID)
		if seen[target] {
			continue
		}
		seen[target] = true
		bought = append(bought, crmcontracts.BoughtField{
			Target: target, Provider: crmcontracts.Provider(provider), AppliedAt: at,
		})
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("contacts: reading which values were bought: %w", err)
	}
	c.BoughtFields = &bought
	return nil
}

// boughtTarget names a filled value the way the contract spells it: the field
// for a column or a handle, the kind and row id for a child row or an edge.
func boughtTarget(table, field string, rowID *ids.UUID) string {
	switch {
	case table == tableRelationship && rowID != nil:
		return fieldEmployment + ":" + rowID.String()
	case (table == tableContactEmail || table == tableContactPhone) && rowID != nil:
		return field + ":" + rowID.String()
	default:
		return field
	}
}
