// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Two names that score over the review bar and are not one brand. The scan
// subject is the short name, whose brand the trigram set finds inside the long one.
const (
	brandName = "Fabrikam GmbH"
	lineName  = "Fabrikam Cloud Services"
)

// companyAt creates a company through the manual create path in one city, at
// a street line when one is given.
func (e *dedupeEnv) companyAt(ctx context.Context, t *testing.T, name, street string) ids.CompanyID {
	t.Helper()
	address := &crmcontracts.Address{PostalCode: new("80331"), City: new("München")}
	if street != "" {
		address.Line1 = &street
	}
	created, err := e.store.CreateCompany(ctx, CreateCompanyInput{DisplayName: name, Source: "manual", Address: address})
	if err != nil {
		t.Fatalf("create %s: %v", name, err)
	}
	return ids.From[ids.CompanyKind](ids.UUID(created.Id))
}

func (e *dedupeEnv) statePhone(ctx context.Context, t *testing.T, company ids.CompanyID, phone string) {
	t.Helper()
	if _, err := e.store.CreateCompanyFact(ctx, company, FactCreateInput{
		Category: factCategoryCompany, Field: FactPhone, Value: phone,
	}); err != nil {
		t.Fatalf("state phone %s: %v", phone, err)
	}
}

func TestASimilarCompanyNameFilesOnlyWithASharedFact(t *testing.T) {
	if score := bestCompanyNamePairing(brandName, "", lineName, ""); score.Confidence < dedupeReviewThreshold || score.SameBrand {
		t.Fatalf("the pair scores %.4f (same brand %v); this test needs a similar name that is not one brand",
			score.Confidence, score.SameBrand)
	}
	cases := map[string]struct {
		share func(e *dedupeEnv, ctx context.Context, t *testing.T, brand, line ids.CompanyID)
		filed bool
	}{
		"nothing shared": {share: func(*dedupeEnv, context.Context, *testing.T, ids.CompanyID, ids.CompanyID) {}},
		"one phone number, written two ways": {
			filed: true,
			share: func(e *dedupeEnv, ctx context.Context, t *testing.T, brand, line ids.CompanyID) {
				e.statePhone(ctx, t, brand, "+49 89 1234567")
				e.statePhone(ctx, t, line, "0049 89 123 45 67")
			},
		},
		"a contact employed at both": {
			filed: true,
			share: func(e *dedupeEnv, _ context.Context, t *testing.T, brand, line ids.CompanyID) {
				colleague := mustContact(t, e, "Greta Hoffmann")
				employ(t, e, colleague, brand)
				employ(t, e, colleague, line)
			},
		},
		"a contact who left one of them": {
			share: func(e *dedupeEnv, ctx context.Context, t *testing.T, brand, line ids.CompanyID) {
				colleague := mustContact(t, e, "Greta Hoffmann")
				employ(t, e, colleague, brand)
				left := clocktest.Now(t).AddDate(-1, 0, 0)
				if _, err := e.store.CreateRelationship(ctx, CreateRelationshipInput{
					Kind: employmentKind, ContactID: &colleague, CompanyID: &line, EndedAt: &left, Source: "manual",
				}); err != nil {
					t.Fatalf("record the past employment: %v", err)
				}
			},
		},
		"numbers that differ": {
			share: func(e *dedupeEnv, ctx context.Context, t *testing.T, brand, line ids.CompanyID) {
				e.statePhone(ctx, t, brand, "+49 89 1234567")
				e.statePhone(ctx, t, line, "+49 89 7654321")
			},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := setupDedupe(t)
			ctx := e.as()
			brand := e.createNamedCompany(ctx, t, brandName)
			line := e.createNamedCompany(ctx, t, lineName)
			tc.share(e, ctx, t, brand, line)
			e.forgetFiledPairs(ctx, t)
			if filed := e.scanOne(t, DedupeScanCompanies, brand.UUID); (filed == 1) != tc.filed {
				t.Errorf("the scan filed %d pairs, want filed=%v", filed, tc.filed)
			}
		})
	}
}

