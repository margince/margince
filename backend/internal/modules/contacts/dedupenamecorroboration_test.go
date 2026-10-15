// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A shared employer lifts a pair over the review bar only when the family names
// agree and the given names are close.
func TestEmployerSignalNeedsCorroboratingNames(t *testing.T) {
	acme := ids.New[ids.CompanyKind]()
	cases := []struct {
		name       string
		candidate  string
		incumbent  string
		sameEmploy bool
		wantReview bool
	}{
		{"same given name, different surname, same employer", "Maria Garcia", "Maria Gomez", true, false},
		{"same given name, different surname, no signal", "Maria Garcia", "Maria Gomez", false, false},
		{"same surname, given name typo, same employer", "Jon Doe", "John Doe", true, true},
		{"same surname, given name typo, no signal", "Jon Doe", "John Doe", false, false},
		{"same employer, similar name", "Katarina Novak", "Katharina Novak", true, true},
		{"same surname, unrelated given name, same employer", "Anna Smith", "Peter Smith", true, false},
		{"reversed name order, same employer", "Doe, Jon", "John Doe", true, true},
		{"initial against a full given name, same employer", "J. Doe", "John Doe", true, true},
		{"Vietnamese name against its ASCII spelling", "Nguyễn Văn Đức", "Nguyen Van Duc", true, true},
		{"Vietnamese name against its ASCII spelling, no signal", "Nguyễn Văn Đức", "Nguyen Van Duc", false, false},
		{"Polish l-stroke against ASCII", "Łukasz Kowalski", "Lukasz Kowalski", true, true},
		{"generational suffix", "John Smith Jr", "John Smith", true, true},
		{"honorific", "Dr. Jon Doe", "John Doe", true, true},
		{"academic suffix after a comma", "John Smith, PhD", "John Smith", true, true},
		{"hyphenated family name against one half", "John Doe-Smith", "John Doe", true, true},
		{"one-word name against a full name is not enough", "Alex", "Alex Johnson", true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := ContactCandidate{FullName: tc.candidate}
			row := contactCandidateRow{fullName: tc.incumbent}
			if tc.sameEmploy {
				c.CurrentPrimaryCompanyID, row.companyID = &acme, &acme
			}
			got := contactConfidence(c, row)
			if review := got >= dedupeReviewThreshold; review != tc.wantReview {
				t.Fatalf("confidence %.4f, files a candidate = %v, want %v", got, review, tc.wantReview)
			}
		})
	}
}

func TestLeadConfidenceUsesTheSameNameCorroboration(t *testing.T) {
	company := "Acme"
	row := leadCandidateRow{fullName: new("Maria Gomez"), company: &company}
	if got := leadConfidence("Maria Garcia", company, "", row); got >= dedupeReviewThreshold {
		t.Fatalf("lead pair sharing only a given name scored %.4f, at or above the review bar", got)
	}
	row.fullName = new("John Doe")
	if got := leadConfidence("Jon Doe", company, "", row); got < dedupeReviewThreshold {
		t.Fatalf("lead twin at one employer scored %.4f, below the review bar", got)
	}
}
