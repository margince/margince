// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type privateLeadMerge struct {
	env     *privacyEnv
	owner   context.Context
	company ids.CompanyID
	source  ids.LeadID
	target  ids.LeadID
}

func newPrivateLeadMerge(t *testing.T) privateLeadMerge {
	t.Helper()
	e := setupCapturePrivacy(t)
	owner := e.linker(e.owner, principal.RowScopeTeam)
	company := e.linkCompany(owner, t, "Private Account")
	if _, err := e.store.UpdateCompany(owner, company, UpdateCompanyInput{
		Domains: &[]CompanyDomainInput{{Domain: "kept.example", IsPrimary: true}},
	}); err != nil {
		t.Fatal(err)
	}
	target := e.linkLead(owner, t, e.owner, "target@kept.example", "Private Account")
	source := e.linkLead(owner, t, e.owner, "source@gmail.com", "Private Account")
	if _, err := e.store.LinkLeadCompany(owner, source, company); err != nil {
		t.Fatal(err)
	}
	return privateLeadMerge{env: e, owner: owner, company: company, source: source, target: target}
}

func (f privateLeadMerge) mergeAndCheck(t *testing.T, carried bool) {
	t.Helper()
	e, owner := f.env, f.owner
	if _, err := e.store.UpdateCompany(owner, f.company, UpdateCompanyInput{Visibility: new("owner")}); err != nil {
		t.Fatal(err)
	}
	reader := e.linker(e.teammate, principal.RowScopeTeam)
	out, err := e.store.MergeLead(reader, f.source, f.target)
	if err != nil {
		t.Fatal(err)
	}
	if out.CompanyId != nil {
		t.Fatal("merge disclosed the unreadable company")
	}
	if err := e.store.tx(owner, func(tx pgx.Tx) error {
		linked, err := leadLinkedCompany(owner, tx, f.target)
		if err == nil && (linked == nil || linked.UUID != f.company.UUID) {
			t.Errorf("survivor company=%v, want %s", linked, f.company)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	changed, present := e.mergeAudit(owner, t, f.target)["company_id"]
	if !carried && present {
		t.Fatalf("merge invented a company change in its audit: %v", changed)
	}
	if carried && changed != f.company.String() {
		t.Fatalf("carried company missing from audit: %v", changed)
	}
}

func TestLeadMergeCarriesAHiddenSourceCompanyLink(t *testing.T) {
	f := newPrivateLeadMerge(t)
	f.mergeAndCheck(t, true)
}

func TestLeadMergeKeepsAHiddenTargetCompanyLink(t *testing.T) {
	f := newPrivateLeadMerge(t)
	e, owner := f.env, f.owner
	if _, err := e.store.LinkLeadCompany(owner, f.target, f.company); err != nil {
		t.Fatal(err)
	}
	other := e.linkCompany(owner, t, "Source Account")
	if _, err := e.store.UpdateLead(owner, f.source, UpdateLeadInput{CompanyName: new("Source Account")}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.store.LinkLeadCompany(owner, f.source, other); err != nil {
		t.Fatal(err)
	}
	updated, err := e.store.UpdateLead(owner, f.target, UpdateLeadInput{
		CompanyName: new("Source Account"), CandidateCompanyKey: new("kept.example"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.CompanyId == nil || updated.CompanyId.String() != f.company.String() {
		t.Fatal("the target must retain its linked company before the merge")
	}
	f.mergeAndCheck(t, false)
}
