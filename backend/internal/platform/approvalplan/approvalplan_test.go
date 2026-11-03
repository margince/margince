// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvalplan

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func TestOnlyAReadyPreviewOfAKnownVersionShowsItsPlan(t *testing.T) {
	cases := []struct {
		preview  *crmcontracts.ApprovalPreview
		withheld bool
	}{
		{nil, false},
		{&crmcontracts.ApprovalPreview{Version: 2, State: "ready"}, false},
		{&crmcontracts.ApprovalPreview{Version: 1, State: "ready"}, false},
		{&crmcontracts.ApprovalPreview{Version: 2, State: "needs_wider_sight"}, true},
		{&crmcontracts.ApprovalPreview{Version: 2, State: "stale"}, true},
		{&crmcontracts.ApprovalPreview{Version: 3, State: "ready"}, true},
	}
	for _, tc := range cases {
		if got := Withheld(tc.preview); got != tc.withheld {
			t.Errorf("Withheld(%+v) = %v, want %v", tc.preview, got, tc.withheld)
		}
	}
}

func TestAPreviewThatHeldAValueBackWithholdsTheStagedCall(t *testing.T) {
	ready := &crmcontracts.ApprovalPreview{Version: 2, State: "ready"}
	if (PreviewRead{Preview: ready}).PayloadWithheld() {
		t.Error("a ready preview that held nothing back withholds the staged call")
	}
	if !(PreviewRead{Preview: ready, HeldBack: true}).PayloadWithheld() {
		t.Error("a ready preview that held a value back leaves the staged call on the approval")
	}
}

func TestOnlyADecidersProposalThatIsNotReadyKeepsItsStagedCall(t *testing.T) {
	for _, tc := range []struct {
		state        string
		builtAsAgent bool
		withheld     bool
	}{
		{"ready", false, false},
		{"ready", true, false},
		{"stale", false, false},
		{"incomplete", false, false},
		{"stale", true, true},
		{"incomplete", true, true},
		{"unavailable", false, false},
		{"unavailable", true, true},
		{NeedsWiderSight, false, true},
		{"a_state_added_later", false, true},
	} {
		preview := &crmcontracts.ApprovalPreview{Version: 2, State: tc.state}
		if got := (PreviewRead{Preview: preview, BuiltAsAgent: tc.builtAsAgent}).PayloadWithheld(); got != tc.withheld {
			t.Errorf("%s (agent=%v): withheld=%v, want %v", tc.state, tc.builtAsAgent, got, tc.withheld)
		}
	}
	if (PreviewRead{BuiltAsAgent: true}).PayloadWithheld() {
		t.Error("an approval with no preview lost its staged call")
	}
}
