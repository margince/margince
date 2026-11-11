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
		addMailLines(&facts, summaryMailLines(approval.SummaryParts, lang, nil), approval.SummaryParts, lang)
		return facts
	}
	var shown []approvalMailLine
	rendered := map[string]bool{}
	for _, change := range reading.Changes {
		before, beforeShown := mailPreviewValue(change.Before, words)
		after, afterShown := mailPreviewValue(change.After, words)
		if beforeShown && afterShown {
			shown = append(shown, approvalMailLine{Label: change.Label, Value: before + " → " + after})
			rendered[change.Field] = true
		}
	}
	for _, detail := range reading.Details {
		if value, ok := mailPreviewValue(detail.Value, words); ok {
			shown = append(shown, approvalMailLine{Label: detail.Label, Value: value})
			rendered[detail.Field] = true
		}
	}
	shown = append(shown, summaryMailLines(approval.SummaryParts, lang, rendered)...)
	addMailLines(&facts, shown, approval.SummaryParts, lang)
	for _, effect := range reading.Effects {
		facts.add(words.MailAlsoChanges, effect.Label)
	}
	addPreviewSections(&facts, reading, words)
	facts.add(words.MailWhyYou, reviewerSentence(approval.Preview.ReviewerReason, words))
	return facts
}

// approvalSentence is the act an approval proposes, without the field values
// the joined summary lists after it. An approval staged without parts reads
// as its whole summary.
func approvalSentence(approval crmcontracts.Approval) string {
	if approval.SystemDisplay != nil && approval.SystemDisplay.Subject != nil {
		return *approval.SystemDisplay.Subject
	}
	if approval.SummaryParts != nil && approval.SummaryParts.Sentence != "" {
		return approval.SummaryParts.Sentence
	}
	if approval.Summary != nil {
		return *approval.Summary
	}
	return ""
}

// summaryMailLines is the summary's own field values that the email has not
// already rendered from the preview. A preview may leave a proposed value out,
// or hide one side of it, and the sentence alone would then lose it.
func summaryMailLines(parts *crmcontracts.ApprovalSummaryParts, lang textlang.Lang, rendered map[string]bool) []approvalMailLine {
	if parts == nil {
		return nil
	}
	var lines []approvalMailLine
	for _, field := range parts.Fields {
		if rendered[field.Field] {
			continue
		}
		value := field.Value
		if value == "" {
			value = "—"
		}
		lines = append(lines, approvalMailLine{Label: agents.FieldLabel(lang, field.Field), Value: value})
	}
	return lines
}

// addMailLines lists at most mailChangeLimit lines and counts the rest,
// together with the fields the summary itself left out.
func addMailLines(facts *approvalMailFacts, lines []approvalMailLine, parts *crmcontracts.ApprovalSummaryParts, lang textlang.Lang) {
	shown := min(len(lines), mailChangeLimit)
	for _, line := range lines[:shown] {
		facts.add(line.Label, line.Value)
	}
	more := len(lines) - shown
	if parts != nil {
		more += parts.MoreFields
	}
	if more > 0 {
		facts.add("", fmt.Sprintf(agents.MoreFields(lang), more))
	}
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

func addPreviewSections(facts *approvalMailFacts, reading agents.PreviewReading, words mailcopy.Copy) {
	lang := textlang.Lang(words.Language)
	for _, conflict := range reading.Conflicts {
		facts.add(agents.PreviewSectionLabel(lang, "conflict"), conflict.Summary)
	}
	for _, file := range reading.Files {
		facts.add(agents.PreviewSectionLabel(lang, "file"), fmt.Sprintf("%s (%s, %d B)", file.Filename, file.ContentType, file.ByteSize))
	}
	for _, row := range reading.ImportRows {
		label := agents.PreviewSectionLabel(lang, "row")
		label = strings.NewReplacer("{kind}", row.ObjectLabel, "{line}", strconv.Itoa(row.Line), "{key}", row.SourceKey).Replace(label)
		facts.add(label, row.OutcomeLabel)
		if row.Target != nil {
			if value, ok := mailPreviewValue(*row.Target, words); ok {
				facts.add(label, value)
			}
		}
		for _, change := range row.Changes {
			before, beforeOK := mailPreviewValue(change.Before, words)
			after, afterOK := mailPreviewValue(change.After, words)
			if beforeOK && afterOK {
				facts.add(change.Label, before+" → "+after)
			}
		}
		for _, detail := range row.Details {
			if value, ok := mailPreviewValue(detail.Value, words); ok {
				facts.add(detail.Label, value)
			}
		}
		for _, effect := range row.Effects {
			facts.add(words.MailAlsoChanges, effect.Label)
		}
	}
}
