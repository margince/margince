// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/platform/mailcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// factsText is an email's facts as the lines the email prints.
func factsText(facts approvalMailFacts) string {
	var out []string
	for _, line := range facts.Lines {
		out = append(out, line.Label+": "+line.Value)
	}
	return facts.Heading + "\n" + strings.Join(out, "\n")
}

func previewText(kind, text string) crmcontracts.ApprovalPreviewValue {
	return crmcontracts.ApprovalPreviewValue{Kind: kind, Text: text}
}

func TestAChangeEmailNamesTheRecordAndEachFieldBeforeAndAfter(t *testing.T) {
	words := mailcopy.For("en")
	usd := "USD"
	summary := `Update deal "Margince CRM build": overwrite Value, Expected close, edited by hand`
	approval := crmcontracts.Approval{
		Summary:     &summary,
		TargetLabel: new("Margince CRM build"),
		Preview: &crmcontracts.ApprovalPreview{
			Version:        1,
			State:          previewReadyState,
			ReviewerReason: reviewerOnBehalfOf,
			Changes: []crmcontracts.ApprovalPreviewChange{
				{
					Field: "amount_minor", Before: crmcontracts.ApprovalPreviewValue{Kind: "money", Text: "2500000", Currency: &usd},
					After: crmcontracts.ApprovalPreviewValue{Kind: "money", Text: "2703600", Currency: &usd},
				},
				{Field: "expected_close_date", Before: previewText(previewValueEmpty, ""), After: previewText(previewValueDate, "2026-10-11")},
				{Field: "owner_id", Before: previewText(previewValueUnavailable, ""), After: previewText("owner", "Rainer")},
				{Field: "lifecycle", Before: previewText(previewValueLifecycle, "customer"), After: previewText(previewValueLifecycle, "former_customer")},
				{Field: "closed_at", Before: previewText(previewValueEmpty, ""), After: previewText(previewValueExecution, "")},
				{Field: "address", Before: previewText(previewValueEmpty, ""), After: previewText("structured", `{"city":"Hanoi"}`)},
			},
		},
	}
	message := notificationMessage(notices.Notice{Subject: summary}, changeMailFacts(approval, words), "https://example.test", words)
	for _, want := range []string{
		"Margince CRM build",
		"Proposal: " + summary,
		"Value: 25,000.00 USD → 27,036.00 USD",
		"Expected close: — → 2026-10-11",
		"Lifecycle: Customer → Former customer",
		"Closed at: — → " + words.MailSetOnApproval,
		`Address: — → {"city":"Hanoi"}`,
		"Why you: " + words.MailReasonOnBehalf,
	} {
		if !strings.Contains(message.Text, want) {
			t.Errorf("the email does not say %q:\n%s", want, message.Text)
		}
	}
	if strings.Contains(message.Text, "Owner") {
		t.Errorf("a change whose old value the reader may not see was shown:\n%s", message.Text)
	}
	for _, wire := range []string{"amount_minor", "expected_close_date"} {
		if strings.Contains(message.Text, wire) {
			t.Errorf("the email names %s by its wire name:\n%s", wire, message.Text)
		}
	}
}

// The count after the limit covers only changes the email could have shown.
func TestAChangeEmailListsAtMostItsLimitAndCountsTheRest(t *testing.T) {
	changes := make([]crmcontracts.ApprovalPreviewChange, mailChangeLimit+4)
	changes[0] = crmcontracts.ApprovalPreviewChange{
		Field: "owner_id", Before: previewText(previewValueUnavailable, ""), After: previewText("owner", "Rainer"),
	}
	for i := 1; i < len(changes); i++ {
		changes[i] = crmcontracts.ApprovalPreviewChange{
			Field: fmt.Sprintf("field_%d", i), Before: previewText(previewValueText, "a"), After: previewText(previewValueText, "b"),
		}
	}
	facts := changeMailFacts(crmcontracts.Approval{Preview: &crmcontracts.ApprovalPreview{Version: 1, State: previewReadyState, Changes: changes}}, mailcopy.For("en"))
	listed := 0
	for _, line := range facts.Lines {
		if strings.HasPrefix(line.Label, "Field ") {
			listed++
		}
	}
	if listed != mailChangeLimit {
		t.Errorf("listed %d changes, want %d", listed, mailChangeLimit)
	}
	last := facts.Lines[len(facts.Lines)-1]
	if last.Label != "" || last.Value != "+3 more" {
		t.Errorf("the last line is %+v, want an unlabelled count of the rest", last)
	}
}

// A promotion's outcome is in its operands and effects, so the email lists
// them as the card does. A message body is shown on the card, not as a line.
func TestAChangeEmailSaysWhatThePromotionWillDo(t *testing.T) {
	words := mailcopy.For("en")
	details := []crmcontracts.ApprovalPreviewDetail{
		{Field: "promotion_contact_name", Value: previewText(previewValueText, "Pat Typed")},
		{Field: "promotion_contact_email", Value: previewText(previewValueUnavailable, "")},
		{Field: "body", Value: previewText(previewValueText, "Dear Pat")},
	}
	effects := []string{"promote_lead_archive_source", "promote_create_contact"}
	approval := crmcontracts.Approval{
		TargetLabel: new("Pat Typed"),
		Preview: &crmcontracts.ApprovalPreview{
			Version: 2, State: previewReadyState, ReviewerReason: reviewerOnBehalfOf,
			Changes: []crmcontracts.ApprovalPreviewChange{}, Details: &details, Effects: &effects,
		},
	}
	text := factsText(changeMailFacts(approval, words))
	for _, want := range []string{
		"Promotion contact name: Pat Typed",
		words.MailAlsoChanges + ": " + agents.EffectSentence(textlang.English, "promote_lead_archive_source"),
		words.MailAlsoChanges + ": " + agents.EffectSentence(textlang.English, "promote_create_contact"),
		"Why you: " + words.MailReasonOnBehalf,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the email does not say %q:\n%s", want, text)
		}
	}
	for _, hidden := range []string{"Promotion contact email", "Dear Pat"} {
		if strings.Contains(text, hidden) {
			t.Errorf("the email shows %q:\n%s", hidden, text)
		}
	}
}

// A reader who cannot open what a change touches is shown nothing of the plan.
// As on the card, they are told that a colleague who can must decide it.
func TestAChangeEmailSaysWhoMustDecideAChangeTheReaderCannotSee(t *testing.T) {
	words := mailcopy.For("en")
	approval := crmcontracts.Approval{
		TargetLabel: new("Kit Merge"),
		Summary:     new("Convert lead Kit Merge"),
		Preview: &crmcontracts.ApprovalPreview{
			Version: 2, State: approvals.PreviewNeedsWiderSight, ReviewerReason: reviewerOnBehalfOf,
			Changes: []crmcontracts.ApprovalPreviewChange{},
		},
	}
	facts := changeMailFacts(approval, words)
	want := []approvalMailLine{
		{Label: words.MailProposal, Value: "Convert lead Kit Merge"},
		{Label: words.MailPreviewStatus, Value: agents.PreviewStateSentence(textlang.English, approvals.PreviewNeedsWiderSight)},
	}
	if !slices.Equal(facts.Lines, want) {
		t.Errorf("the email says %+v, want %+v", facts.Lines, want)
	}
	if !awaitsWiderSight(approval.Preview) {
		t.Error("the email for a change its reader cannot see is not sent")
	}
}
