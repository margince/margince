// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func (e *dedupeEnv) qualifyTypedLead(ctx context.Context, t *testing.T, email *string, company *string, createCompany bool) PromoteOutcome {
	t.Helper()
	lead, _, err := e.store.CreateLead(ctx, CreateLeadInput{FullName: new("Dana Buyer"), Email: email, CompanyName: company, Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	id := ids.From[ids.LeadKind](ids.UUID(lead.Id))
	preview, err := e.store.PreviewLeadPromotion(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	in := PromoteLeadInput{Trigger: string(TriggerHumanQualify), CreateCompany: createCompany}
	plan, err := e.store.PreviewLeadContactChange(ctx, id, in)
	if err != nil {
		t.Fatal(err)
	}
	out, err := e.store.QualifyLead(ctx, id, in)
	if err != nil {
		t.Fatal(err)
	}
	proposed := proposedCompanyFact(ctx, t, e, ids.From[ids.ContactKind](ids.UUID(out.Contact.Id)))
	previewed := ""
	if preview.TypedCompany != nil {
		previewed = *preview.TypedCompany
	}
	if !createCompany && previewed != proposed {
		t.Fatalf("preview offered %q, promotion proposed %q", previewed, proposed)
	}
	if plan.TypedCompany != proposed {
		t.Fatalf("approval preview proposes %q, promotion proposed %q", plan.TypedCompany, proposed)
	}
	return out
}

func proposedCompanyFact(ctx context.Context, t *testing.T, e *dedupeEnv, contact ids.ContactID) string {
	t.Helper()
	var value string
	err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT value FROM contact_profile_field
			WHERE contact_id = $1 AND field = 'company_name' AND source = $2`, contact, leadPromotionSource).Scan(&value)
	})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal(err)
	}
	return value
}

func TestQualifyingAPersonalAddressProposesTheTypedCompany(t *testing.T) {
	for name, email := range map[string]*string{"personal address": new("dana.buyer@gmail.com"), "no address": nil} {
		t.Run(name, func(t *testing.T) {
			e := setupDedupe(t)
			ctx := e.as()
			out := e.qualifyTypedLead(ctx, t, email, new("Harbor Analytics"), false)
			if out.Contact.Employer != nil {
				t.Fatalf("employer from typed text alone: %+v", out.Contact.Employer)
			}
			if countCompaniesNamed(ctx, t, e, "Harbor Analytics") != 0 {
				t.Fatal("a company was created from typed text alone")
			}
			if got := proposedCompanyFact(ctx, t, e, ids.From[ids.ContactKind](ids.UUID(out.Contact.Id))); got != "Harbor Analytics" {
				t.Fatalf("proposed company %q, want the typed name", got)
			}
		})
	}
}

func TestQualifyingAPersonalAddressWithCreateCompanyLinksTheTypedCompany(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	out := e.qualifyTypedLead(ctx, t, new("dana.buyer@gmail.com"), new("Harbor Analytics"), true)
	if out.Contact.Employer == nil || out.Contact.Employer.CompanyName != "Harbor Analytics" {
		t.Fatalf("employer: %+v", out.Contact.Employer)
	}
	if got := proposedCompanyFact(ctx, t, e, ids.From[ids.ContactKind](ids.UUID(out.Contact.Id))); got != "" {
		t.Fatalf("proposed %q beside the employer it created", got)
	}
}

func TestQualifyingABusinessAddressLinksTheCompanyHoldingItsDomain(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	held := e.createNamedCompany(ctx, t, "Harbor Holdings", "harbor-analytics.test")
	out := e.qualifyTypedLead(ctx, t, new("dana@harbor-analytics.test"), new("Harbor Analytics"), false)
	if out.Contact.Employer == nil || ids.UUID(out.Contact.Employer.CompanyId) != held.UUID {
		t.Fatalf("employer: %+v, want the company holding the domain", out.Contact.Employer)
	}
	if countCompaniesNamed(ctx, t, e, "Harbor Analytics") != 0 {
		t.Fatal("a second company was created beside the domain's")
	}
}

func TestQualifyingABusinessAddressNamesTheNewCompanyAsTyped(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	out := e.qualifyTypedLead(ctx, t, new("dana@harbor-analytics.test"), new("Harbor Analytics"), false)
	if out.Contact.Employer == nil || out.Contact.Employer.CompanyName != "Harbor Analytics" {
		t.Fatalf("employer: %+v", out.Contact.Employer)
	}
	if got := proposedCompanyFact(ctx, t, e, ids.From[ids.ContactKind](ids.UUID(out.Contact.Id))); got != "" {
		t.Fatalf("proposed %q beside a resolved employer", got)
	}
}

func TestQualifyingWithoutATypedCompanyProposesNothing(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	out := e.qualifyTypedLead(ctx, t, new("dana@harbor-analytics.test"), nil, false)
	if got := proposedCompanyFact(ctx, t, e, ids.From[ids.ContactKind](ids.UUID(out.Contact.Id))); got != "" {
		t.Fatalf("proposed %q with no typed name", got)
	}
	status, _, _, _, _ := e.dispositionRow(ctx, t, "harbor-analytics.test")
	if status != DomainPending {
		t.Fatalf("domain status %s, want the durable triage capture opens", status)
	}
}
