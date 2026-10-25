// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What an approval email says about a proposed change. The record is the
// heading, then the proposal, each changed field as before and after, and why
// the reader is asked. Every value comes from the card's preview, which is read
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

// mailChangeLimit bounds the changes one email lists; the card shows the rest.
const mailChangeLimit = 8

// The preview value kinds the email renders in a form of its own.
const (
	previewValueMoney       = "money"
	previewValueUnavailable = "unavailable_name"
	previewValueLifecycle   = "lifecycle"
	previewValueExecution   = "execution"
)

// changeMailFacts is the facts for an approval whose preview is ready.
func changeMailFacts(approval crmcontracts.Approval, words mailcopy.Copy) approvalMailFacts {
	facts := approvalMailFacts{Rich: true}
	if approval.TargetLabel != nil {
		facts.Heading = *approval.TargetLabel
	}
	if approval.Summary != nil {
		facts.add(words.MailProposal, plainSentence(*approval.Summary))
	}
	lang := textlang.Lang(words.Language)
	var shown []approvalMailLine
	for _, change := range approval.Preview.Changes {
		before, beforeShown := mailPreviewValue(change.Before, words)
		after, afterShown := mailPreviewValue(change.After, words)
		if beforeShown && afterShown {
			shown = append(shown, approvalMailLine{Label: agents.FieldLabel(lang, change.Field), Value: before + " → " + after})
		}
	}
	for i, line := range shown {
		if i == mailChangeLimit {
			facts.add("", fmt.Sprintf(agents.MoreFields(lang), len(shown)-i))
			break
		}
		facts.add(line.Label, line.Value)
	}
	facts.add(words.MailWhyYou, reviewerSentence(approval.Preview.ReviewerReason, words))
	return facts
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
