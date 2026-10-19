// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/mailcopy"
)

func TestCustomerWorkIsUsefulWithoutAnAIFinding(t *testing.T) {
	title := "Northstar needs the revised quote"
	detail := "Buyer asked for the two-year option."
	item := crmcontracts.WorklistItem{Title: &title, Detail: &detail}
	for _, language := range mailcopy.Languages() {
		rendered := mailWorkItem(item, mailcopy.For(string(language)))
		if !strings.Contains(rendered, title) || !strings.Contains(rendered, detail) {
			t.Fatalf("%s lost the customer context: %s", language, rendered)
		}
	}
}

func TestAnUnnamedWorkItemCannotBecomeACountOnlyTeaser(t *testing.T) {
	if got := mailWorkItem(crmcontracts.WorklistItem{}, mailcopy.For("en")); got != "" {
		t.Fatalf("unnameable item = %q", got)
	}
}

func TestMailActionsUseTheWorklistWireVocabulary(t *testing.T) {
	words := mailcopy.For("en")
	complete := crmcontracts.WorklistItemPrimaryActionComplete
	for _, test := range []struct {
		item crmcontracts.WorklistItem
		want string
	}{
		{crmcontracts.WorklistItem{PrimaryAction: &complete}, words.MailTask},
		{crmcontracts.WorklistItem{Actions: []crmcontracts.WorklistItemActions{crmcontracts.WorklistItemActionsReply}}, words.MailReply},
		{crmcontracts.WorklistItem{Source: crmcontracts.WorklistItemSourceMeeting}, words.MailPrepare},
		{crmcontracts.WorklistItem{Source: crmcontracts.WorklistItemSourceMeetingOutcome}, words.MailReview},
	} {
		if got := mailWorkAction(test.item, words); got != test.want {
			t.Errorf("action = %q, want %q", got, test.want)
		}
	}
}
