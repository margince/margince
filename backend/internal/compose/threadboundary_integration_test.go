// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
)

func subjectMail(subject, to, messageID, at, inReplyTo string, references ...string) []byte {
	return []byte(strings.Replace(string(threadMail(to, messageID, at, inReplyTo, references...)),
		"Subject: Re: Angebot", "Subject: "+subject, 1))
}

// angebotThread captures an opener and one reply on the same topic for owner.
func angebotThread(t *testing.T, e *integration.Env) string {
	t.Helper()
	addr := seatAddress(t, e, e.Rep1)
	captureThreadMail(t, e, e.Rep1, subjectMail("Angebot", addr, "opener@counterparty.example", "Mon, 01 Jun 2026 08:00:00 +0000", ""))
	captureThreadMail(t, e, e.Rep1, subjectMail("Re: Angebot", addr, "answer@counterparty.example", "Tue, 02 Jun 2026 08:00:00 +0000",
		"opener@counterparty.example", "opener@counterparty.example"))
	return addr
}

func TestAReplyOnANewTopicOpensItsOwnConversation(t *testing.T) {
	e := integration.Setup(t)
	addr := angebotThread(t, e)
	captureThreadMail(t, e, e.Rep1, subjectMail("Re: Rechnung März", addr, "invoice@counterparty.example", "Wed, 03 Jun 2026 08:00:00 +0000",
		"answer@counterparty.example", "opener@counterparty.example", "answer@counterparty.example"))

	if got := threadKeyOfMessage(t, e, "invoice@counterparty.example"); got != "invoice@counterparty.example" {
		t.Errorf("the reply on a new topic is filed under %q, want a conversation of its own", got)
	}
	for _, id := range []string{"opener@counterparty.example", "answer@counterparty.example"} {
		if got := threadKeyOfMessage(t, e, id); got != "opener@counterparty.example" {
			t.Errorf("%s moved to %q; the conversation the reply left must stay where it was", id, got)
		}
	}
}

func TestAReplyToTheNewTopicFollowsItWithoutRejoiningTheOldOne(t *testing.T) {
	e := integration.Setup(t)
	addr := angebotThread(t, e)
	captureThreadMail(t, e, e.Rep1, subjectMail("Rechnung März", addr, "invoice@counterparty.example", "Wed, 03 Jun 2026 08:00:00 +0000",
		"answer@counterparty.example", "opener@counterparty.example", "answer@counterparty.example"))
	captureThreadMail(t, e, e.Rep1, subjectMail("RE: Rechnung  März", addr, "paid@counterparty.example", "Thu, 04 Jun 2026 08:00:00 +0000",
		"invoice@counterparty.example", "opener@counterparty.example", "answer@counterparty.example", "invoice@counterparty.example"))

	if got := threadKeyOfMessage(t, e, "paid@counterparty.example"); got != "invoice@counterparty.example" {
		t.Errorf("the reply to the new topic is filed under %q, want the new topic's conversation", got)
	}
	if got := threadKeyOfMessage(t, e, "opener@counterparty.example"); got != "opener@counterparty.example" {
		t.Errorf("the old conversation was merged into %q by a reply that references it", got)
	}
}

func TestABareReplyPrefixDrawsNoBoundary(t *testing.T) {
	e := integration.Setup(t)
	addr := angebotThread(t, e)
	captureThreadMail(t, e, e.Rep1, subjectMail("Re:", addr, "blank@counterparty.example", "Wed, 03 Jun 2026 08:00:00 +0000",
		"answer@counterparty.example", "opener@counterparty.example", "answer@counterparty.example"))

	if got := threadKeyOfMessage(t, e, "blank@counterparty.example"); got != "opener@counterparty.example" {
		t.Errorf("a reply with no topic of its own is filed under %q, want its parent's conversation", got)
	}
}

// The parent is named by the sender's own header, so a message the seat never held
// cannot split or steer the reply: it keeps the root its header gives it.
func TestAParentTheSeatDoesNotHoldDecidesNothing(t *testing.T) {
	e := integration.Setup(t)
	captureThreadMail(t, e, e.Rep2, subjectMail("Angebot", seatAddress(t, e, e.Rep2), "foreign@counterparty.example", "Mon, 01 Jun 2026 08:00:00 +0000", ""))
	captureThreadMail(t, e, e.Rep1, subjectMail("Re: Rechnung März", seatAddress(t, e, e.Rep1), "claimed@counterparty.example", "Tue, 02 Jun 2026 08:00:00 +0000",
		"foreign@counterparty.example", "foreign@counterparty.example"))

	if got := threadKeyOfMessage(t, e, "claimed@counterparty.example"); got != "foreign@counterparty.example" {
		t.Errorf("a parent only another seat holds moved the reply to %q, want the root its header names", got)
	}
}
