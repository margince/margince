// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Merging two contacts must not silently drop a role only one of them held.
//
// The relink's duplicate rule archives a source edge when the target already
// has "the same" edge, and what counts as the same is (kind, company, deal).
// For the two kinds whose uniqueness is keyed on the ROLE as well — a deal
// stakeholder and a billing contact — that rule is too coarse: it treats
// Acme's invoice recipient and Acme's approver as one edge and archives one of
// them.
//
// It fails quietly, which is why it is worth a test of its own. Archiving
// removes the row from the partial unique index BEFORE the relink runs, so no
// constraint fires, no error is returned, and the merge reports success. The
// account simply has no recipient afterwards.

import (
	"testing"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// billingRolesAt counts one company's live billing roles, by name.
func billingRolesAt(t *testing.T, companyID ids.UUID) []string {
	t.Helper()
	rows, err := OwnerConn(t).Query(t.Context(), `
		SELECT role FROM relationship
		 WHERE kind = 'billing_contact' AND company_id = $1 AND archived_at IS NULL
		 ORDER BY role`, companyID)
	if err != nil {
		t.Fatalf("reading billing roles: %v", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var role string
		if err := rows.Scan(&role); err != nil {
			t.Fatalf("scanning a role: %v", err)
		}
		out = append(out, role)
	}
	return out
}

func TestMergingTwoBillingContactsKeepsBothTheirRoles(t *testing.T) {
	e := Setup(t)
	admin := e.Admin()

	company := e.SeedCompany(t, "Acme", nil)
	source, err := e.Contacts.CreateContact(admin, contacts.CreateContactInput{
		FullName: "Recipient Contact", Source: "manual",
	})
	if err != nil {
		t.Fatalf("create source: %v", err)
	}
	target, err := e.Contacts.CreateContact(admin, contacts.CreateContactInput{
		FullName: "Approver Contact", Source: "manual",
	})
	if err != nil {
		t.Fatalf("create target: %v", err)
	}
	src, tgt := ContactIDOf(ids.UUID(source.Id)), ContactIDOf(ids.UUID(target.Id))

	// The same company, two DIFFERENT capacities. Merging the two contacts is
	// saying they were always one contact — who therefore holds both roles.
	seedBillingEdge(t, src.UUID, company, "recipient")
	seedBillingEdge(t, tgt.UUID, company, "approver")

	if _, err := e.Contacts.MergeContact(admin, src, tgt); err != nil {
		t.Fatalf("merge: %v", err)
	}

	roles := billingRolesAt(t, company)
	if len(roles) != 2 || roles[0] != "approver" || roles[1] != "recipient" {
		t.Fatalf("after the merge Acme's billing roles are %v, want [approver recipient] — "+
			"the merge archived a role the survivor did not hold, and the account lost its invoice recipient", roles)
	}
	// And both survive on the SURVIVOR, not orphaned on the merged-away row.
	for _, role := range roles {
		var holder ids.UUID
		if err := OwnerConn(t).QueryRow(t.Context(), `
			SELECT contact_id FROM relationship
			 WHERE kind = 'billing_contact' AND company_id = $1 AND role = $2 AND archived_at IS NULL`,
			company, role).Scan(&holder); err != nil {
			t.Fatalf("reading the %s holder: %v", role, err)
		}
		if holder != tgt.UUID {
			t.Errorf("the %s role sits on %s, want the survivor %s", role, holder, tgt)
		}
	}
}

func TestMergingTwoBillingContactsCollapsesTheSameRoleOnce(t *testing.T) {
	e := Setup(t)
	admin := e.Admin()

	company := e.SeedCompany(t, "Globex", nil)
	source, err := e.Contacts.CreateContact(admin, contacts.CreateContactInput{
		FullName: "Dup One", Source: "manual",
	})
	if err != nil {
		t.Fatalf("create source: %v", err)
	}
	target, err := e.Contacts.CreateContact(admin, contacts.CreateContactInput{
		FullName: "Dup Two", Source: "manual",
	})
	if err != nil {
		t.Fatalf("create target: %v", err)
	}
	src, tgt := ContactIDOf(ids.UUID(source.Id)), ContactIDOf(ids.UUID(target.Id))

	// The mirror case, without which a fix that simply stopped deduping would
	// look correct here: the SAME capacity on both sides really is one edge
	// after the merge, and two would violate uq_rel_billing_contact.
	seedBillingEdge(t, src.UUID, company, "recipient")
	seedBillingEdge(t, tgt.UUID, company, "recipient")

	if _, err := e.Contacts.MergeContact(admin, src, tgt); err != nil {
		t.Fatalf("merge: %v", err)
	}
	if roles := billingRolesAt(t, company); len(roles) != 1 {
		t.Fatalf("after merging two recipients Globex has %v, want exactly one", roles)
	}
	if _, err := e.Contacts.GetContact(admin, tgt, storekit.LiveOnly); err != nil {
		t.Fatalf("survivor unreadable after merge: %v", err)
	}
}

func TestMergingTwoCompaniesKeepsBothBillingRolesOfOneContact(t *testing.T) {
	e := Setup(t)
	admin := e.Admin()

	// The same gap on the other side of the same index: one contact is the
	// recipient at company A and the approver at company B. Merging A into B
	// does not make the two capacities one.
	source := e.SeedCompany(t, "Source Co", nil)
	target := e.SeedCompany(t, "Target Co", nil)
	holder, err := e.Contacts.CreateContact(admin, contacts.CreateContactInput{
		FullName: "Both Roles", Source: "manual",
	})
	if err != nil {
		t.Fatalf("create contact: %v", err)
	}
	contactID := ContactIDOf(ids.UUID(holder.Id))

	seedBillingEdge(t, contactID.UUID, source, "recipient")
	seedBillingEdge(t, contactID.UUID, target, "approver")

	if _, err := e.Contacts.MergeCompany(admin,
		ids.From[ids.CompanyKind](source), ids.From[ids.CompanyKind](target)); err != nil {
		t.Fatalf("merge companies: %v", err)
	}

	roles := billingRolesAt(t, target)
	if len(roles) != 2 || roles[0] != "approver" || roles[1] != "recipient" {
		t.Fatalf("after the company merge the survivor's billing roles are %v, want [approver recipient]", roles)
	}
}
