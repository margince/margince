// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Whoever may edit a company may run its partner program. The margin tier
// prices every later commission, so setting or clearing it also takes the
// partner grant. The web form and the assistant's set-up share this one rule.

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// companyEditorPerms is the seeded rep's shape: it edits its own companies and
// reads the partner register without the partner write grant.
func companyEditorPerms() principal.Permissions {
	return principal.Permissions{
		Objects: map[string]principal.ObjectGrant{
			"partner": {Read: true}, "company": {Read: true, Update: true},
		},
		RowScope: principal.RowScopeOwn,
	}
}

func TestACompanyEditorSetsUpAPartnerProgramWithoutATier(t *testing.T) {
	e := Setup(t)
	company := ids.From[ids.CompanyKind](e.SeedCompany(t, "Harbour Hosting", &e.Rep1))
	owner := e.As(e.Rep1, []ids.UUID{e.Team1}, companyEditorPerms())
	step := "send the agreement"

	if err := e.Contacts.CheckPartnerSetUp(owner, company); err != nil {
		t.Fatalf("the set-up check refused the company's own editor: %v", err)
	}
	written, err := e.Contacts.UpsertPartner(owner, contacts.UpsertPartnerInput{
		CompanyID: company, PartnerRole: "hosting", NextStep: &step,
	})
	if err != nil {
		t.Fatalf("the company's editor setting up its partner program: %v", err)
	}
	if written.MarginTier != nil {
		t.Errorf("margin_tier = %v, want none set", *written.MarginTier)
	}

	edited := "call the partner manager"
	if _, err := e.Contacts.UpsertPartner(owner, contacts.UpsertPartnerInput{
		CompanyID: company, PartnerRole: "consulting", NextStep: &edited,
	}); err != nil {
		t.Fatalf("the company's editor changing the program's terms: %v", err)
	}
	after, err := e.Contacts.GetPartner(e.As(e.AdminUser, nil, partnerReadingPerms()), company)
	if err != nil {
		t.Fatalf("reading the partner back: %v", err)
	}
	if after.NextStep == nil || *after.NextStep != edited {
		t.Errorf("next_step = %v, want %q", after.NextStep, edited)
	}
}

func TestACompanyEditorCannotSetOrClearTheMarginTier(t *testing.T) {
	e := Setup(t)
	owner := e.As(e.Rep1, []ids.UUID{e.Team1}, companyEditorPerms())
	tier := "tier2_20"

	fresh := ids.From[ids.CompanyKind](e.SeedCompany(t, "Harbour Hosting", &e.Rep1))
	_, err := e.Contacts.UpsertPartner(owner, contacts.UpsertPartnerInput{
		CompanyID: fresh, PartnerRole: "hosting", MarginTier: &tier,
	})
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("a set-up naming a tier without the partner grant: err = %v, want permission denied", err)
	}
	if _, err := e.Contacts.GetPartner(e.As(e.AdminUser, nil, partnerReadingPerms()), fresh); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("the refused set-up left a partner program behind: err = %v", err)
	}

	tiered := ids.From[ids.CompanyKind](e.SeedPartnerCompany(t, "Northgate Partners", &tier, &e.Rep1))
	_, err = e.Contacts.UpsertPartner(owner, contacts.UpsertPartnerInput{
		CompanyID: tiered, PartnerRole: "consulting", Cleared: []string{"margin_tier"},
	})
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("clearing the tier without the partner grant: err = %v, want permission denied", err)
	}
	after, err := e.Contacts.GetPartner(e.As(e.AdminUser, nil, partnerReadingPerms()), tiered)
	if err != nil {
		t.Fatalf("reading the partner back: %v", err)
	}
	if after.MarginTier == nil || string(*after.MarginTier) != tier {
		t.Errorf("margin_tier = %v, want %q kept", after.MarginTier, tier)
	}
}

func TestThePartnerGrantSetsTheTier(t *testing.T) {
	e := Setup(t)
	company := ids.From[ids.CompanyKind](e.SeedCompany(t, "Harbour Hosting", &e.Rep1))
	tier := "tier3_25"

	written, err := e.Contacts.UpsertPartner(e.As(e.AdminUser, nil, partnerWritingPerms()), contacts.UpsertPartnerInput{
		CompanyID: company, PartnerRole: "hosting", MarginTier: &tier,
	})
	if err != nil {
		t.Fatalf("setting the tier with the partner grant: %v", err)
	}
	if written.MarginTier == nil || string(*written.MarginTier) != tier {
		t.Errorf("margin_tier = %v, want %q", written.MarginTier, tier)
	}
}

func TestThePartnerGrantAloneCannotSetUpAProgram(t *testing.T) {
	e := Setup(t)
	company := ids.From[ids.CompanyKind](e.SeedCompany(t, "Harbour Hosting", &e.Rep1))
	partnerOnly := e.As(e.Rep1, []ids.UUID{e.Team1}, principal.Permissions{
		Objects: map[string]principal.ObjectGrant{
			"partner": {Read: true, Update: true}, "company": {Read: true},
		},
		RowScope: principal.RowScopeOwn,
	})

	if err := e.Contacts.CheckPartnerSetUp(partnerOnly, company); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("the set-up check without the company grant: err = %v, want permission denied", err)
	}
	_, err := e.Contacts.UpsertPartner(partnerOnly, contacts.UpsertPartnerInput{CompanyID: company, PartnerRole: "hosting"})
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a set-up without the company grant: err = %v, want permission denied", err)
	}
}
