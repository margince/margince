// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

func TestAContactCreatedWithACompanyHasACurrentEmployment(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	in, err := ContactCreationInput(crmcontracts.CreateContactRequest{
		FullName: "Nora Newhire", Source: "manual", CompanyName: new("Newhire Works GmbH"),
	})
	if err != nil {
		t.Fatalf("mapping the request: %v", err)
	}
	created, err := e.store.CreateContact(ctx, in)
	if err != nil {
		t.Fatalf("creating a contact with a company: %v", err)
	}
	if created.Employer == nil || created.Employer.CompanyName != "Newhire Works GmbH" {
		t.Fatalf("employer on the answer = %+v, want the company the request named", created.Employer)
	}
	contactID := ids.From[ids.ContactKind](ids.UUID(created.Id))
	assertEmployedAt(ctx, t, e, contactID, ids.From[ids.CompanyKind](ids.UUID(created.Employer.CompanyId)))

	existing := e.createNamedCompany(ctx, t, "Picked Employer AG")
	in, err = ContactCreationInput(crmcontracts.CreateContactRequest{
		FullName: "Paul Picked", Source: "manual", CompanyId: new(openapiUUID(existing)),
	})
	if err != nil {
		t.Fatalf("mapping the request: %v", err)
	}
	picked, err := e.store.CreateContact(ctx, in)
	if err != nil {
		t.Fatalf("creating a contact at an existing company: %v", err)
	}
	assertEmployedAt(ctx, t, e, ids.From[ids.ContactKind](ids.UUID(picked.Id)), existing)
}

// The edge is refused after the contact is written, so the refusal must take
// the contact with it. A missing company and an unreadable one answer alike.
func TestACreateWhoseCompanyCannotBeReadLeavesNoContact(t *testing.T) {
	e := setupDedupe(t)
	hidden := e.createNamedCompany(e.asOther(), t, "Hidden Employer GmbH")
	if _, err := e.store.UpdateCompany(e.asOther(), hidden, UpdateCompanyInput{Visibility: new("owner")}); err != nil {
		t.Fatalf("making the company private: %v", err)
	}

	for name, companyID := range map[string]ids.UUID{"hidden": hidden.UUID, "missing": ids.NewV7()} {
		t.Run(name, func(t *testing.T) {
			ctx := e.as()
			fullName := "Refused " + name
			in, err := ContactCreationInput(crmcontracts.CreateContactRequest{
				FullName: fullName, Source: "manual", CompanyId: new(openapi_types.UUID(companyID)),
			})
			if err != nil {
				t.Fatalf("mapping the request: %v", err)
			}
			_, err = e.store.CreateContact(ctx, in)
			refused, ok := errors.AsType[*auth.ReferenceNotFoundError](err)
			if !ok || refused.Field != endCompanyColumn {
				t.Fatalf("err = %v, want a refusal naming company_id", err)
			}
			if got := countContactsNamed(ctx, t, e, fullName); got != 0 {
				t.Errorf("%d contact(s) survived a refused employer, want 0", got)
			}
		})
	}
}

func TestCreateAndLinkGivesTheNewCompanyTheWorkDomain(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	contactID := e.contactWithEmail(ctx, t, "Dana Domain", "dana@linkco.test")

	linked, err := e.store.LinkContactEmployer(ctx, contactID, LinkEmployerInput{
		CompanyName: new("Linkco"), Title: new("Head of Procurement"),
	})
	if err != nil {
		t.Fatalf("create and link: %v", err)
	}
	if !linked.CompanyCreated || linked.CompanyDomain == nil || *linked.CompanyDomain != "linkco.test" {
		t.Fatalf("created = %v, domain = %v, want a new company on linkco.test", linked.CompanyCreated, linked.CompanyDomain)
	}
	if got := e.domainsOf(ctx, t, linked.CompanyID); len(got) != 1 || got[0] != "linkco.test" {
		t.Errorf("company domains = %v, want [linkco.test]", got)
	}
	assertEmployedAt(ctx, t, e, contactID, linked.CompanyID)
	if linked.Contact.Employer == nil || linked.Contact.Employer.CompanyId != openapiUUID(linked.CompanyID) {
		t.Errorf("the answer's contact employer = %+v, want the new company", linked.Contact.Employer)
	}
}

