// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A chat message on a contact's History carries whose move it is, by the rules
// the mail badge follows. Theirs is owed a reply, ours waits for them, and a
// reply ends the wait.

import (
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// chat logs one chat message with the contact through the ordinary writer, on
// one conversation key per chat.
func (o *owedEnv) chat(t *testing.T, contact ids.UUID, direction string, at time.Time, chatKey string) ids.ActivityID {
	t.Helper()
	subject := "Chat with the buyer"
	logged, _, err := o.e.Activities.LogActivity(o.reader(), activities.LogActivityInput{
		Kind: "message", Direction: &direction, Subject: &subject, OccurredAt: &at, Source: "manual",
		ChannelProvider: "whatsapp", ThreadKey: chatKey,
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}},
	})
	if err != nil {
		t.Fatalf("logging the %s message: %v", direction, err)
	}
	return ids.From[ids.ActivityKind](ids.UUID(logged.Id))
}

func (o *owedEnv) messageMove(t *testing.T, message ids.ActivityID) crmcontracts.ActivityMessageMove {
	t.Helper()
	read, err := o.e.Activities.GetActivity(o.reader(), message, storekit.LiveOnly)
	if err != nil {
		t.Fatalf("reading the message: %v", err)
	}
	if read.MessageMove == nil {
		t.Fatal("the chat message carries no move")
	}
	return *read.MessageMove
}

func TestAChatMessageCarriesWhoseMoveItIs(t *testing.T) {
	o := setupOwed(t)
	buyer := o.contact(t, "Pat Buyer", "pat@customer.example")

	theirs := o.chat(t, buyer, "inbound", o.now.Add(-3*time.Hour), "chat-theirs")
	if got := o.messageMove(t, theirs); got != crmcontracts.ActivityMessageMoveNeedsReply {
		t.Errorf("their chat message reads %q, want needs_reply", got)
	}

	ours := o.chat(t, buyer, "outbound", o.now.Add(-2*time.Hour), "chat-ours")
	if got := o.messageMove(t, ours); got != crmcontracts.ActivityMessageMoveWaitingForThem {
		t.Errorf("our chat message reads %q, want waiting_for_them", got)
	}

	o.chat(t, buyer, "inbound", o.now.Add(-time.Hour), "chat-ours")
	if got := o.messageMove(t, ours); got != crmcontracts.ActivityMessageMoveNone {
		t.Errorf("our chat message reads %q after their reply, want none", got)
	}
}
