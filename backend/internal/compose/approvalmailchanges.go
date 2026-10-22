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

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/platform/mailcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// mailChangeLimit bounds the changes one email lists; the card shows the rest.
const mailChangeLimit = 8

// changeMailFacts is the facts for an approval whose preview is ready.
func changeMailFacts(approval crmcontracts.Approval, words mailcopy.Copy) approvalMailFacts {
	facts := approvalMailFacts{Rich: true}
	if approval.TargetLabel != nil {
		facts.Heading = *approval.TargetLabel
	}
	if approval.Summary != nil {
		facts.add(words.MailProposal, *approval.Summary)
	}
	lang := textlang.Lang(words.Language)
	listed := 0
	for _, change := range approval.Preview.Changes {
		before, beforeShown := mailPreviewValue(change.Before, words)
		after, afterShown := mailPreviewValue(change.After, words)
		if !beforeShown || !afterShown {
			continue
		}
		if listed == mailChangeLimit {
			facts.add("", fmt.Sprintf(agents.MoreFields(lang), len(approval.Preview.Changes)-listed))
			break
		}
		facts.add(agents.FieldLabel(lang, change.Field), before+" → "+after)
		listed++
	}
	facts.add(words.MailWhyYou, reviewerSentence(approval.Preview.ReviewerReason, words))
	return facts
}

// mailPreviewValue is one preview value as plain text. A value the reader may
// not see, or one with no plain form, is not shown, and neither is its line.
func mailPreviewValue(value crmcontracts.ApprovalPreviewValue, words mailcopy.Copy) (string, bool) {
	switch value.Kind {
	case previewValueEmpty:
		return "—", true
	case "money":
		minor, err := strconv.ParseInt(value.Text, 10, 64)
		if err == nil && value.Currency != nil {
			return words.Money(minor, *value.Currency), true
		}
	case "unavailable_name", "structured":
		return "", false
	}
	return value.Text, value.Text != ""
}
