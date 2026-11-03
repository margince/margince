// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// A Unicode domain and its punycode spelling are one domain to every company
// and duplicate matcher. The writers store the folded domain beside each
// address, and the matchers compare it, since Postgres cannot fold IDNA.

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestTheSweepAttachesAContactWhoseAddressSpellsTheDomainOtherwise(t *testing.T) {
	// A company domain is stored as a host name, so it is punycode. The
	// address is what may arrive in Unicode.
	for _, tc := range []struct{ name, address, companyDomain string }{
		{"a Unicode address at a punycode company domain", "anna@bücher.example", "xn--bcher-kva.example"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := setupDedupe(t)
			ctx := e.as()
			contact, err := e.store.EnsureCounterparty(ctx, e.ensureInput(ctx, t, tc.address, "Anna Brandt", "xn--bcher-kva.example"))
			if err != nil {
				t.Fatalf("ensure: %v", err)
			}
			company, err := e.store.CreateCompany(ctx, CreateCompanyInput{
				DisplayName: "Bücher GmbH",
				Domains:     []CompanyDomainInput{{Domain: tc.companyDomain, IsPrimary: true}},
			})
			if err != nil {
				t.Fatalf("creating the company: %v", err)
			}

			owed, err := e.store.DomainsOwedTheirContacts(ctx, 50)
			if err != nil {
				t.Fatal(err)
			}
			offered := false
			for _, d := range owed {
				if d.CompanyID.UUID == ids.UUID(company.Id) {
					offered = true
					if _, err := e.store.AttachDomainBacklog(ctx, d); err != nil {
						t.Fatalf("attaching the backlog: %v", err)
					}
				}
			}
			if !offered {
				t.Fatalf("the sweep does not offer the company at %s, whose contact writes from %s", tc.companyDomain, tc.address)
			}
			if employer := e.employerOf(ctx, t, contact.ContactID); employer == nil || *employer != ids.UUID(company.Id) {
				t.Errorf("the contact works at %v after the sweep, want %s", employer, company.Id)
			}
		})
	}
}

// Two leads at differently named companies share only a mail domain, spelled
// two ways. That is the employer term, and it puts the pair on the queue.
func TestTwoLeadsAtOneDomainInTwoSpellingsAreProposed(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	e.createLead(ctx, t, "Jonas Petersen", "jonas@bücher.example", "Bücher")
	e.createLead(ctx, t, "Jonas Peterson", "j.peterson@xn--bcher-kva.example", "Buecher Verlag")

	if rows := openCandidates(ctx, t, e, entityLead); len(rows) != 1 {
		t.Fatalf("open lead queue holds %d candidates, want the pair the shared domain names", len(rows))
	}
}
