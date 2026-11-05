// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"testing"

	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A company not stored yet is compared on what its create or lookup carries.
func TestACreateAtTheSameStreetAddressFilesAtCreateTime(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	brand := e.companyAt(ctx, t, brandName, "Marienplatz 8", "")
	line := e.companyAt(ctx, t, lineName, "Marienplatz 8", "")
	if field := e.filedPairField(ctx, t, brand, line); field == "" {
		t.Error("creating a similar-named company at the same street address filed no pair")
	}
}

func TestALookupCarryingTheSameNumberMeetsTheSimilarName(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	brand := e.createNamedCompany(ctx, t, brandName)
	e.statePhone(ctx, t, brand, "+49 89 1234567")
	for numbers, want := range map[string]DedupeDecision{
		"0049 89 1234567": DecisionFuzzyReview, "+49 89 7654321": DecisionNoMatch,
	} {
		got := e.dedupeCompanyInTx(ctx, t, CompanyCandidate{DisplayName: lineName, Phones: []string{numbers}})
		if got.Decision != want {
			t.Errorf("a lookup carrying %s answered %s, want %s", numbers, got.Decision, want)
		}
	}
	// With the lookup, a second stored holder makes three: a switchboard.
	e.statePhone(ctx, t, e.createNamedCompany(ctx, t, "Northwind Traders"), "+49 89 1234567")
	if got := e.dedupeCompanyInTx(ctx, t, CompanyCandidate{DisplayName: lineName, Phones: []string{"+49891234567"}}); got.Decision != DecisionNoMatch {
		t.Errorf("a lookup carrying a switchboard number answered %s, want no match", got.Decision)
	}
}

// A contact kept private by its owner is not evidence: a reader who sees both
// companies would otherwise learn of an employment they cannot see.
func TestOnlyAContactEverySeatSeesLinksTwoSimilarCompanies(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	brand := e.createNamedCompany(ctx, t, brandName)
	line := e.createNamedCompany(ctx, t, lineName)
	colleague := mustContact(t, e, "Greta Hoffmann")
	employ(t, e, colleague, brand)
	employ(t, e, colleague, line)
	if _, err := e.store.UpdateContact(ctx, colleague, UpdateContactInput{Visibility: new("owner")}); err != nil {
		t.Fatalf("make the contact private: %v", err)
	}
	e.forgetFiledPairs(ctx, t)
	if filed := e.scanOne(t, DedupeScanCompanies, brand.UUID); filed != 0 {
		t.Errorf("a private contact at both filed %d pairs, want none", filed)
	}
	if _, err := e.store.UpdateContact(ctx, colleague, UpdateContactInput{Visibility: new("workspace")}); err != nil {
		t.Fatalf("publish the contact: %v", err)
	}
	if filed := e.scanOne(t, DedupeScanCompanies, brand.UUID); filed != 1 {
		t.Errorf("the published contact at both filed %d pairs, want the pair", filed)
	}
}

func TestAJobThatHasNotStartedLinksNothing(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	brand := e.createNamedCompany(ctx, t, brandName)
	line := e.createNamedCompany(ctx, t, lineName)
	colleague := mustContact(t, e, "Greta Hoffmann")
	employ(t, e, colleague, brand)
	starts := clocktest.Now(t).AddDate(1, 0, 0)
	if _, err := e.store.CreateRelationship(ctx, CreateRelationshipInput{
		Kind: employmentKind, ContactID: &colleague, CompanyID: &line, StartedAt: &starts, Source: "manual",
	}); err != nil {
		t.Fatalf("record the future employment: %v", err)
	}
	e.forgetFiledPairs(ctx, t)
	if filed := e.scanOne(t, DedupeScanCompanies, brand.UUID); filed != 0 {
		t.Errorf("a job starting next year filed %d pairs, want none", filed)
	}
}

// A number three companies state is a switchboard and links none of them.
func TestASwitchboardNumberLinksNothing(t *testing.T) {
	for holders, filed := range map[int]bool{2: true, 3: false} {
		e := setupDedupe(t)
		ctx := e.as()
		brand := e.createNamedCompany(ctx, t, brandName)
		e.statePhone(ctx, t, brand, "+49 89 1234567")
		e.statePhone(ctx, t, e.createNamedCompany(ctx, t, lineName), "0049 89 1234567")
		if holders == 3 {
			e.statePhone(ctx, t, e.createNamedCompany(ctx, t, "Northwind Traders"), "+49 (89) 1234567")
		}
		e.forgetFiledPairs(ctx, t)
		if got := e.scanOne(t, DedupeScanCompanies, brand.UUID); (got == 1) != filed {
			t.Errorf("a number %d companies state: the scan filed %d pairs, want filed=%v", holders, got, filed)
		}
	}
}

// An archived company is no duplicate to review, whichever side it is on and
// whatever lane filed the pair.
func TestTheRecheckClosesACompanyPairWithAnArchivedEnd(t *testing.T) {
	e := setupDedupe(t)
	ctx := asArchiver(e)
	var pairs []ids.UUID
	for i, confidence := range []float64{0.8, identityConflictConfidence} {
		for _, archiveLeft := range []bool{true, false} {
			left := e.plainCompany(ctx, t, "Fabrikam "+string(rune('A'+i))+" Left")
			right := e.plainCompany(ctx, t, "Fabrikam "+string(rune('A'+i))+" Right")
			pairs = append(pairs, e.fileOldRulePair(ctx, t, entityCompany, left, right, confidence))
			gone := right
			if archiveLeft {
				gone = left
			}
			if _, err := e.store.ArchiveCompany(ctx, ids.From[ids.CompanyKind](gone), nil); err != nil {
				t.Fatalf("archive company: %v", err)
			}
		}
	}
	e.recheck(t)
	for i, id := range pairs {
		if archived, _ := e.candidateState(ctx, t, id); !archived {
			t.Errorf("pair %d with an archived company is still open", i)
		}
	}
}
