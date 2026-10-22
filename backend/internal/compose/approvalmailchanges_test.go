// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"fmt"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/platform/mailcopy"
)

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
	facts := changeMailFacts(crmcontracts.Approval{Preview: &crmcontracts.ApprovalPreview{Changes: changes}}, mailcopy.For("en"))
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