func TestCreateAndLinkFromConsumerMailGivesNoDomain(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	contactID := e.contactWithEmail(ctx, t, "Gina Gmail", "gina.gmail@gmail.com")

	linked, err := e.store.LinkContactEmployer(ctx, contactID, LinkEmployerInput{CompanyName: new("Gina Consulting")})
	if err != nil {
		t.Fatalf("create and link: %v", err)
	}
	if linked.CompanyDomain != nil {
		t.Errorf("domain = %q, want none for a consumer mailbox", *linked.CompanyDomain)
	}
	if got := e.domainsOf(ctx, t, linked.CompanyID); len(got) != 0 {
		t.Errorf("company domains = %v, want none", got)
	}
	assertEmployedAt(ctx, t, e, contactID, linked.CompanyID)
}

func TestCreateAndLinkOnAHeldDomainNamesTheHolderAndWritesNothing(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	holder := e.createNamedCompany(ctx, t, "Heldco", "heldco.test")
	contactID := e.contactWithEmail(ctx, t, "Sam Held", "sam@heldco.test")

	_, err := e.store.LinkContactEmployer(ctx, contactID, LinkEmployerInput{CompanyName: new("Heldco Two")})
	dup, ok := errors.AsType[*DuplicateDomainError](err)
	if !ok || dup.ExistingID != holder {
		t.Fatalf("err = %v, want a duplicate-domain refusal naming %s", err, holder)
	}
	if got := countCompaniesNamed(ctx, t, e, "Heldco Two"); got != 0 {
		t.Errorf("%d near-duplicate company(s) written, want 0", got)
	}
	if got := e.employmentsOf(ctx, t, contactID); got != 0 {
		t.Errorf("%d employment edge(s) written, want 0", got)
	}
}

func TestCreateAndLinkOnAContactTheCallerCannotSeeIsNotFound(t *testing.T) {
	e := setupDedupe(t)
	private := e.contactWithEmail(e.asOther(), t, "Priya Private", "priya@privateco.test")
	if _, err := e.store.UpdateContact(e.asOther(), private, UpdateContactInput{Visibility: new("owner")}); err != nil {
		t.Fatalf("making the contact private: %v", err)
	}

	_, err := e.store.LinkContactEmployer(e.as(), private, LinkEmployerInput{CompanyName: new("Privateco")})
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("err = %v, want not found", err)
	}
	if got := countCompaniesNamed(e.as(), t, e, "Privateco"); got != 0 {
		t.Errorf("%d company(s) written for an unseen contact, want 0", got)
	}
}

// A holder the caller cannot read is never named: the company is created
// without the domain, and the answer carries no hint of the holder.
func TestCreateAndLinkOnADomainAHiddenCompanyHoldsCreatesItWithoutTheDomain(t *testing.T) {
	e := setupDedupe(t)
	holder := e.createNamedCompany(e.asOther(), t, "Hiddenholder", "hiddenholder.test")
	if _, err := e.store.UpdateCompany(e.asOther(), holder, UpdateCompanyInput{Visibility: new("owner")}); err != nil {
		t.Fatalf("making the holder private: %v", err)
	}
	ctx := e.as()
	contactID := e.contactWithEmail(ctx, t, "Hal Hidden", "hal@hiddenholder.test")

	linked, err := e.store.LinkContactEmployer(ctx, contactID, LinkEmployerInput{CompanyName: new("Hiddenholder Two")})
	if err != nil {
		t.Fatalf("create and link: %v", err)
	}
	if !linked.CompanyCreated || linked.CompanyDomain != nil || linked.CompanyID == holder {
		t.Fatalf("created = %v, domain = %v, company = %v; want a new company with no domain", linked.CompanyCreated, linked.CompanyDomain, linked.CompanyID)
	}
	if got := e.domainsOf(ctx, t, linked.CompanyID); len(got) != 0 {
		t.Errorf("company domains = %v, want none", got)
	}
	assertEmployedAt(ctx, t, e, contactID, linked.CompanyID)
}

