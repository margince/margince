// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"slices"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func TestOnlyCurrentCommercialContextOffersADecision(t *testing.T) {
	for _, state := range []string{"ready", "stale", "incomplete", "unavailable"} {
		item := approvalItem(crmcontracts.Approval{Kind: "advance_deal", Preview: &crmcontracts.ApprovalPreview{Version: 1, State: state}}, nil)
		if got := slices.Contains(item.Actions, "decide"); got != (state == "ready") {
			t.Errorf("%s offers a decision: %v", state, got)
		}
	}
}
