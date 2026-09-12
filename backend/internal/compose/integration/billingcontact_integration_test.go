// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Who handles a company's invoices, end to end: the role vocabulary that makes
// the edge mean something, the uniqueness that keeps one capacity one row, and
// the three surfaces that render the same list.
//
// Every rule here is enforced in Postgres or in SQL the store writes — a CHECK
// that is null-safe, a partial unique index, and two row-scope clauses — so a
// unit test over hand-built rows would prove none of them.

import (
	"net/http"
	"testing"
)

// billingContact posts one billing edge and reports what the server made of it.
func (e *relEnv) billingContact(t *testing.T, body AnyMap) (status int, id, detail string) {
	t.Helper()
	body["kind"] = "billing_contact"
	body["contact_id"] = e.contactID
	body["company_id"] = e.companyID
	body["source"] = "ui"
	var out struct {
		ID     string `json:"id"`
		Detail string `json:"detail"`
	}
	status = e.Call(t, "POST", "/v1/relationships", body, nil, &out)
	return status, out.ID, out.Detail
}

func TestABillingContactMustSayInWhatCapacity(t *testing.T) {
	e := setupRelationships(t)

	// No role at all. The database CHECK would refuse this too, but the point
	// of the Go check is the sentence: the caller is told the field and the
	// three words, not a constraint name.
	status, _, detail := e.billingContact(t, AnyMap{})
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("billing contact with no role → %d, want 422", status)
	}
	if detail == "" {
		t.Error("a refusal with no detail leaves the caller guessing which field to send")
	}

	// A word outside the vocabulary. A different mistake from omitting it, and
	// it gets a different sentence — one naming the three that are accepted.
	status, _, outside := e.billingContact(t, AnyMap{"role": "payer"})
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("billing contact with role=payer → %d, want 422", status)
	}
	if outside == detail {
		t.Error("an omitted role and a wrong role answer the same sentence — they are different mistakes with different fixes")
	}

	// And the case the rule must NOT refuse, without which a constraint that
	// refused everything would read identically to this one.
	if status, id, _ := e.billingContact(t, AnyMap{"role": "recipient"}); status != http.StatusCreated {
		t.Fatalf("billing contact with role=recipient → %d, want 201", status)
	} else if id == "" {
		t.Error("a created edge with no id cannot be changed or archived by the panel")
	}
}

func TestOneContactCanHoldSeveralBillingRolesButNotTheSameOneTwice(t *testing.T) {
	e := setupRelationships(t)

	if status, _, _ := e.billingContact(t, AnyMap{"role": "recipient"}); status != http.StatusCreated {
		t.Fatalf("recipient → %d", status)
	}
	// The small-customer case the key is shaped for: one office manager who is
	// addressed, approves and pays. Refusing this would force three contacts onto
	// an account that has one.
	if status, _, _ := e.billingContact(t, AnyMap{"role": "approver"}); status != http.StatusCreated {
		t.Fatalf("a second role for the same contact → %d, want 201", status)
	}
	if status, _, _ := e.billingContact(t, AnyMap{"role": "accounts_payable"}); status != http.StatusCreated {
		t.Fatalf("a third role for the same contact → %d, want 201", status)
	}

	// The same capacity twice is a duplicate, and uq_rel_billing_contact says so.
	if status, _, _ := e.billingContact(t, AnyMap{"role": "recipient"}); status != http.StatusConflict {
		t.Errorf("the same role twice → %d, want 409", status)
	}
}

func TestABillingRoleCannotBePatchedToAWordTheCreatePathRefuses(t *testing.T) {
	e := setupRelationships(t)

	status, id, _ := e.billingContact(t, AnyMap{"role": "recipient"})
	if status != http.StatusCreated {
		t.Fatalf("recipient → %d", status)
	}

	// Without the check on the patch path, the generic relationship PATCH is
	// the way around the create path's vocabulary: the row keeps its kind and
	// gains a role no reader can interpret.
	if status := e.Call(t, "PATCH", "/v1/relationships/"+id, AnyMap{"role": "payer"}, nil, nil); status != http.StatusUnprocessableEntity {
		t.Errorf("patching a billing role to an outside word → %d, want 422", status)
	}

	// Moving between the three is ordinary editing and must still work.
	if status := e.Call(t, "PATCH", "/v1/relationships/"+id, AnyMap{"role": "approver"}, nil, nil); status != http.StatusOK {
		t.Errorf("patching recipient → approver → %d, want 200", status)
	}
}

func TestAnEmploymentRoleStaysFreeText(t *testing.T) {
	e := setupRelationships(t)

	// The billing rule is asked per KIND, not of the column. A role vocabulary
	// applied to every kind would refuse every job title in the product.
	if status := e.Call(t, "POST", "/v1/relationships", AnyMap{
		"kind": "employment", "contact_id": e.contactID, "company_id": e.companyID,
		"role": "vp_of_something_nobody_enumerated", "source": "ui",
	}, nil, nil); status != http.StatusCreated {
		t.Errorf("an employment with a free-text role → %d, want 201", status)
	}
}

