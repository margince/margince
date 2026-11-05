// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// A site read that names a company on the sender's own domain. Which of the
// two owns the domain, and who decides when a machine cannot.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/clocktest"
)

// resolveStatedName answers a domain with the company name its site stated.
func (e *dedupeEnv) resolveStatedName(ctx context.Context, t *testing.T, domain, stated string) ResolveDomainTriageResult {
	t.Helper()
	res, err := e.store.ResolveDomainTriage(ctx, ResolveDomainTriageInput{
		Domain: domain, Status: DomainCompany, Source: DomainSourceSiteRead,
		Evidence: "the site states a name", ReadID: e.startTriageRead(ctx, t, domain),
		DossierName: stated, SeedURL: "https://" + domain,
	})
	if err != nil {
		t.Fatalf("resolve %s: %v", domain, err)
	}
	return res
}

// assertHeldForAHuman checks that a domain carries no company and still waits
// on the question a colleague answers.
func (e *dedupeEnv) assertHeldForAHuman(ctx context.Context, t *testing.T, domain string, res ResolveDomainTriageResult) {
	t.Helper()
	if res.CompanyID != nil || res.CompanyCreated {
		t.Fatalf("resolve = %+v, want no company", res)
	}
	if n := e.countCompaniesOn(ctx, t, domain); n != 0 {
		t.Fatalf("%d companies carry %s, want 0", n, domain)
	}
	status, reason, _, _, companyID := e.dispositionRow(ctx, t, domain)
	if status != DomainPending || reason != PendingUnevidenced || companyID != nil {
		t.Fatalf("disposition = %q / %q / %v, want pending for a human with no company", status, reason, companyID)
	}
}

// A personal page that names its owner's employer reads like a founder's brand.
// Neither files the domain on the company, and a later mail maps to none.
func TestASiteNamingTheSendersEmployerIsHeldForAHuman(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	e.openTriageFirst(ctx, t, "jonas@lindqvist.example", "Jonas Lindqvist", "lindqvist.example")
	e.assertHeldForAHuman(ctx, t, "lindqvist.example", e.resolveStatedName(ctx, t, "lindqvist.example", "Acme GmbH"))

	later, err := e.store.EnsureCounterparty(ctx, e.ensureInput(ctx, t, "jonas@lindqvist.example", "Jonas Lindqvist", "lindqvist.example"))
	if err != nil {
		t.Fatalf("ensure after the verdict: %v", err)
	}
	if later.CompanyID != nil {
		t.Fatalf("a later mail = %+v, want no company", later)
	}
}

// A founder's own company under a brand unrelated to the founder-name domain.
// A machine cannot tell it from an employer, so it is held too.
func TestAFoundersBrandOnTheirNameDomainIsHeldForAHuman(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	e.openTriageFirst(ctx, t, "john@johnsmith.example", "John Smith", "johnsmith.example")
	e.assertHeldForAHuman(ctx, t, "johnsmith.example", e.resolveStatedName(ctx, t, "johnsmith.example", "Blue Oak Advisory"))
}

// A site that names its own domain is still the company. That holds even when
// the domain is also its founder's surname, because a read site outranks a name.
func TestASiteNamingItsOwnDomainStaysTheCompanyOfItsFounder(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	e.openTriageFirst(ctx, t, "alexander@baumert.example", "Alexander Baumert", "baumert.example")
	res := e.resolveStatedName(ctx, t, "baumert.example", "Baumert Agentur GmbH")
	if !res.CompanyCreated {
		t.Fatalf("resolve = %+v, want the agency created", res)
	}
	if n := e.countCompaniesOn(ctx, t, "baumert.example"); n != 1 {
		t.Fatalf("%d companies on the agency's domain, want 1", n)
	}
}

// Old mail goes to a human before any automatic verdict settles the domain,
// a refusal as much as a company.
func TestStaleEvidenceKeepsAnAutomaticPersonalVerdictOpen(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	e.openTriageFirst(ctx, t, "greta@vogel.example", "Greta Vogel", "vogel.example")
	old := clocktest.Now(t).AddDate(-(staleEvidenceYears + 2), 0, 0)
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE company_domain_disposition SET last_evidence_at = $2 WHERE domain = $1`, "vogel.example", old)
		return err
	}); err != nil {
		t.Fatalf("ageing the evidence: %v", err)
	}
	if _, err := e.store.ResolveDomainTriage(ctx, ResolveDomainTriageInput{
		Domain: "vogel.example", Status: DomainPersonal, Source: DomainSourceSiteRead,
		Evidence: "the site reads as a personal page", ReadID: e.startTriageRead(ctx, t, "vogel.example"),
	}); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	status, reason, _, _, _ := e.dispositionRow(ctx, t, "vogel.example")
	if status != DomainPending || reason != PendingStaleEvidence {
		t.Fatalf("disposition = %q / %q, want pending for stale evidence", status, reason)
	}
}
