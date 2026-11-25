// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package connector

import (
	"context"
	"time"
)

// What a chat network says about a message it already delivered. A network
// reports a change with the ChatBinding and message ids it filed the message
// under. One core function then acts on it for every network.

// ChatChangeKind says what happened to a delivered chat message.
type ChatChangeKind string

const (
	// ChatEdited is new text for a message.
	ChatEdited ChatChangeKind = "edited"
	// ChatDeleted is the provider's report that the message was removed.
	ChatDeleted ChatChangeKind = "deleted"
	// ChatDeliveryReported is a delivery status (sent, delivered, read, failed).
	// It says something about the transport, never about the conversation.
	ChatDeliveryReported ChatChangeKind = "delivery"
)

// ChatDeliveryStatus is what the provider reported about a message's delivery.
type ChatDeliveryStatus string

const (
	// ChatStatusSent is accepted by the provider for delivery.
	ChatStatusSent ChatDeliveryStatus = "sent"
	// ChatStatusDelivered reached the recipient's device.
	ChatStatusDelivered ChatDeliveryStatus = "delivered"
	// ChatStatusRead was opened by the recipient.
	ChatStatusRead ChatDeliveryStatus = "read"
	// ChatStatusFailed was not delivered.
	ChatStatusFailed ChatDeliveryStatus = "failed"
)

// ChatChange is one provider report about a message already delivered. It
// names the message the way ChatMessage does, so the same key reaches the same
// stored record.
type ChatChange struct {
	Kind ChatChangeKind
	// ConversationID, IdempotencyKey and Keys name the message as ChatMessage does.
	ConversationID string
	IdempotencyKey string
	Keys           *ChatOwnKeys

	// Body and Revision are an edit's new text and the provider's own edit
	// time. The revision orders edits: an edit no newer than the stored one
	// changes nothing, whatever order the reports arrive in.
	Body     string
	Revision time.Time
	// Restated is the whole message as the edit report carries it, when the
	// provider restates it. It files the message when the edit arrives before
	// the original did. A provider that reports only the new text leaves it nil.
	Restated *ChatMessage

	// Status and StatusAt are a delivery report's state and time.
	Status   ChatDeliveryStatus
	StatusAt time.Time
}

// ChatChangeOutcome says what a change did.
type ChatChangeOutcome string

const (
	// ChatChangeApplied is an edit written to the stored message.
	ChatChangeApplied ChatChangeOutcome = "applied"
	// ChatChangeStale is an edit no newer than the stored text, or a change to
	// a message already removed. A redelivered change lands here.
	ChatChangeStale ChatChangeOutcome = "stale"
	// ChatChangeNotFiled is an edit for a message that has not landed and that
	// the report could not restate. The caller retries it after the original.
	ChatChangeNotFiled ChatChangeOutcome = "not_filed"
	// ChatChangeRemoved is a deletion that destroyed the stored content.
	ChatChangeRemoved ChatChangeOutcome = "removed"
	// ChatChangeKept is a deletion the stored message outlives: the account is
	// a shared one, a colleague holds a copy, or the law requires keeping it.
	ChatChangeKept ChatChangeOutcome = "kept"
	// ChatChangeTombstoned is a deletion for a message not filed yet. A late
	// original is refused.
	ChatChangeTombstoned ChatChangeOutcome = "tombstoned"
	// ChatChangeNoted is a delivery status. It writes nothing to the
	// conversation.
	ChatChangeNoted ChatChangeOutcome = "noted"
)

// ChatChanger is the Sink's verb for a delivered chat message that changed.
// The binding has been checked, and the context carries the acting member and
// seat, as for filing.
type ChatChanger interface {
	ChangeChat(ctx context.Context, key NaturalKey, change ChatChange) (ChatChangeOutcome, error)
}
