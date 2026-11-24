// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// A reply on a new topic is filed under a conversation of its own, and names the
// message it answers. The contact wrote only into that message's conversation,
// too long ago for any arm but the thread arm. Transmit asks it, as staging did.
func TestTransmitAsksTheConversationAReplyOnANewTopicAnswers(t *testing.T) {
	e := setupResolve(t)
	anchor := e.inboundFrom(t, "their-question", e.address, longAgo(t))
	delivery := e.splitReplyDelivery(t, anchor)
	e.stageReply(t, delivery, anchor)
	if allowed, resolved := e.transmitReply(t, delivery); !allowed || resolved != commsauthz.CategoryReplyToInbound {
		t.Fatalf("transmit = allowed %v as %s, want the reply staging allowed", allowed, resolved)
	}
}

// A merge that renames the answered conversation before transmit does not strand
// the reply: transmit reads the answered message's current key.
func TestASplitReplyFollowsAMergeOfTheConversationItAnswers(t *testing.T) {
	e := setupResolve(t)
	anchor := e.inboundFrom(t, "their-question", e.address, longAgo(t))
	delivery := e.splitReplyDelivery(t, anchor)
	e.stageReply(t, delivery, anchor)
	e.renameConversation(t, "their-question", "merged-conversation")
	if allowed, resolved := e.transmitReply(t, delivery); !allowed || resolved != commsauthz.CategoryReplyToInbound {
		t.Fatalf("transmit = allowed %v as %s, want the reply on the merged conversation", allowed, resolved)
	}
}

// A reply that stays in its conversation follows a merge to transmit too.
func TestAReplyInItsOwnConversationFollowsAMergeToTransmit(t *testing.T) {
	e := setupResolve(t)
	anchor := e.inboundFrom(t, "their-question", e.address, longAgo(t))
	delivery := e.deliveryOnThread(t, "their-question")
	e.stageReply(t, delivery, anchor)
	e.renameConversation(t, "their-question", "merged-conversation")
	if allowed, resolved := e.transmitReply(t, delivery); !allowed || resolved != commsauthz.CategoryReplyToInbound {
		t.Fatalf("transmit = allowed %v as %s, want the reply on the merged conversation", allowed, resolved)
	}
}

// splitReplyDelivery is a delivery whose activity sits on its own key. It names
// the anchor it answers, as the send path writes a reply on a new topic.
func (e *resolveEnv) splitReplyDelivery(t *testing.T, anchor ids.UUID) ids.UUID {
	t.Helper()
	delivery := e.deliveryOnThread(t, "our-new-topic")
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE comms_outbound SET answers_activity_id = $2 WHERE id = $1`, delivery, anchor); err != nil {
		t.Fatalf("naming the answered message: %v", err)
	}
	return delivery
}

func (e *resolveEnv) renameConversation(t *testing.T, from, to string) {
	t.Helper()
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE activity SET thread_key = $2 WHERE thread_key = $1`, from, to); err != nil {
		t.Fatalf("renaming the conversation: %v", err)
	}
}

// longAgo is outside every reply window, so only the thread arm admits a reply.
func longAgo(t *testing.T) time.Time {
	t.Helper()
	return clocktest.Now(t).Add(-2 * 365 * 24 * time.Hour)
}

func (e *resolveEnv) stageReply(t *testing.T, delivery, anchor ids.UUID) {
	t.Helper()
	req := commsauthz.Request{
		AnchorActivityID: anchor, Recipients: []connector.Recipient{{Email: e.address}},
		Subject: "Invoice 4711", Body: "Attached.", AuthoredSubject: "Invoice 4711", AuthoredBody: "Attached.",
	}
	if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		set, err := e.gate.AuthorizeStagingTx(e.ctx, tx, delivery, req)
		if err == nil && set.Decisions[0].Verdict != commsauthz.VerdictAllow {
			t.Fatalf("staging = %s %q, want the reply allowed on the thread it answers",
				set.Decisions[0].Verdict, set.Decisions[0].ReasonCode)
		}
		return err
	}); err != nil {
		t.Fatalf("staging: %v", err)
	}
}

func (e *resolveEnv) transmitReply(t *testing.T, delivery ids.UUID) (bool, commsauthz.Category) {
	t.Helper()
	ticket, err := e.gate.AuthorizeTransmit(e.ctx, commsauthz.TransmitRequest{
		DeliveryID: delivery, Attempt: 1, Recipients: []connector.Recipient{{Email: e.address}}, Subject: "Invoice 4711", Body: "Attached.",
	})
	if err != nil {
		t.Fatalf("transmit: %v", err)
	}
	var resolved string
	if err := e.owner.QueryRow(context.Background(), `
		SELECT resolved_category FROM communication_decision
		 WHERE delivery_id = $1 AND phase = 'transmit'`, delivery).Scan(&resolved); err != nil {
		t.Fatalf("reading the transmit decision: %v", err)
	}
	return ticket.Allowed, commsauthz.Category(resolved)
}