func TestCreateAndLinkOnASuppressedDomainGivesNoDomain(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	if _, err := e.store.SetDomainAdmission(ctx, "blockedvendor.test", DomainSuppressed, "a tool we use"); err != nil {
		t.Fatalf("suppressing the domain: %v", err)
	}
	contactID := e.contactWithEmail(ctx, t, "Bea Blocked", "bea@blockedvendor.test")

	linked, err := e.store.LinkContactEmployer(ctx, contactID, LinkEmployerInput{CompanyName: new("Blocked Vendor Ltd")})
	if err != nil {
		t.Fatalf("create and link: %v", err)
	}
	if linked.CompanyDomain != nil {
		t.Errorf("domain = %q, want none on a suppressed domain", *linked.CompanyDomain)
	}
	if got := e.domainsOf(ctx, t, linked.CompanyID); len(got) != 0 {
		t.Errorf("company domains = %v, want none", got)
	}
}

func TestAnAgentContactCreateWithACompanyWritesNothing(t *testing.T) {
	e := setupDedupe(t)
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:tools", UserID: e.rep, OnBehalfOf: e.rep,
		Permissions: principal.Permissions{
			Objects: map[string]principal.ObjectGrant{
				"contact": {Create: true, Read: true, Update: true}, "company": {Create: true, Read: true},
				"relationship": {Create: true, Read: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
	_, err := e.store.CreateContact(ctx, CreateContactInput{
		FullName: "Agent Made", Source: "agent", Employer: EmployerChoice{CompanyName: new("Agent Side Effect GmbH"), Source: "agent"},
	})
	if refused, ok := errors.AsType[*values.ParseError](err); !ok || refused.Code != "employer_needs_a_human" {
		t.Fatalf("err = %v, want employer_needs_a_human", err)
	}
	if contacts, companies := countContactsNamed(e.as(), t, e, "Agent Made"), countCompaniesNamed(e.as(), t, e, "Agent Side Effect GmbH"); contacts+companies != 0 {
		t.Errorf("%d contact(s) and %d company(s) written, want none", contacts, companies)
	}
}

func (e *dedupeEnv) contactWithEmail(ctx context.Context, t *testing.T, name, email string) ids.ContactID {
	t.Helper()
	created, err := e.store.CreateContact(ctx, CreateContactInput{
		FullName: name, Source: "manual",
		Emails: []ContactEmailInput{{Email: email, EmailType: emailTypeWork, IsPrimary: true}},
	})
	if err != nil {
		t.Fatalf("seeding contact %s: %v", name, err)
	}
	return ids.From[ids.ContactKind](ids.UUID(created.Id))
}

func (e *dedupeEnv) domainsOf(ctx context.Context, t *testing.T, companyID ids.CompanyID) []string {
	t.Helper()
	var domains []string
	err := e.store.tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT domain FROM company_domain WHERE company_id = $1 AND archived_at IS NULL ORDER BY domain`, companyID)
		if err != nil {
			return err
		}
		domains, err = pgx.CollectRows(rows, pgx.RowTo[string])
		return err
	})
	if err != nil {
		t.Fatalf("reading company domains: %v", err)
	}
	return domains
}

func (e *dedupeEnv) employmentsOf(ctx context.Context, t *testing.T, contactID ids.ContactID) int {
	t.Helper()
	var found int
	err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM relationship
			WHERE kind = 'employment' AND contact_id = $1 AND archived_at IS NULL`, contactID).Scan(&found)
	})
	if err != nil {
		t.Fatalf("counting employments: %v", err)
	}
	return found
}

func validationField(err error) (field, code string, details map[string]any) {
	refused, ok := errors.AsType[*httperr.DetailedError](err)
	if !ok || len(refused.Fields) == 0 {
		return "", "", nil
	}
	return refused.Fields[0].Field, refused.Fields[0].Code, refused.Details
}
