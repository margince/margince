// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// PO-F-1's worked examples pin the arithmetic end to end; these assert
// the confidence AND which side of the threshold it lands on, because
// the number only matters through that comparison.
func TestContactConfidenceReproducesTheSpecWorkedExamples(t *testing.T) {
	acme := ids.New[ids.CompanyKind]()
	globex := ids.New[ids.CompanyKind]()

	t.Run("same name-ish, same employer, queues for review", func(t *testing.T) {
		// "Jon Doe" vs "John Doe", both current-primary at Acme:
		// 0.55*0.9667 + 0.45*1.0 = 0.982 ≥ 0.72 → 🟡, not merged.
		got := contactConfidence(
			ContactCandidate{FullName: "Jon Doe", CurrentPrimaryCompanyID: new(acme)},
			contactCandidateRow{fullName: "John Doe", companyID: &acme},
		)
		if !closeToPrinted(got, 0.982) {
			t.Fatalf("confidence = %.4f, spec pins 0.982", got)
		}
		if got < dedupeReviewThreshold {
			t.Fatalf("confidence %.4f fell below the %.2f threshold — the spec queues this pair", got, dedupeReviewThreshold)
		}
	})

	t.Run("same name-ish, different employer, creates", func(t *testing.T) {
		// Same names, Globex vs Acme: 0.55*0.9667 + 0.45*0.0 = 0.532
		// < 0.72 → NO_MATCH. Two different contacts who share a name.
		got := contactConfidence(
			ContactCandidate{FullName: "Jon Doe", CurrentPrimaryCompanyID: new(globex)},
			contactCandidateRow{fullName: "John Doe", companyID: &acme},
		)
		if !closeToPrinted(got, 0.532) {
			t.Fatalf("confidence = %.4f, spec pins 0.532", got)
		}
		if got >= dedupeReviewThreshold {
			t.Fatalf("confidence %.4f reached the %.2f threshold — the spec creates here", got, dedupeReviewThreshold)
		}
	})
}

func TestDedupeWeightsSumToOne(t *testing.T) {
	// The spec pins "weights sum to 1.0 so confidence ∈ [0,1]" — the
	// threshold comparison is only meaningful while that holds.
	if sum := dedupeNameWeight + dedupeCompanyDomainWeight; !closeEnough(sum, 1.0) {
		t.Fatalf("weights sum to %.4f, want 1.0 — confidence is no longer in [0,1]", sum)
	}
}

func TestCompanyMatchPrefersTheMostSpecificEvidence(t *testing.T) {
	acme := ids.New[ids.CompanyKind]()
	other := ids.New[ids.CompanyKind]()
	domain := "acme.com"

	t.Run("shared current-primary employer scores 1.0", func(t *testing.T) {
		got := companyMatch(
			ContactCandidate{CurrentPrimaryCompanyID: new(acme)},
			contactCandidateRow{companyID: &acme, companyDomain: &domain},
		)
		if got != 1.0 {
			t.Fatalf("company_match = %.2f, want 1.0", got)
		}
	})

	t.Run("shared email domain scores 0.8", func(t *testing.T) {
		got := companyMatch(
			ContactCandidate{Emails: []string{"NEW.HIRE@Acme.com"}, CurrentPrimaryCompanyID: new(other)},
			contactCandidateRow{companyID: &acme, companyDomain: &domain},
		)
		if got != 0.8 {
			t.Fatalf("company_match = %.2f, want 0.8 — the domain match survives case", got)
		}
	})

	t.Run("no employer evidence scores 0", func(t *testing.T) {
		got := companyMatch(
			ContactCandidate{Emails: []string{"someone@globex.com"}},
			contactCandidateRow{companyID: &acme, companyDomain: &domain},
		)
		if got != 0.0 {
			t.Fatalf("company_match = %.2f, want 0.0", got)
		}
	})
}

func TestEmailDomain(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"Jane.Doe@Acme.COM", "acme.com"},
		{"  jane@acme.com  ", "acme.com"},
		{"weird@sub@acme.com", "acme.com"},
		{"anna@Bücher.example", "xn--bcher-kva.example"},
		{"anna@xn--bcher-kva.example", "xn--bcher-kva.example"},
		{"not-an-address", ""},
		{"", ""},
	} {
		if got := emailDomain(tc.in); got != tc.want {
			t.Fatalf("emailDomain(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// A Unicode domain and its punycode spelling are one mail domain, so a
// contact's addresses in both spellings name it once.
func TestBothSpellingsOfADomainAreOneMailDomain(t *testing.T) {
	got := mailDomainsOf([]string{"anna@bücher.example", "ben@xn--bcher-kva.example", "no-domain", "cara@acme.com"})
	if want := []string{"xn--bcher-kva.example", "acme.com"}; !slices.Equal(got, want) {
		t.Fatalf("mailDomainsOf = %v, want %v", got, want)
	}
}

// A domain in unicode and the same domain in punycode are one domain here, as
// they are everywhere else in the tree.
//
// Two companies whose names differ by an accent stay two. The fold punycodes
// rather than unaccenting, and that pair is the one the old fold guarded.
func TestOneDomainFoldKeepsAccentedCompaniesApart(t *testing.T) {
	t.Parallel()
	munich := normalizeDomain("münich.example")
	if got := normalizeDomain("xn--mnich-kva.example"); got != munich {
		t.Errorf("punycode folds to %q and unicode to %q, want one domain", got, munich)
	}
	if got := normalizeDomain("MÜNICH.example"); got != munich {
		t.Errorf("upper case folds to %q, want %q", got, munich)
	}
	if plain := normalizeDomain("munich.example"); plain == munich {
		t.Errorf("munich.example folds to %q, the same as the accented one", plain)
	}
}
