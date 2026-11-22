// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"fmt"
	"testing"

	"github.com/margince/margince/backend/internal/platform/httperr"
)

// wireReviewState is the review state a legal refusal carries in its field
// fault, as the HTTP and tool surfaces read it.
func wireReviewState(t *testing.T, err error) string {
	t.Helper()
	fault, ok := httperr.Classify(err)
	if !ok || len(fault.Fields) != 1 {
		t.Fatalf("the refusal classified as %+v, want one field fault", fault)
	}
	if fault.Fields[0].Code != "legal_review_required" {
		t.Fatalf("the field fault code is %q, want legal_review_required", fault.Fields[0].Code)
	}
	return fault.Fields[0].Params["review_state"]
}

// The screen words the next step from the state, so each review standing
// names its own state on the wire.
func TestTheLegalRefusalNamesTheReviewStateOnTheWire(t *testing.T) {
	for _, tc := range []struct {
		name    string
		refusal LegalReviewRequiredError
		want    string
	}{
		{"never asked", LegalReviewRequiredError{}, reviewStateNone},
		{"withdrawn", LegalReviewRequiredError{ReviewStatus: ReviewWithdrawn}, reviewStateNone},
		{"waiting for legal", LegalReviewRequiredError{ReviewStatus: ReviewRequested}, reviewStatePending},
		{"changes asked", LegalReviewRequiredError{ReviewStatus: ReviewChangesNeeded}, reviewStateChangesNeeded},
		{"terms changed", LegalReviewRequiredError{ReviewStatus: ReviewApproved, TermsChanged: true}, reviewStateTermsChanged},
	} {
		t.Run(tc.name, func(t *testing.T) {
			refusal := tc.refusal
			if got := wireReviewState(t, fmt.Errorf("advance: %w", &refusal)); got != tc.want {
				t.Errorf("review_state = %q, want %q", got, tc.want)
			}
		})
	}
}
