// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"fmt"
	"slices"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestQualifyingATypedEmployerCreatesOrReusesTheCompany(t *testing.T) {
	for _, existing := range []bool{false, true} {
		t.Run(fmt.Sprint(existing), func(t *testing.T) {
			e := setupDedupe(t)
			ctx := e.as()
			var company ids.CompanyID
			if existing {
				company = e.createNamedCompany(ctx, t, "Northwind Research")
			}
			lead, _, err := e.store.CreateLead(ctx, CreateLeadInput{FullName: new("Dana Buyer"), Email: new("dana@northwind-research.test"), CompanyName: new("Northwind Research"), Source: "manual"})
			if err != nil {
				t.Fatal(err)
			}
			out, err := e.store.QualifyLead(ctx, ids.From[ids.LeadKind](ids.UUID(lead.Id)), PromoteLeadInput{Trigger: string(TriggerHumanQualify)})
			if err != nil {
				t.Fatal(err)
			}
			if out.Contact.Employer == nil || out.Contact.Employer.CompanyName != "Northwind Research" {
				t.Fatalf("employer: %+v", out.Contact.Employer)
			}
			if existing && ids.UUID(out.Contact.Employer.CompanyId) != company.UUID {
				t.Fatal("did not reuse company")
			}
			if countCompaniesNamed(ctx, t, e, "Northwind Research") != 1 {
				t.Fatal("duplicate company")
			}
		})
	}
}

func TestManualWorkAddressResolvesWithoutAnotherMessage(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	id := e.contactWithEmail(ctx, t, "Dana Buyer", "dana@northwind-research.test")
	status, _, _, _, _ := e.dispositionRow(ctx, t, "northwind-research.test")
	if status != DomainPending {
		t.Fatalf("status %s, want durable pending", status)
	}
	resolved := e.resolveStatedName(ctx, t, "northwind-research.test", "Northwind Research")
	if resolved.CompanyID == nil {
		t.Fatal("triage did not resolve company")
	}
	assertEmployedAt(ctx, t, e, id, *resolved.CompanyID)
	second := e.contactWithEmail(ctx, t, "Robin Buyer", "robin@northwind-research.test")
	assertEmployedAt(ctx, t, e, second, *resolved.CompanyID)
}

