// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"fmt"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/heldreason"
)

// A stale approval at the fire holds the message with a reason its review can
// answer, instead of failing the job as an unclassified error.
func TestAStaleApprovalHoldsTheScheduledMessageForReview(t *testing.T) {
	reason, ok := heldreason.Of(fmt.Errorf("staging: %w", &InstructionStaleError{Why: "edited"}))
	if !ok || reason != heldreason.SendRefused {
		t.Fatalf("heldreason.Of = (%q, %v), want %q", reason, ok, heldreason.SendRefused)
	}
	if !slices.Contains(heldreason.Reviewable(), reason) {
		t.Errorf("reason %q is not one a review can stand over", reason)
	}
}
