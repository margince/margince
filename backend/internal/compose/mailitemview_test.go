// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"strings"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/mailcopy"
)

func TestCustomerWorkIsUsefulWithoutAnAIFinding(t *testing.T) {
	title := "Northstar needs the revised quote"
	detail := "Buyer asked for the two-year option."
	item := crmcontracts.WorklistItem{Title: &title, Detail: &detail}
	for _, language := range mailcopy.Languages() {
		rendered := mailWorkItem(item, mailcopy.For(string(language)), nil)
		if !strings.Contains(rendered, title) || !strings.Contains(rendered, detail) {
			t.Fatalf("%s lost the customer context: %s", language, rendered)
		}
	}
}

func TestAnUnnamedWorkItemCannotBecomeACountOnlyTeaser(t *testing.T) {
	if got := mailWorkItem(crmcontracts.WorklistItem{}, mailcopy.For("en"), nil); got != "" {
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

func TestDateOnlyPromiseDoesNotInventOrShiftADeadlineTime(t *testing.T) {
	zone := time.FixedZone("ICT", 7*60*60)
	due := time.Date(2026, 10, 23, 23, 59, 59, 0, zone)
	title := "Agree the renewal"
	item := crmcontracts.WorklistItem{Source: crmcontracts.WorklistItemSourceWeeklyCommitment, Title: &title, DueAt: &due}
	got := mailWorkItem(item, mailcopy.For("de"), time.FixedZone("PDT", -7*60*60))
	if !strings.Contains(got, "23.10.2026") || strings.Contains(got, "23:59") || strings.Contains(got, "UTC") {
		t.Fatalf("date-only commitment changed: %s", got)
	}
	item.Source = crmcontracts.WorklistItemSourceTask
	got = mailWorkItem(item, mailcopy.For("de"), zone)
	if !strings.Contains(got, "23:59") || !strings.Contains(got, "UTC+07:00") {
		t.Fatalf("timed task lost its instant: %s", got)
	}
}
