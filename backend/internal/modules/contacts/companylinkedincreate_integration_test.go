// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestACompanyCreatedWithALinkedInURLStoresItsCanonicalSpelling(t *testing.T) {
	e := setupDedupe(t)
	raw := "http://www.linkedin.com/company/voltaq/about/?trk=x"
	company, err := e.store.CreateCompany(e.as(), CreateCompanyInput{
		DisplayName: "Voltaq GmbH", Source: "manual", LinkedInURL: &raw,
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := "https://www.linkedin.com/company/voltaq"; company.LinkedinUrl == nil || *company.LinkedinUrl != want {
		t.Fatalf("linkedin_url = %v, want %q", company.LinkedinUrl, want)
	}
}

func TestASecondCompanyWithTheSameLinkedInURLIsRefusedAsTheUpdatePathRefusesIt(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	first := "https://www.linkedin.com/company/twinco"
	if _, err := e.store.CreateCompany(ctx, CreateCompanyInput{
		DisplayName: "Twinco AG", Source: "manual", LinkedInURL: &first,
	}); err != nil {
		t.Fatal(err)
	}
	respelled := "www.linkedin.com/company/TwinCo/"
	_, createErr := e.store.CreateCompany(ctx, CreateCompanyInput{
		DisplayName: "Twinco Holding", Source: "manual", LinkedInURL: &respelled,
	})
	if _, typed := errors.AsType[*DuplicateCompanyLinkedInError](createErr); !typed || !errors.Is(createErr, apperrors.ErrConflict) {
		t.Fatalf("second create err = %v, want the typed LinkedIn conflict", createErr)
	}

	other, err := e.store.CreateCompany(ctx, CreateCompanyInput{DisplayName: "Twinco Holding", Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	_, updateErr := e.store.UpdateCompany(ctx, ids.From[ids.CompanyKind](ids.UUID(other.Id)), UpdateCompanyInput{LinkedInURL: &respelled})
	if _, typed := errors.AsType[*DuplicateCompanyLinkedInError](updateErr); !typed {
		t.Fatalf("update err = %v, want the same typed LinkedIn conflict as the create", updateErr)
	}
}

func TestACompanyCreateRefusesAProfileURLWhereACompanyURLBelongs(t *testing.T) {
	e := setupDedupe(t)
	profile := "https://www.linkedin.com/in/ada"
	_, err := e.store.CreateCompany(e.as(), CreateCompanyInput{
		DisplayName: "Profile Co", Source: "manual", LinkedInURL: &profile,
	})
	if err == nil {
		t.Fatal("a /in/ URL was accepted as a company's LinkedIn URL")
	}
}
