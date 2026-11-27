// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import "testing"

// Two numbered records at one mail domain are two contacts, however alike the
// rest of the name reads.
func TestNamesThatDisagreeOnANumberAreNotOfferedForMerging(t *testing.T) {
	t.Parallel()
	const sharedDomain = 0.8
	if got := contactConfidenceOf(contactNameFormsOf("Bulk 01"), contactNameFormsOf("Bulk 02"), sharedDomain); got >= dedupeReviewThreshold {
		t.Errorf("Bulk 01 against Bulk 02 scores %v, at or over the review bar %v", got, dedupeReviewThreshold)
	}
	if contactConfidenceReaches(contactNameFormsOf("Test User 7"), contactNameFormsOf("Test User 8"), 1, dedupeReviewThreshold) {
		t.Error("Test User 7 against Test User 8 reaches the review bar")
	}
}

func TestANumberWrittenTwoWaysIsOneNumber(t *testing.T) {
	t.Parallel()
	for _, pair := range [][2]string{
		{"Bulk 01", "Bulk 1"},
		{"Anna Muster", "Anna Muster"},
		{"Bulk 01", "Bulk"},
	} {
		if got := contactNameSimilarity(pair[0], pair[1]); got < nameSimilarity(pair[0], pair[1]) {
			t.Errorf("contactNameSimilarity(%q, %q) = %v, under the plain score %v", pair[0], pair[1], got, nameSimilarity(pair[0], pair[1]))
		}
	}
}
