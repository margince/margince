// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What an approval email says about a proposed change. The record is the
// heading, then the proposal and each changed field as before and after. Each
// operand and further effect follow, then why the reader is asked. Every value comes from the card's preview, which is read
// as the recipient, so the email names nothing the card would not show them.

import (
	"fmt"
	"strconv"
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/platform/mailcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// mailChangeLimit bounds the fields one email lists; the card shows the rest.
const mailChangeLimit = 8

// The preview value kinds the email renders in a form of its own.
const (
	previewValueMoney       = "money"
	previewValueUnavailable = "unavailable_name"
	previewValueLifecycle   = "lifecycle"
	previewValueExecution   = "execution"
)

// changeMailFacts is the facts for an approval with a preview, as the card
// reads it: each changed field, operand and further effect. A preview that is
// not ready has only the card's sentence for its state.
func changeMailFacts(approval crmcontracts.Approval, words mailcopy.Copy) approvalMailFacts {
	facts := approvalMailFacts{Rich: true}
	if approval.TargetLabel != nil {
		facts.Heading = *approval.TargetLabel
	}
	if sentence := approvalSentence(approval); sentence != "" {
		facts.add(words.MailProposal, sentence)
	}
	lang := textlang.Lang(words.Language)
	reading := agents.ReadPreview(lang, approval.Preview)
	if reading.Note != "" {
		facts.add(words.MailPreviewStatus, reading.Note)
		addSummaryFields(&facts, approval.SummaryParts, lang, nil)
		return facts
	}
	var shown []approvalMailLine
	for _, change := range reading.Changes {
		before, beforeShown := mailPreviewValue(change.Before, words)
		after, afterShown := mailPreviewValue(change.After, words)
		if beforeShown && afterShown {
			shown = append(shown, approvalMailLine{Label: change.Label, Value: before + " → " + after})
		}
	}
	for _, detail := range reading.Details {
		if value, ok := mailPreviewValue(detail.Value, words); ok {
			shown = append(shown, approvalMailLine{Label: detail.Label, Value: value})
		}
	}
	for i, line := range shown {
		if i == mailChangeLimit {
			facts.add("", fmt.Sprintf(agents.MoreFields(lang), len(shown)-i))
			break
		}
		facts.add(line.Label, line.Value)
	}
	addSummaryFields(&facts, approval.SummaryParts, lang, previewFieldNames(approval.Preview))
	for _, effect := range reading.Effects {
		facts.add(words.MailAlsoChanges, effect.Label)
	}
	facts.add(words.MailWhyYou, reviewerSentence(approval.Preview.ReviewerReason, words))
	return facts
}

// approvalSentence is the act an approval proposes, without the field values
// the joined summary lists after it. An approval staged without parts reads
// as its whole summary.
func approvalSentence(approval crmcontracts.Approval) string {
	if approval.SummaryParts != nil && approval.SummaryParts.Sentence != "" {
		return approval.SummaryParts.Sentence
	}
	if approval.Summary != nil {
		return *approval.Summary
	}
	return ""
}

// addSummaryFields lists the summary's own field values that the preview does
// not name. A preview may leave a proposed field out, and the sentence alone
// would then lose it.
func addSummaryFields(facts *approvalMailFacts, parts *crmcontracts.ApprovalSummaryParts, lang textlang.Lang, named map[string]bool) {
	if parts == nil {
		return
	}
	for _, field := range parts.Fields {
		if named[field.Field] {
			continue
		}
		value := field.Value
		if value == "" {
			value = "—"
		}
		facts.add(agents.FieldLabel(lang, field.Field), value)
	}
	if parts.MoreFields > 0 {
		facts.add("", fmt.Sprintf(agents.MoreFields(lang), parts.MoreFields))
	}
}

// previewFieldNames collects the field names the preview carries, changed or operand.
func previewFieldNames(preview *crmcontracts.ApprovalPreview) map[string]bool {
	named := map[string]bool{}
	if preview == nil {
		return named
	}
	for _, change := range preview.Changes {
		named[change.Field] = true
	}
	if preview.Details != nil {
		for _, detail := range *preview.Details {
			named[detail.Field] = true
		}
	}
	return named
}

// mailPreviewValue is one preview value as plain text. A value the reader may
// not see is not shown, and neither is its line. A lifecycle code reads as its
// words, because this side holds no translation of it.
func mailPreviewValue(value crmcontracts.ApprovalPreviewValue, words mailcopy.Copy) (string, bool) {
	switch value.Kind {
	case previewValueEmpty:
		return "—", true
	case previewValueExecution:
		return words.MailSetOnApproval, true
	case previewValueUnavailable:
		return "", false
	case previewValueMoney:
		minor, err := strconv.ParseInt(value.Text, 10, 64)
		if err == nil && value.Currency != nil {
			return words.Money(minor, *value.Currency), true
		}
	case previewValueLifecycle:
		code := strings.ReplaceAll(value.Text, "_", " ")
		if code != "" {
			return strings.ToUpper(code[:1]) + code[1:], true
		}
	}
	return value.Text, value.Text != ""
}
