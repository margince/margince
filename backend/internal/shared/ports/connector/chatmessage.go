// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package connector

import (
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// What every chat network hands the core, split by who may state it.
//
// A ChatBinding is a fact about the authenticated connection. The adapter reads
// it from a row it resolved itself, never from the message. A ChatMessage holds
// only what the provider said about one message. The capture sink derives the
// stored record and the audience from the two, so a later feature works for
// every network in one place.

// ChatOwnership says whose conversation a connection carries.
type ChatOwnership string

const (
	// ChatOwnedByWorkspace is a connection every colleague may read from, such
	// as the workspace's own bot.
	ChatOwnedByWorkspace ChatOwnership = "workspace"
	// ChatOwnedByMember is one member's own account. Its messages are that
	// member's correspondence until they share it.
	ChatOwnedByMember ChatOwnership = "member"
)

// ChatBinding is what the authenticated connection says about a message.
type ChatBinding struct {
	// Provider is the channel_provider the message travelled on.
	Provider string
	// CapturedBy is the connector principal's id, e.g. "connector:telegram".
	CapturedBy string
	// KeyScope and ThreadScope namespace the provider's chat and message ids.
	// Chat ids repeat across bots and accounts, so each connection names the
	// scope its ids are unique in. The sink joins scope, conversation and message
	// with colons. A scope is built from parts that hold no colon.
	KeyScope    string
	ThreadScope string
	// AccountScope is the account the counterparty identity was seen through.
	// It is empty when the provider's user ids are global.
	AccountScope string
	Ownership    ChatOwnership
	// OwnerUserID is the member behind a member-owned connection.
	OwnerUserID ids.UUID
}

// ChatParty is the contact on the other side of a conversation.
type ChatParty struct {
	// UserID is the provider's id for them. It is the identity, so it is never empty.
	UserID      string
	DisplayName string
	Username    string
}

// ChatMessage is one message as the provider stated it.
type ChatMessage struct {
	// ConversationID is the provider's own, unscoped.
	ConversationID string
	// IdempotencyKey is required and names this message within the conversation:
	// the provider's message id when it has one, else the caller's operation id.
	// The sink namespaces it by the connection, so a second filing of the same
	// key adds nothing.
	IdempotencyKey string
	// Direction is DirectionInbound or DirectionOutbound, from the connection owner's side.
	Direction    string
	Counterparty ChatParty
	Body         string
	// OccurredAt is when the provider says the message was sent.
	OccurredAt time.Time
	// StoredOriginal names the raw_capture row the adapter already wrote. Raw
	// carries the bytes when it has not.
	StoredOriginal ids.UUID
	Raw            []byte
}

// CaptureOnlyThreadScope is the thread scope of a conversation that is read and
// never answered through the workspace's own connector, such as a Telegram
// Business chat. Sending for that account needs the account's own connection.
// Its thread key is "<provider>:biz:<account>:<conversation>".
const CaptureOnlyThreadScope = "biz"

// IsCaptureOnlyThread reports whether a stored thread key names a conversation
// that is captured and never replied to from here.
func IsCaptureOnlyThread(provider, threadKey string) bool {
	return strings.HasPrefix(threadKey, provider+":"+CaptureOnlyThreadScope+":")
}
