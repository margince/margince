// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The anchor's subject is "Pricing question". A reply whose subject shares no
// topic word with it starts its own conversation, as capture splits mailbox
// copies. A shared word, or no topic at all, keeps the reply in it.
func TestAReplyOnANewTopicStartsItsOwnConversation(t *testing.T) {
	for name, c := range map[string]struct {
		subject string
		splits  bool
	}{
		"a new topic":         {"Invoice 4711 overdue", true},
		"the same topic":      {"Re: pricing", false},
		"a bare reply prefix": {"Re:", false},
	} {
		t.Run(name, func(t *testing.T) {
			e := setupSend(t)
			anchor := e.seedAnchor(t, "parent@buyer.test", "root@buyer.test")
			stager := &recordingStager{}
			in := sendInput("transactional")
			in.Subject = c.subject

			sent, err := e.store(stubUnsubscribeLinker{}).SendEmail(
				e.as(principal.RowScopeAll), FromActivity(anchor), in, stubConsentGate{}, stager)
			if err != nil {
				t.Fatalf("SendEmail: %v", err)
			}

			staged := stager.only(t)
			stored := e.storedThreadKey(t, ids.UUID(sent.Id))
			if !c.splits {
				if staged.ThreadKey != "root@buyer.test" || stored != "root@buyer.test" || staged.SplitFromThread != "" {
					t.Fatalf("staged key %q, stored key %q, split from %q; want the anchor's conversation kept",
						staged.ThreadKey, stored, staged.SplitFromThread)
				}
				return
			}
			if staged.ThreadKey != staged.MessageID || stored != staged.MessageID {
				t.Fatalf("staged key %q, stored key %q; want the reply's own identity %q", staged.ThreadKey, stored, staged.MessageID)
			}
			// No header may name the old conversation, or a provider and the
			// recipient's mail client would file the reply back into it.
			if staged.InReplyTo != "" || len(staged.References) != 0 {
				t.Fatalf("In-Reply-To %q / References %v, want both empty", staged.InReplyTo, staged.References)
			}
			if staged.SplitFromThread != "root@buyer.test" {
				t.Fatalf("split from %q, want the anchor's conversation, so its verdicts are carried", staged.SplitFromThread)
			}
			if staged.AnswersActivity != anchor.UUID {
				t.Fatalf("the delivery names %v as the message it answers, want the anchor %v", staged.AnswersActivity, anchor)
			}
		})
	}
}

// A reply to a calendar entry answers no mail, so it starts a conversation
// rather than splitting one, whatever the subject: nothing is carried over.
func TestAReplyToANonMailAnchorSplitsNothing(t *testing.T) {
	e := setupSend(t)
	anchor := e.seedNonMailAnchor(t, "event-17@calendar.test")
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE activity SET thread_key = 'event-17@calendar.test' WHERE id = $1`, anchor); err != nil {
		t.Fatalf("filing the calendar entry under a key: %v", err)
	}
	stager := &recordingStager{}
	in := sendInput("transactional")
	in.Subject = "Invoice 4711 overdue"

	if _, err := e.store(stubUnsubscribeLinker{}).SendEmail(
		e.as(principal.RowScopeAll), FromActivity(anchor), in, stubConsentGate{}, stager); err != nil {
		t.Fatalf("SendEmail: %v", err)
	}
	staged := stager.only(t)
	if staged.SplitFromThread != "" || staged.ThreadKey != staged.MessageID {
		t.Fatalf("split from %q under key %q, want a fresh conversation keyed %q", staged.SplitFromThread, staged.ThreadKey, staged.MessageID)
	}
	if staged.AnswersActivity != (ids.UUID{}) {
		t.Fatalf("the delivery names %v as the message it answers, want none for a calendar entry", staged.AnswersActivity)
	}
}

// A conversation a seat has not judged yet keeps a reply on a new topic. The
// pending question names the old message, so the reply waits with it.
func TestAReplyStaysInAConversationStillAwaitingItsVerdict(t *testing.T) {
	e := setupSend(t)
	anchor := e.seedAnchor(t, "parent@buyer.test", "root@buyer.test")
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO capture_thread_verdict (thread_key, user_id, status) VALUES ('root@buyer.test', $1, 'pending')`, e.rep); err != nil {
		t.Fatalf("seeding the pending verdict: %v", err)
	}
	stager := &recordingStager{}
	in := sendInput("transactional")
	in.Subject = "Invoice 4711 overdue"

	if _, err := e.store(stubUnsubscribeLinker{}).SendEmail(
		e.as(principal.RowScopeAll), FromActivity(anchor), in, stubConsentGate{}, stager); err != nil {
		t.Fatalf("SendEmail: %v", err)
	}
	if staged := stager.only(t); staged.ThreadKey != "root@buyer.test" || staged.SplitFromThread != "" {
		t.Fatalf("filed under %q, split from %q; want the pending conversation kept", staged.ThreadKey, staged.SplitFromThread)
	}
}
