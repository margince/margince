// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion
//gate:kind parity H3

package gates

// The answer sheet offers a customer review only the outcomes the server
// accepts. One side alone drifts: an answer the server refuses fails on submit,
// and one the sheet hides cannot be given at all.

import (
	"os"
	"regexp"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/modules/assurance"
)

const frontendReviewOutcomes = "../frontend/src/features/analytics/inputcheck.answer.tsx"

func TestTheAnswerSheetOffersAReviewTheOutcomesTheServerAccepts(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile(frontendReviewOutcomes)
	if err != nil {
		t.Fatal(err)
	}
	set := regexp.MustCompile(`reviewOutcomes = new Set\(\[([^\]]*)\]\)`).FindSubmatch(raw)
	if set == nil {
		t.Fatalf("%s declares no reviewOutcomes set, so this gate compares nothing", frontendReviewOutcomes)
	}
	var offered []string
	for _, m := range regexp.MustCompile(`"([a-z_]+)"`).FindAllSubmatch(set[1], -1) {
		offered = append(offered, string(m[1]))
	}
	accepted := assurance.ReviewOutcomes()
	slices.Sort(offered)
	slices.Sort(accepted)
	if !slices.Equal(offered, accepted) {
		t.Fatalf("the sheet offers a review %v and the server accepts %v; change both together", offered, accepted)
	}
}
