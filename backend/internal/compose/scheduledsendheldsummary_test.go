// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/activities"
)

// heldReasonEnum matches the contract's closed set of hold reasons.
var heldReasonEnum = regexp.MustCompile(`enum: \[(consent_withdrawn[^\]]*)\]`)

// Every hold reason the contract declares reaches the rep's card as words in
// every shipped language, never as its wire code. The census is the
// contract's own enum, so a reason added there without words fails here.
func TestEveryHoldReasonHasWordsOnTheCardInEveryLanguage(t *testing.T) {
	t.Parallel()
	contract, err := os.ReadFile("../../api/crm.yaml")
	if err != nil {
		t.Fatalf("reading the contract: %v", err)
	}
	found := heldReasonEnum.FindSubmatch(contract)
	if found == nil {
		t.Fatal("the contract declares no held_reason enum this test can read")
	}
	var reasons []string
	for _, reason := range strings.Split(string(found[1]), ",") {
		if reason = strings.TrimSpace(reason); reason != "null" {
			reasons = append(reasons, reason)
		}
	}
	if len(reasons) < 10 {
		t.Fatalf("read %d hold reasons from the contract, want at least 10: %v", len(reasons), reasons)
	}
	for lang, said := range approvalSummaryByLang {
		for _, reason := range reasons {
			summary := heldSummary(said, activities.HeldNotice{Subject: "Next steps", Reason: reason})
			if strings.Contains(summary, reason) || heldReasonText(said, reason) == "" {
				t.Errorf("%s: the card for %s reads %q, want words instead of the code", lang, reason, summary)
			}
		}
	}
}
