// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"strings"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/emailview"
	"github.com/margince/margince/backend/internal/platform/mailcopy"
)

func TestCustomerWorkIsUsefulWithoutAnAIFinding(t *testing.T) {
	title := "Northstar needs the revised quote"
	detail := "Buyer asked for the two-year option."
	item := crmcontracts.WorklistItem{Title: &title, Detail: &detail}
	for _, language := range mailcopy.Languages() {
		rendered := mailWorkText(item, mailcopy.For(string(language)), nil)
		if !strings.Contains(rendered, title) || !strings.Contains(rendered, detail) {
			t.Fatalf("%s lost the customer context: %s", language, rendered)
		}
	}
}

func TestAnUnnamedWorkItemCannotBecomeACountOnlyTeaser(t *testing.T) {
	if got := mailWorkText(crmcontracts.WorklistItem{}, mailcopy.For("en"), nil); got != "" {
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
	got := mailWorkText(item, mailcopy.For("de"), time.FixedZone("PDT", -7*60*60))
	if !strings.Contains(got, "23.10.2026") || strings.Contains(got, "23:59") || strings.Contains(got, "UTC") {
		t.Fatalf("date-only commitment changed: %s", got)
	}
	item.Source = crmcontracts.WorklistItemSourceTask
	got = mailWorkText(item, mailcopy.For("de"), zone)
	if !strings.Contains(got, "23:59") || !strings.Contains(got, "UTC+07:00") {
		t.Fatalf("timed task lost its instant: %s", got)
	}
}

func TestAnApprovalWithoutADecisionActionDoesNotAskForAttentionByMail(t *testing.T) {
	item := crmcontracts.WorklistItem{Source: crmcontracts.WorklistItemSourceApproval, Title: new("Review the stage change")}
	if got := mailWorkText(item, mailcopy.For("en"), nil); got != "" {
		t.Fatalf("unactionable approval mailed: %s", got)
	}
	item.Actions = []crmcontracts.WorklistItemActions{"decide"}
	if got := mailWorkText(item, mailcopy.For("en"), nil); got == "" {
		t.Fatal("ready approval omitted")
	}
}

// mailWorkText is a work item in the plain alternative, or "" when the item
// does not belong in mail.
func mailWorkText(item crmcontracts.WorklistItem, words mailcopy.Copy, zone *time.Location) string {
	row, ok := mailWorkItem(item, words, zone)
	if !ok {
		return ""
	}
	var content emailview.Content
	content.Item(row)
	return content.String()
}

func TestAWorkItemShowsItsCauseAndDeadlineAsBadges(t *testing.T) {
	title, cause := "Reply to Ana Sommer", "Customer waiting"
	due := time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)
	row, ok := mailWorkItem(crmcontracts.WorklistItem{Title: &title, CauseLabel: &cause, DueAt: &due}, mailcopy.For("en"), time.UTC)
	if !ok {
		t.Fatal("a named item was dropped")
	}
	var content emailview.Content
	content.Item(row)
	markup := emailview.Render(content.Message("Your priorities", "en", ""))
	for _, want := range []string{`class="c-warningText bg-warningSurface bd-warningBorder"`, ">" + cause + "</span>", `class="c-infoText bg-infoSurface bd-infoBorder"`} {
		if !strings.Contains(markup, want) {
			t.Errorf("the row lost %q", want)
		}
	}
}
