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
	masked := crmcontracts.ApprovalPreviewValue{Kind: MaskedKind}
	shown := crmcontracts.ApprovalPreviewValue{Kind: "text", Text: "Harbor"}
	ready := func(changes []crmcontracts.ApprovalPreviewChange, details []crmcontracts.ApprovalPreviewDetail) *crmcontracts.ApprovalPreview {
		return &crmcontracts.ApprovalPreview{Version: 2, State: "ready", Changes: changes, Details: &details}
	}
	if HeldBack(ready([]crmcontracts.ApprovalPreviewChange{{Field: "name", Before: shown, After: shown}}, nil)) {
		t.Error("a preview that held nothing back reads as holding a value back")
	}
	if !HeldBack(ready(nil, []crmcontracts.ApprovalPreviewDetail{{Field: "to", Value: masked}})) {
		t.Error("a masked detail is not read as held back")
	}
	if !HeldBack(ready([]crmcontracts.ApprovalPreviewChange{{Field: "email", Before: masked, After: masked}}, nil)) {
		t.Error("a masked change is not read as held back")
	}
	if !(PreviewRead{Preview: ready(nil, nil), HeldBack: true}).PayloadWithheld() {
		t.Error("a ready preview that held a value back leaves the staged call on the approval")
	}
}

func TestOnlyADecidersStaleOrIncompleteProposalKeepsItsStagedCall(t *testing.T) {
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
		{"unavailable", false, true},
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
