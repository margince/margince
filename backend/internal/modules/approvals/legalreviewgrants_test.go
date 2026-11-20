// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/platform/approvalsubject"
)

// The card is decided under exactly the grants the review's own answer takes,
// so a seat is told about a review only if it could answer it.
func TestTheLegalReviewCardTakesTheGrantsTheAnswerTakes(t *testing.T) {
	var got []string
	for _, g := range decisionGrants[approvalsubject.KindLegalReviewRequest] {
		if g.Object != approvalsubject.ObjectLegalReview {
			t.Errorf("card grant on %q, want %q", g.Object, approvalsubject.ObjectLegalReview)
		}
		got = append(got, string(g.Action))
	}
	var want []string
	for _, a := range approvalsubject.LegalReviewDecisionActions() {
		want = append(want, string(a))
	}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("card actions %v, answer actions %v", got, want)
	}
}