func TestTheCompanyPageAndTheFinanceCardNameTheSameBillingContacts(t *testing.T) {
	e := setupRelationships(t)

	for _, role := range []string{"accounts_payable", "recipient"} {
		if status, _, _ := e.billingContact(t, AnyMap{"role": role}); status != http.StatusCreated {
			t.Fatalf("%s → %d", role, status)
		}
	}

	type billing struct {
		RelationshipID string `json:"relationship_id"`
		ContactID      string `json:"contact_id"`
		FullName       string `json:"full_name"`
		Role           string `json:"role"`
	}

	var page struct {
		BillingContacts []billing `json:"billing_contacts"`
	}
	if status := e.Call(t, "GET", "/v1/companies/"+e.companyID+"/360", nil, nil, &page); status != http.StatusOK {
		t.Fatalf("company 360 → %d", status)
	}
	if len(page.BillingContacts) != 2 {
		t.Fatalf("company 360 lists %d billing contacts, want 2", len(page.BillingContacts))
	}
	// Ordered by capacity — addressed, then approved, then paid — rather than
	// alphabetically, which would open the list on accounts_payable.
	if page.BillingContacts[0].Role != "recipient" {
		t.Errorf("first billing contact is %q, want recipient — the list reads in invoice order", page.BillingContacts[0].Role)
	}

	var card struct {
		BillingContacts []billing `json:"billing_contacts"`
	}
	if status := e.Call(t, "GET", "/v1/companies/"+e.companyID+"/finance-summary", nil, nil, &card); status != http.StatusOK {
		t.Fatalf("finance summary → %d", status)
	}
	// The finance card reads through a compose adapter into the SAME store read
	// and the SAME projection. Two lists that could disagree would leave a
	// reader with no way to tell which panel is stale.
	if len(card.BillingContacts) != len(page.BillingContacts) {
		t.Fatalf("finance card lists %d, company page lists %d — one read, two answers",
			len(card.BillingContacts), len(page.BillingContacts))
	}
	for i := range card.BillingContacts {
		if card.BillingContacts[i] != page.BillingContacts[i] {
			t.Errorf("row %d differs between the finance card and the company page:\n  card = %+v\n  page = %+v",
				i, card.BillingContacts[i], page.BillingContacts[i])
		}
	}

	// And the contact's own page, the same edges asked from the other end.
	var subject struct {
		BillingRoles []struct {
			CompanyID   string `json:"company_id"`
			CompanyName string `json:"company_name"`
			Role        string `json:"role"`
		} `json:"billing_roles"`
	}
	if status := e.Call(t, "GET", "/v1/contacts/"+e.contactID+"/360", nil, nil, &subject); status != http.StatusOK {
		t.Fatalf("contact 360 → %d", status)
	}
	if len(subject.BillingRoles) != 2 {
		t.Fatalf("contact 360 lists %d billing roles, want 2", len(subject.BillingRoles))
	}
	for _, r := range subject.BillingRoles {
		if r.CompanyID != e.companyID {
			t.Errorf("billing role names company %s, want %s", r.CompanyID, e.companyID)
		}
		if r.CompanyName == "" {
			t.Error("a billing role with no company name renders as a role attached to nothing")
		}
	}
}

func TestArchivingABillingContactFreesTheRoleAndKeepsTheHistory(t *testing.T) {
	e := setupRelationships(t)

	status, id, _ := e.billingContact(t, AnyMap{"role": "recipient"})
	if status != http.StatusCreated {
		t.Fatalf("recipient → %d", status)
	}
	if status := e.Call(t, "DELETE", "/v1/relationships/"+id, nil, nil, nil); status != http.StatusOK {
		t.Fatalf("archive → %d, want 200", status)
	}

	// The unique index carries archived_at IS NULL, so naming a new recipient
	// after the old one left is a new fact rather than a duplicate.
	if status, _, _ := e.billingContact(t, AnyMap{"role": "recipient"}); status != http.StatusCreated {
		t.Errorf("a new recipient after the old one was archived → %d, want 201", status)
	}

	// And the archived row is gone from the projection without being gone from
	// the table: the audit trail keeps who was named and when.
	var page struct {
		BillingContacts []struct {
			RelationshipID string `json:"relationship_id"`
		} `json:"billing_contacts"`
	}
	if status := e.Call(t, "GET", "/v1/companies/"+e.companyID+"/360", nil, nil, &page); status != http.StatusOK {
		t.Fatalf("company 360 → %d", status)
	}
	if len(page.BillingContacts) != 1 {
		t.Fatalf("company 360 lists %d billing contacts after one was archived, want 1", len(page.BillingContacts))
	}
	if page.BillingContacts[0].RelationshipID == id {
		t.Error("the archived edge is still on the page")
	}
}