// One street address is a shared fact; one city or postal code is not.
func TestASimilarCompanyNameAtOneStreetAddressIsFiled(t *testing.T) {
	for name, tc := range map[string]struct {
		brandStreet, lineStreet string
		filed                   bool
	}{
		"one street address":   {"Marienplatz 8", "Marienplatz 8", true},
		"two street addresses": {"Marienplatz 8", "Sendlinger Straße 1", false},
		"one city, no street":  {"", "", false},
	} {
		t.Run(name, func(t *testing.T) {
			e := setupDedupe(t)
			ctx := e.as()
			brand := e.companyAt(ctx, t, brandName, tc.brandStreet)
			e.companyAt(ctx, t, lineName, tc.lineStreet)
			e.forgetFiledPairs(ctx, t)
			if got := e.scanOne(t, DedupeScanCompanies, brand.UUID); (got == 1) != tc.filed {
				t.Errorf("the scan filed %d pairs, want filed=%v", got, tc.filed)
			}
		})
	}
}

// One live domain belongs to one company, so a shared domain is a nested one,
// and the subdomain lane files it whatever the names say.
func TestASimilarCompanyNameOnANestedDomainIsFiled(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	e.createNamedCompany(ctx, t, brandName, "fabrikam.example")
	line := e.createNamedCompany(ctx, t, lineName, "cloud.fabrikam.example")
	e.forgetFiledPairs(ctx, t)
	if filed := e.scanOne(t, DedupeScanCompanies, line.UUID); filed != 1 {
		t.Errorf("the scan filed %d pairs, want the nested-domain pair", filed)
	}
}

// Names equal once the legal form is removed file on create, where the new
// company has nothing stored to share yet.
func TestOneBrandFilesOnTheNameAlone(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	for incumbent, created := range map[string]string{"Fabrikam GmbH": "Fabrikam", "Acme Inc.": "ACME"} {
		stored := e.createNamedCompany(ctx, t, incumbent)
		if field := e.filedPairField(ctx, t, stored, e.createNamedCompany(ctx, t, created)); field == "" {
			t.Errorf("creating %s beside %s filed no pair", created, incumbent)
		}
	}
	line := e.createNamedCompany(ctx, t, lineName)
	if field := e.filedPairField(ctx, t, e.companyNamed(ctx, t, brandName), line); field != "" {
		t.Errorf("creating %s beside %s filed a pair on %q with nothing shared", lineName, brandName, field)
	}
}

// The nightly close step asks the filing rule's own question. An open pair
// filed on a name alone closes; the same pair with a shared fact stays.
func TestTheRecheckClosesASimilarNamePairWithNoSharedFact(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	brand, line := e.createNamedCompany(ctx, t, brandName), e.createNamedCompany(ctx, t, lineName)
	numberBrand := e.createNamedCompany(ctx, t, "Fabrikam AG")
	numberLine := e.createNamedCompany(ctx, t, "Fabrikam Cloud Services AG")
	e.statePhone(ctx, t, numberBrand, "+49 89 1234567")
	e.statePhone(ctx, t, numberLine, "+49891234567")
	e.forgetFiledPairs(ctx, t)
	stale := e.fileOldRulePair(ctx, t, entityCompany, brand.UUID, line.UUID, 0.8)
	corroborated := e.fileOldRulePair(ctx, t, entityCompany, numberBrand.UUID, numberLine.UUID, 0.8)

	if got := e.recheck(t); got.Closed != 1 {
		t.Fatalf("the pass closed %d pairs, want the name-only pair (%+v)", got.Closed, got)
	}
	if archived, verbs := e.candidateState(ctx, t, stale); !archived || verbs[len(verbs)-1] != "archive" {
		t.Errorf("the name-only pair: archived=%v history=%v, want archived with an archive audit row", archived, verbs)
	}
	if archived, _ := e.candidateState(ctx, t, corroborated); archived {
		t.Error("the pair sharing a phone number was closed; today's rule still files it")
	}
}

// companyNamed answers the live company of this display name.
func (e *dedupeEnv) companyNamed(ctx context.Context, t *testing.T, name string) ids.CompanyID {
	t.Helper()
	var id ids.UUID
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT id FROM company WHERE display_name = $1 AND archived_at IS NULL`, name).Scan(&id)
	}); err != nil {
		t.Fatalf("find %s: %v", name, err)
	}
	return ids.From[ids.CompanyKind](id)
}
