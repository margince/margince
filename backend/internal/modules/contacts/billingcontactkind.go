// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The one relationship kind whose role is part of what the row MEANS: who
// handles a company's invoices, and in what capacity. Its vocabulary and its
// refusals live here rather than among the generic edge rules, because they
// are the only per-kind role rules in the module and mixing them in made the
// generic file read as though every kind had them.

// BillingContactKind names who handles a company's invoices: a contact↔company
// edge whose role says in what capacity. Exported because the company's finance
// projection reads it through compose, which cannot see the unexported kinds.
//
// It anchors on the COMPANY, unlike employment, which shares its endpoints.
// Whose fact is it that Acme's invoices go to this contact — Acme's, or the
// contact's? It is Acme's: one contact is a billing contact at one customer
// and nothing at the next, and the reader who maintains the list is looking at
// the company's finance tab. So the company's write authority governs the edge,
// and a caller who may see a contact but not change the customer cannot quietly
// redirect that customer's invoices.
const BillingContactKind = "billing_contact"

// BillingContactRoles is the bounded capacity vocabulary, in the order a
// reader thinks about an invoice: it is addressed to somebody, approved by
// somebody, and paid by somebody. The database holds the same three in
// rel_billing_contact_role; this is what lets the refusal name them before the
// constraint does, so a caller is told what to send rather than which
// constraint they broke.
var BillingContactRoles = []string{"recipient", "approver", "accounts_payable"}

// validBillingContactRole answers the one rule that is this kind's alone: the
// role is REQUIRED, and it is one of three. Every other kind keeps free text,
// so the check is asked per kind rather than of the column.
//
// Asked in Go as well as in the CHECK because the two answer different
// audiences. The constraint is the guarantee — no row exists without a role,
// whoever wrote it. This is the sentence the caller reads, naming the field and
// the three words it accepts, instead of a constraint name they cannot act on.
func validBillingContactRole(kind string, role *string) error {
	if kind != BillingContactKind {
		return nil
	}
	if role == nil || *role == "" {
		return &RequiredFieldError{Field: fieldRole}
	}
	for _, allowed := range BillingContactRoles {
		if *role == allowed {
			return nil
		}
	}
	return &BillingContactRoleError{Role: *role}
}

// roleKeyedDuplicateSQL is what makes two edges "the same" during a merge, for
// the kinds whose uniqueness is keyed on the role as well as the endpoints.
//
// A deal stakeholder and a billing contact are keyed on (parent, contact,
// role): Acme's invoice recipient and Acme's approver are two edges, and
// merging the contacts who held them says one contact holds both. Without
// this the merge archives one, and SILENTLY — archiving takes the row out of
// the partial unique index before the relink runs, so nothing conflicts and the
// merge reports success over an account that just lost its recipient.
//
// Every other kind keeps the coarser rule: there the role is a label on one
// edge rather than part of its identity, and two employments at one company are
// a duplicate whether or not the titles match.
//
// Both merge paths ask it of the same index, so it is written here rather than
// spelled in each — the half that drifted would be the one nobody noticed
// dropping rows.
//
// Held by: TestMergingTwoBillingContactsKeepsBothTheirRoles and
// TestMergingTwoCompaniesKeepsBothBillingRolesOfOneContact
// (backend/internal/compose/integration/mergerolededupe_integration_test.go),
// which fail if either path stops applying it.
const roleKeyedDuplicateSQL = `(a.kind NOT IN ('deal_stakeholder', '` + BillingContactKind + `')
		         OR b.role IS NOT DISTINCT FROM a.role)`
