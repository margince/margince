// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func companyIDPtr(id ids.CompanyID) *ids.CompanyID { return &id }

// PO-F-1's worked examples pin the arithmetic end to end; these assert
// the confidence AND which side of the threshold it lands on, because
// the number only matters through that comparison.
func TestPersonConfidenceReproducesTheSpecWorkedExamples(t *testing.T) {
	acme := ids.New[ids.CompanyKind]()
	globex := ids.New[ids.CompanyKind]()

	t.Run("same name-ish, same employer, queues for review", func(t *testing.T) {
		// "Jon Doe" vs "John Doe", both current-primary at Acme:
		// 0.55*0.9667 + 0.45*1.0 = 0.982 ≥ 0.72 → 🟡, not merged.
		got := personConfidence(
			PersonCandidate{FullName: "Jon Doe", CurrentPrimaryCompanyID: companyIDPtr(acme)},
			personCandidateRow{fullName: "John Doe", companyID: &acme},
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
		// < 0.72 → NO_MATCH. Two different people who share a name.
		got := personConfidence(
			PersonCandidate{FullName: "Jon Doe", CurrentPrimaryCompanyID: companyIDPtr(globex)},
			personCandidateRow{fullName: "John Doe", companyID: &acme},
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
			PersonCandidate{CurrentPrimaryCompanyID: companyIDPtr(acme)},
			personCandidateRow{companyID: &acme, companyDomain: &domain},
		)
		if got != 1.0 {
			t.Fatalf("company_match = %.2f, want 1.0", got)
		}
	})

	t.Run("shared email domain scores 0.8", func(t *testing.T) {
		got := companyMatch(
			PersonCandidate{Emails: []string{"NEW.HIRE@Acme.com"}, CurrentPrimaryCompanyID: companyIDPtr(other)},
			personCandidateRow{companyID: &acme, companyDomain: &domain},
		)
		if got != 0.8 {
			t.Fatalf("company_match = %.2f, want 0.8 — the domain match survives case", got)
		}
	})

	t.Run("no employer evidence scores 0", func(t *testing.T) {
		got := companyMatch(
			PersonCandidate{Emails: []string{"someone@globex.com"}},
			personCandidateRow{companyID: &acme, companyDomain: &domain},
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
		{"not-an-address", ""},
		{"", ""},
	} {
		if got := emailDomain(tc.in); got != tc.want {
			t.Fatalf("emailDomain(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