func TestConcurrentContactsReuseOneNamedEmployerAndReplayEmployment(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := range 2 {
		wg.Go(func() {
			created, err := e.store.CreateContact(ctx, CreateContactInput{FullName: fmt.Sprintf("Buyer %d", i), Source: "manual", Employer: EmployerChoice{CompanyName: new("Shared Employer")}})
			if err == nil {
				err = e.store.ResolveContactEmployer(ctx, ids.From[ids.ContactKind](ids.UUID(created.Id)), EmployerChoice{CompanyName: new("Shared Employer"), Source: "manual"})
			}
			errs <- err
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if countCompaniesNamed(ctx, t, e, "Shared Employer") != 1 {
		t.Fatal("duplicate employer")
	}
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		var count int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM relationship WHERE kind = 'employment'`).Scan(&count); err != nil {
			return err
		}
		if count != 2 {
			t.Errorf("employment count %d, want two", count)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestConflictingEmployerNameAndDomainRollBackContact(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	e.createNamedCompany(ctx, t, "Named Employer")
	e.createNamedCompany(ctx, t, "Other Employer", "other-employer.test")
	_, err := e.store.CreateContact(ctx, CreateContactInput{FullName: "Conflicting Buyer", Source: "manual", Employer: EmployerChoice{CompanyName: new("Named Employer")}, Emails: []ContactEmailInput{{Email: "buyer@other-employer.test", EmailType: "work", IsPrimary: true}}})
	if field, code := refusalOf(err); field != fieldCompanyName || code != "conflict" {
		t.Fatalf("refusal: %v", err)
	}
	if countContactsNamed(ctx, t, e, "Conflicting Buyer") != 0 {
		t.Fatal("contact survived refused employer")
	}
}

func TestManualWorkAddressRecordsDomainSplitReviewOnce(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	e.seedCompanyOnDomain(ctx, t, "Parent Works", "parent-works.test")
	e.seedCompanyOnDomain(ctx, t, "Subsidiary Works", "sub.parent-works.test")
	id := e.contactWithEmail(ctx, t, "Dana Buyer", "dana@sub.parent-works.test")
	if err := e.store.ResolveContactEmployer(ctx, id, EmployerChoice{Source: "manual"}); err != nil {
		t.Fatal(err)
	}
	if candidates := openCandidates(ctx, t, e, entityCompany); len(candidates) != 1 {
		t.Fatalf("domain split reviews: %d, want one", len(candidates))
	}
}

func TestOwnerlessWorkAddressIsDueAndLinksAfterTriage(t *testing.T) {
	e := setupDedupe(t)
	ctx := principal.WithActor(e.as(), principal.Principal{Type: principal.PrincipalSystem, ID: "system:employer-test"})
	id := e.contactWithEmail(ctx, t, "Ownerless Buyer", "buyer@ownerless-work.test")
	due, err := e.store.ListDueDomains(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(due, func(d DueDomain) bool { return d.Domain == "ownerless-work.test" }) {
		t.Fatal("ownerless question is stranded")
	}
	result := e.resolveStatedName(ctx, t, "ownerless-work.test", "Ownerless Works")
	if result.CompanyID == nil {
		t.Fatal("no company")
	}
	assertEmployedAt(ctx, t, e, id, *result.CompanyID)
}

func TestStoredWorkAddressDoesNotRestoreAnArchivedEmployment(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	company := e.createNamedCompany(ctx, t, "Past Works", "past-work.test")
	id := e.contactWithEmail(ctx, t, "Past Buyer", "buyer@past-work.test")
	var edge ids.UUID
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT id FROM relationship WHERE contact_id=$1 AND company_id=$2`, id, company).Scan(&edge)
	}); err != nil {
		t.Fatal(err)
	}
	actor, ok := principal.Actor(ctx)
	if !ok {
		t.Fatal("missing actor")
	}
	grant := actor.Permissions.Objects["relationship"]
	grant.Delete = true
	actor.Permissions.Objects["relationship"] = grant
	ctx = principal.WithActor(ctx, actor)
	if _, err := e.store.ArchiveRelationship(ctx, edge, nil); err != nil {
		t.Fatal(err)
	}
	if err := e.store.ResolveContactEmployer(ctx, id, EmployerChoice{}); err != nil {
		t.Fatal(err)
	}
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		employer, err := currentEmployerID(ctx, tx, id)
		if employer != nil {
			t.Error("restored ended employer")
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func TestNamedEmployerStoresTheRegistrableAndMailDomains(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	result, err := e.store.CreateContact(ctx, CreateContactInput{FullName: "Domain Buyer", Source: "manual", Employer: EmployerChoice{CompanyName: new("Domain Works")}, Emails: []ContactEmailInput{{Email: "buyer@mail.domain-work.test", EmailType: "work", IsPrimary: true}}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Employer == nil {
		t.Fatal("missing employer")
	}
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		var primary string
		if err := tx.QueryRow(ctx, `SELECT domain FROM company_domain WHERE company_id=$1 AND is_primary AND archived_at IS NULL`, result.Employer.CompanyId).Scan(&primary); err != nil {
			return err
		}
		if primary != "domain-work.test" {
			t.Errorf("primary domain: %s", primary)
		}
		var count int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM company_domain WHERE company_id=$1 AND domain='mail.domain-work.test' AND archived_at IS NULL`, result.Employer.CompanyId).Scan(&count); err != nil {
			return err
		}
		if count != 1 {
			t.Error("mail host missing")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestMissingCompanyGrantRollsBackWorkContact(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	actor, ok := principal.Actor(ctx)
	if !ok {
		t.Fatal("missing actor")
	}
	grant := actor.Permissions.Objects["company"]
	grant.Create = false
	actor.Permissions.Objects["company"] = grant
	restricted := principal.WithActor(ctx, actor)
	_, err := e.store.CreateContact(restricted, CreateContactInput{FullName: "Refused Buyer", Source: "manual", Emails: []ContactEmailInput{{Email: "buyer@refused-work.test", EmailType: "work", IsPrimary: true}}})
	if err == nil {
		t.Fatal("company authority bypassed")
	}
	if countContactsNamed(ctx, t, e, "Refused Buyer") != 0 {
		t.Fatal("contact survived refused employer resolution")
	}
}
