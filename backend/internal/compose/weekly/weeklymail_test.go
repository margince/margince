// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package weekly

// What the weekly message actually says.
//
// The body is the one part of this arc a rep reads OUTSIDE the product, where
// nothing can be clicked to check it and no panel is beside it to correct it.
// So the shape is asserted here rather than eyeballed once.

import (
	"strings"
	"testing"
	"time"
)

func mailFixture() Review {
	amount := int64(1250000)
	return Review{
		LocalWeekStart: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		Counts: Counts{
			TasksDue: 9, TasksDone: 7, TasksCarriedOver: 2,
			DealsMoved: 3, DealsWon: 1, DealsLost: 1,
			ProposalsAccepted: 4, ProposalsRejected: 2,
			BriefItemsActed: 6, BriefItemsDismissed: 3,
		},
		Deals: []DealLine{{
			Label: "Stahlbau Krämer", Outcome: OutcomeWon,
			AmountMinor: &amount, Currency: "EUR",
			OccurredAt: time.Date(2026, 6, 3, 11, 0, 0, 0, time.UTC),
		}},
	}
}

// The counts a rep reads on the screen are the counts the message states. Both
// render from one Review, so this test is what holds them to it: a message
// disagreeing with the panel about somebody's own week gives them no way to
// tell which one lied.
func TestTheMessageStatesTheWeeksNumbers(t *testing.T) {
	body := MailBody(mailFixture(), "https://crm.example.test")

	for _, want := range []string{
		"7 of 9", // promised, delivered
		"1 won · 1 lost · 3 moved",
		"4 yes · 2 no", // proposals decided
		"6 acted · 3 dismissed",
		"Carried into Monday:  2",
		"Stahlbau Krämer",
		"12500.00 EUR",
		"2026-06-03",
		"https://crm.example.test",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the weekly message does not state %q:\n%s", want, body)
		}
	}
}

// The subject names the week. These land weekly into a mailbox that already
// holds last week's, and two identical subjects are two messages a reader
// cannot tell apart in a list.
func TestTheSubjectNamesTheWeek(t *testing.T) {
	got := MailSubject(mailFixture())
	if !strings.Contains(got, "1 June 2026") {
		t.Errorf("the subject does not name the week: %q", got)
	}
}

// An installation with no canonical origin mails the week without a link,
// never a link built on an empty base. An unusable URL in a message whose only
// call to action it is would be worse than no line at all.
func TestNoBaseURLMeansNoLinkRatherThanABrokenOne(t *testing.T) {
	body := MailBody(mailFixture(), "")
	if strings.Contains(body, "http") {
		t.Errorf("the message carries a link with no base configured:\n%s", body)
	}
	if !strings.Contains(body, "7 of 9") {
		t.Error("the message lost its numbers along with its link")
	}
}

// A deal label is the one field in this body that carries text somebody
// outside the installation may have chosen. Newlines in it must not be able to
// forge structure — a label holding "\n\nFrom: " would otherwise read as a
// header block to anything parsing the message.
func TestADealLabelCannotForgeStructureInTheBody(t *testing.T) {
	review := mailFixture()
	review.Deals[0].Label = "Krämer\r\nFrom: attacker@example.test\n\nYour week of forever"

	body := MailBody(review, "")
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "From:") {
			t.Fatalf("a deal label forged a header line:\n%s", body)
		}
	}
	if !strings.Contains(body, "Krämer From: attacker@example.test") {
		t.Errorf("the label was not flattened onto its own line:\n%s", body)
	}
}

// A week with many deals says how many it did not list. A message that quietly
// stopped at ten would read as a complete week that happened to have ten.
func TestALongWeekSaysWhatItDidNotList(t *testing.T) {
	review := mailFixture()
	// The fixture already carries one, so this brings the week to cap+4.
	for i := 0; i < mailDealCap+3; i++ {
		review.Deals = append(review.Deals, DealLine{
			Label: "Deal", Outcome: OutcomeMoved, OccurredAt: review.LocalWeekStart,
		})
	}

	body := MailBody(review, "")
	if !strings.Contains(body, "… and 4 more, on Home") {
		t.Errorf("a capped week does not say what it left out:\n%s", body)
	}
	if got := strings.Count(body, "  · "); got != mailDealCap {
		t.Errorf("the message listed %d deals, the cap is %d", got, mailDealCap)
	}
}
