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
// Whose fact is it that Acme's invoices go to this person — Acme's, or the
// person's? It is Acme's: the same person is a billing contact at one customer
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
