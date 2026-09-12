// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Reading a company's billing contacts: who handles its invoices, and in what
// capacity. One read, served to two surfaces — the company's contacts tab and
// its finance card — because those two panels answer the same question and a
// second query would be the one that forgets the visibility rule below.

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// BillingContact is one contact who handles a company's invoices, in the shape
// both reading surfaces need: who they are, in what capacity, and how to reach
// them.
//
// Email is read LIVE off the contact rather than copied onto the edge. An
// address stored on the relationship would be a second copy going stale the
// day they change it, and the invoice would then go to an address only this
// table still believes in.
type BillingContact struct {
	RelationshipID ids.UUID
	ContactID      ids.ContactID
	FullName       string
	Role           string
	Email          *string
	Title          *string
}

// BillingContactsFor lists one company's live billing contacts, newest role
// first within the vocabulary's own order.
//
// TWO scopes are applied, not one. The caller must be able to see the COMPANY,
// which the caller has already established by the time they assemble its page —
// and each named CONTACT must pass the caller's own contact row scope, applied
// here in SQL. Without the second, the finance card would name contacts the
// reader cannot open anywhere else in the product, which is how an edge becomes
// a way to enumerate a restricted roster.
//
// A contact hidden by that scope is simply absent. The count is of what the
// reader may see, never of the row total: reporting "4 billing contacts" over a
// list of two states the size of a set the reader was refused.
func (s *Store) BillingContactsFor(
	ctx context.Context, tx pgx.Tx, companyID ids.CompanyID,
) ([]BillingContact, error) {
	if err := auth.Require(ctx, "relationship", principal.ActionRead); err != nil {
		return nil, err
	}
	if err := auth.Require(ctx, anchorContact, principal.ActionRead); err != nil {
		return nil, err
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	companyPos := arg(companyID)
	scope, err := auth.ScopeClauseFor(ctx, "contact", "c", arg)
	if err != nil {
		return nil, err
	}
	sql := storekit.SQLf(`
		SELECT r.id, c.id, c.full_name, r.role, c.title,
		       (SELECT e.email FROM contact_email e
		         WHERE e.contact_id = c.id AND e.archived_at IS NULL`+
		ReachableEmailOrder+`
		         LIMIT 1)
		  FROM relationship r
		  JOIN contact c ON c.id = r.contact_id
		 WHERE r.kind = '`+BillingContactKind+`'
		   AND r.company_id = $%d
		   AND r.archived_at IS NULL
		   AND c.archived_at IS NULL`, companyPos)
	if scope != "" {
		sql += " AND " + scope
	}
	// Ordered by the role vocabulary's own sense — addressed, approved, paid —
	// rather than alphabetically, which would open the list on
	// accounts_payable. Then by name, so two contacts in one capacity hold a
	// stable order across reads.
	sql += `
		 ORDER BY CASE r.role
		            WHEN 'recipient' THEN 0
		            WHEN 'approver' THEN 1
		            ELSE 2
		          END, c.full_name, c.id`
	rows, err := tx.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]BillingContact, 0, 4)
	for rows.Next() {
		var b BillingContact
		if err := rows.Scan(&b.RelationshipID, &b.ContactID, &b.FullName, &b.Role, &b.Title, &b.Email); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// BillingCompany is one company a contact bills for, for the contact's own page:
// the inverse of the read above, asked from the other end.
type BillingCompany struct {
	RelationshipID ids.UUID
	CompanyID      ids.CompanyID
	CompanyName    string
	Role           string
}

// BillingRolesOf lists the companies one contact is a billing contact for.
//
// Scoped on the COMPANY here, mirroring the read above: the contact is already
// open in front of the reader, and what must not leak is which customers they
// bill for. A company outside the caller's scope is absent.
func (s *Store) BillingRolesOf(
	ctx context.Context, tx pgx.Tx, contactID ids.ContactID,
) ([]BillingCompany, error) {
	if err := auth.Require(ctx, "relationship", principal.ActionRead); err != nil {
		return nil, err
	}
	if err := auth.Require(ctx, companyEntity, principal.ActionRead); err != nil {
		return nil, err
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	contactPos := arg(contactID)
	scope, err := auth.ScopeClauseFor(ctx, companyEntity, "o", arg)
	if err != nil {
		return nil, err
	}
	sql := storekit.SQLf(`
		SELECT r.id, o.id, o.display_name, r.role
		  FROM relationship r
		  JOIN company o ON o.id = r.company_id
		 WHERE r.kind = '`+BillingContactKind+`'
		   AND r.contact_id = $%d
		   AND r.archived_at IS NULL
		   AND o.archived_at IS NULL`, contactPos)
	if scope != "" {
		sql += " AND " + scope
	}
	sql += ` ORDER BY o.display_name, o.id, r.role`
	rows, err := tx.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]BillingCompany, 0, 4)
	for rows.Next() {
		var b BillingCompany
		if err := rows.Scan(&b.RelationshipID, &b.CompanyID, &b.CompanyName, &b.Role); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}
