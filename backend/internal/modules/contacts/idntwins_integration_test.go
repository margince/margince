// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/owndomain"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// addFoldedLegacyDomain writes a row the way an older release stored a
// Unicode domain: the raw spelling, with its fold beside it.
func (e *dedupeEnv) addFoldedLegacyDomain(ctx context.Context, t *testing.T, company ids.CompanyID, domain string) {
	t.Helper()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO company_domain (company_id, domain, is_primary, source, captured_by, domain_folded)
			VALUES ($1, $2, false, 'manual', 'human:test', $3)`, company, domain, owndomain.StoredOwn(domain))
		return err
	}); err != nil {
		t.Fatalf("adding legacy domain %s: %v", domain, err)
	}
}

// twinCompanies makes two companies holding one domain in its two spellings.
func (e *dedupeEnv) twinCompanies(ctx context.Context, t *testing.T) (ids.CompanyID, ids.CompanyID) {
	t.Helper()
	first := e.createNamedCompany(ctx, t, "Bücher Verlag", "xn--bcher-kva.example")
	second := e.createNamedCompany(ctx, t, "Buecher Handel", "buecher-handel.example")
	e.addFoldedLegacyDomain(ctx, t, second, "bücher.example")
	return first, second
}

func TestAnAddressAtTwinSpellingsOfOneDomainAnswersNoCompany(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	e.twinCompanies(ctx, t)
	for _, email := range []string{"anna@bücher.example", "anna@xn--bcher-kva.example"} {
		if got, found, err := e.store.CompanyForAddress(ctx, email); err != nil || found {
			t.Errorf("%s answered %+v found=%v err=%v, want no company while two hold the domain", email, got, found, err)
		}
	}
}

func TestTheSweepSkipsADomainTwoCompaniesHold(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	first, second := e.twinCompanies(ctx, t)
	if _, err := e.store.EnsureCounterparty(ctx, e.ensureInput(ctx, t, "anna@bücher.example", "Anna Brandt", "xn--bcher-kva.example")); err != nil {
		t.Fatal(err)
	}
	owed, err := e.store.DomainsOwedTheirContacts(ctx, 50)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range owed {
		if d.CompanyID == first || d.CompanyID == second {
			t.Errorf("the sweep offers %s on %s, which two companies hold", d.CompanyID, d.Domain)
		}
	}
}

func TestALeadKeyNamesNoCompanyWhileTwoHoldItsDomain(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	first, second := e.twinCompanies(ctx, t)
	key := "bücher.example"
	name := "Anna"
	lead, _, err := e.store.CreateLead(ctx, CreateLeadInput{FullName: &name, CandidateCompanyKey: &key, Source: "inbound"})
	if err != nil {
		t.Fatal(err)
	}
	if lead.CandidateCompanyKey == nil || *lead.CandidateCompanyKey != "xn--bcher-kva.example" {
		t.Fatalf("the key was stored as %v, want the punycode form", lead.CandidateCompanyKey)
	}
	if lead.CompanyMatch != nil {
		t.Errorf("the lead was offered %v while two companies hold the domain", lead.CompanyMatch)
	}
	e.archiveAllDomains(ctx, t, second)
	single, err := e.store.GetLead(ctx, ids.From[ids.LeadKind](ids.UUID(lead.Id)), storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if single.CompanyMatch == nil || ids.UUID(single.CompanyMatch.Id) != first.UUID {
		t.Errorf("with one holder left the offer is %v, want %s", single.CompanyMatch, first)
	}
}

func (e *dedupeEnv) archiveAllDomains(ctx context.Context, t *testing.T, company ids.CompanyID) {
	t.Helper()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE company_domain SET archived_at = now() WHERE company_id = $1`, company)
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func TestAnEditToTheOtherSpellingOfAKeyStillNamesTheLinkedCompany(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	company := e.createNamedCompany(ctx, t, "Bücher Verlag", "buecher-verlag.example")
	e.addFoldedLegacyDomain(ctx, t, company, "bücher.example")
	for _, key := range []string{"bücher.example", "xn--bcher-kva.example"} {
		var named bool
		if err := e.store.tx(ctx, func(tx pgx.Tx) error {
			var err error
			named, err = namesCompany(ctx, tx, company, nil, &key)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		if !named {
			t.Errorf("key %q does not name the company holding the legacy Unicode row", key)
		}
	}
}

func TestALeadKeyIsStoredFoldedOnCreateAndUpdate(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	name := "Anna"
	typed := "https://www.Bücher.example"
	lead, _, err := e.store.CreateLead(ctx, CreateLeadInput{FullName: &name, CandidateCompanyKey: &typed, Source: "inbound"})
	if err != nil || lead.CandidateCompanyKey == nil || *lead.CandidateCompanyKey != "xn--bcher-kva.example" {
		t.Fatalf("created key = %v (%v), want the punycode host", lead.CandidateCompanyKey, err)
	}
	slug := "Acme Inc"
	updated, err := e.store.UpdateLead(ctx, ids.From[ids.LeadKind](ids.UUID(lead.Id)), UpdateLeadInput{CandidateCompanyKey: &slug})
	if err != nil || updated.CandidateCompanyKey == nil || *updated.CandidateCompanyKey != "Acme Inc" {
		t.Fatalf("a slug key = %v (%v), want it as typed", updated.CandidateCompanyKey, err)
	}
	other := "München.example"
	updated, err = e.store.UpdateLead(ctx, ids.From[ids.LeadKind](ids.UUID(lead.Id)), UpdateLeadInput{CandidateCompanyKey: &other})
	if err != nil || updated.CandidateCompanyKey == nil || *updated.CandidateCompanyKey != "xn--mnchen-3ya.example" {
		t.Fatalf("an updated key = %v (%v), want the punycode host", updated.CandidateCompanyKey, err)
	}
}
