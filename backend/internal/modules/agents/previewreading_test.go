// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"encoding/json"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

func readApprovalOf(t *testing.T, preview *crmcontracts.ApprovalPreview) StagedApproval {
	t.Helper()
	tool := readApprovalTool{inbox: &fakeInbox{one: StagedApproval{
		StagedActionID: ids.NewV7(), Kind: "promote_lead", Status: "pending", Preview: preview,
	}}}
	out, err := tool.Handle(context.Background(), json.RawMessage(`{"staged_action_id":"`+ids.NewV7().String()+`"}`))
	if err != nil {
		t.Fatalf("Handle err = %v", err)
	}
	var got StagedApproval
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	return got
}

func TestReadApprovalSaysWhatTheCardSaysApprovingDoes(t *testing.T) {
	details := []crmcontracts.ApprovalPreviewDetail{
		{Field: "promotion_contact_name", Value: crmcontracts.ApprovalPreviewValue{Kind: "text", Text: "Pat Typed"}},
		{Field: "body", Value: crmcontracts.ApprovalPreviewValue{Kind: "text", Text: "Hello"}},
	}
	effects := []string{"promote_lead_archive_source", "an_effect_without_words"}
	got := readApprovalOf(t, &crmcontracts.ApprovalPreview{
		Version: 2, State: "ready", Changes: []crmcontracts.ApprovalPreviewChange{}, Details: &details, Effects: &effects,
	})

	if got.PreviewState != "ready" || got.PreviewNote != "" {
		t.Errorf("state = %q, note = %q, want a ready preview with no note", got.PreviewState, got.PreviewNote)
	}
	if len(got.Details) != 1 || got.Details[0].Value.Text != "Pat Typed" || got.Details[0].Label != "Promotion contact name" {
		t.Errorf("details = %+v, want the contact name and no message body", got.Details)
	}
	want := []PreviewEffect{
		{Effect: "promote_lead_archive_source", Label: EffectSentence(textlang.English, "promote_lead_archive_source")},
		{Effect: "an_effect_without_words", Label: "an_effect_without_words"},
	}
	if len(got.Effects) != len(want) || got.Effects[0] != want[0] || got.Effects[1] != want[1] {
		t.Errorf("effects = %+v, want %+v", got.Effects, want)
	}
	if want[0].Label == want[0].Effect {
		t.Error("the archive effect has no sentence, so the card's words never reach the tool")
	}
}

func TestReadApprovalSaysWhoMustDecideAPreviewItCannotShow(t *testing.T) {
	for _, tc := range []struct {
		name    string
		preview crmcontracts.ApprovalPreview
		state   string
	}{
		{"wider sight", crmcontracts.ApprovalPreview{Version: 2, State: "needs_wider_sight"}, "needs_wider_sight"},
		{"stale", crmcontracts.ApprovalPreview{Version: 1, State: "stale"}, "stale"},
		{"unknown state", crmcontracts.ApprovalPreview{Version: 1, State: "conflict"}, "conflict"},
		{"unknown version", crmcontracts.ApprovalPreview{Version: 9, State: "ready"}, "unavailable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := readApprovalOf(t, &tc.preview)
			if got.PreviewState != tc.state || got.PreviewNote != PreviewStateSentence(textlang.English, tc.state) {
				t.Errorf("state = %q, note = %q", got.PreviewState, got.PreviewNote)
			}
			if got.PreviewNote == "" || got.Changes != nil || got.Details != nil || got.Effects != nil {
				t.Errorf("a preview that is not ready reads as %+v", got)
			}
		})
	}
	if note := PreviewStateSentence(textlang.English, "needs_wider_sight"); note == PreviewStateSentence(textlang.English, "unavailable") {
		t.Errorf("wider sight reads as the unavailable sentence: %q", note)
	}
}
