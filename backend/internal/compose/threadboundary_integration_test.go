// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
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

// A conversation the owner held stays held when a reply changes its subject. A new
// conversation starts pending, which a classifier may later clear; the owner's decision
// is carried so that it cannot.
func TestAHeldConversationStaysHeldUnderANewTopic(t *testing.T) {
	e := integration.Setup(t)
	addr := angebotThread(t, e)
	owner := e.As(e.Rep1, nil, integration.AccountRepPerms)
	if err := database.WithWorkspaceTx(owner, e.Pool, func(tx pgx.Tx) error {
		return capture.NewThreadVerdictStore(e.DB()).DecideAsOwner(owner, tx, "opener@counterparty.example", false)
	}); err != nil {
		t.Fatalf("holding the conversation: %v", err)
	}
	captureThreadMail(t, e, e.Rep1, subjectMail("Re: Privat", addr, "private@counterparty.example", "Wed, 03 Jun 2026 08:00:00 +0000",
		"answer@counterparty.example", "opener@counterparty.example", "answer@counterparty.example"))

	if got := threadKeyOfMessage(t, e, "private@counterparty.example"); got != "private@counterparty.example" {
		t.Fatalf("the reply is filed under %q, want its own conversation", got)
	}
	if got := e.WsScalar(t, `SELECT coalesce(max(status), 'none') FROM capture_thread_verdict
		WHERE thread_key = 'private@counterparty.example' AND user_id = $1`, e.Rep1); got != "held_by_owner" {
		t.Errorf("the split conversation's verdict is %q, want the owner's held_by_owner carried over", got)
	}
}

// A sender who names their reply after another conversation's key does not land in it:
// the split refuses a key some other message carries and the reply stays with its parent.
func TestASplitNeverTakesAKeyAnotherConversationCarries(t *testing.T) {
	e := integration.Setup(t)
	addr := angebotThread(t, e)
	// A conversation keyed on a root this seat never received.
	captureThreadMail(t, e, e.Rep1, subjectMail("Re: Vertrag", addr, "member@counterparty.example", "Tue, 02 Jun 2026 09:00:00 +0000",
		"ghost-root@counterparty.example", "ghost-root@counterparty.example"))
	captureThreadMail(t, e, e.Rep1, subjectMail("Re: Something else", addr, "ghost-root@counterparty.example", "Wed, 03 Jun 2026 08:00:00 +0000",
		"answer@counterparty.example", "opener@counterparty.example", "answer@counterparty.example"))

	if got := threadKeyOfMessage(t, e, "ghost-root@counterparty.example"); got != "opener@counterparty.example" {
		t.Errorf("the reply naming another conversation's key is filed under %q, want its parent's conversation", got)
	}
}
