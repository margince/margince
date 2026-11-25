// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/company360"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/finance"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The company page and the finance card read who handles the invoices through
// one store read. So a scoped reader is told the same withheld count and the
// same colleagues on both.
func TestTheFinanceCardAndTheCompanyPageWithholdTheSameBillingContacts(t *testing.T) {
	e := integration.Setup(t)
	company := e.SeedCompany(t, "Shared Account", &e.Rep1)
	mine := e.SeedContact(t, "Visible Payer", &e.Rep1)
	theirs := e.SeedContact(t, "Hidden Payer", &e.Rep3)
	e.MakeCapturePrivate(t, "contact", theirs, e.Rep3)
	for contact, role := range map[ids.UUID]string{mine: "recipient", theirs: "approver"} {
		if _, err := integration.OwnerConn(t).Exec(context.Background(), `
			INSERT INTO relationship (kind, contact_id, company_id, role, source, captured_by)
			VALUES ('billing_contact', $1, $2, $3, 'test', 'human:x')`, contact, company, role); err != nil {
			t.Fatalf("seeding a billing edge: %v", err)
		}
	}

	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, principal.Permissions{
		RoleKeys: []string{"rep"},
		Objects: map[string]principal.ObjectGrant{
			"contact": {Read: true}, "company": {Read: true}, "relationship": {Read: true}, "finance": {Read: true},
		},
		RowScope: principal.RowScopeTeam,
	})
	companyID := ids.From[ids.CompanyKind](company)
	page, err := company360.NewService(e.Pool, e.Contacts, e.Deals, e.Projects, approvals.NewService(e.DB()), time.Now).
		Assemble(rep, companyID)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	card, err := finance.NewStore(e.DB(), FinanceRates()).
		WithBillingContacts(financeBillingContacts{contacts: contacts.NewStore(e.DB())}).
		SummaryFor(rep, companyID)
	if err != nil {
		t.Fatalf("finance summary: %v", err)
	}

	if page.BillingContactsHidden == nil || page.BillingContactsHidden.Count != 1 ||
		len(page.BillingContactsHidden.Holders) != 1 || ids.UUID(page.BillingContactsHidden.Holders[0].UserId) != e.Rep3 {
		t.Fatalf("company page billing_contacts_hidden = %+v, want one entry held by Rep3", page.BillingContactsHidden)
	}
	if card.BillingContactsHidden == nil || !reflect.DeepEqual(*card.BillingContactsHidden, *page.BillingContactsHidden) {
		t.Errorf("finance card billing_contacts_hidden = %+v, company page %+v; one read must give one answer",
			card.BillingContactsHidden, *page.BillingContactsHidden)
	}
}
