// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package connector

import (
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
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
	// SeatID is the capture_connection seat the account files through. Its
	// posture decides who reads a message, as a mailbox's does for mail. Zero
	// for the workspace's own bot, which belongs to no account.
	SeatID ids.UUID
	// SourceSystem is the natural key's source system when it is not the
	// provider. An extension unit's records are keyed under the source the unit
	// declared, so its messages and its other records share one namespace. Empty
	// means Provider.
	SourceSystem string
	// PinnedPosture is the posture a member-owned source declared for everything
	// it files: PostureClassified or PostureHeld, never shared. A seat answers
	// first. The pin answers for a source with no seat. Empty holds.
	PinnedPosture string
}

// The postures a member-owned chat source may pin. A pin may only narrow who
// reads a message, so shared is not offered.
const (
	ChatPostureClassified = "classified"
	ChatPostureHeld       = "held"
)

// ChatLink is what the capture side reads from the approved row of a business
// chat account when it files a stored message. It names the account, whose it is
// and the seat it files through.
type ChatLink struct {
	// AccountID is the provider's stable id of the account.
	AccountID   string
	Ownership   ChatOwnership
	OwnerUserID ids.UUID
	SeatID      ids.UUID
}

// ChatParty is the contact on the other side of a conversation.
type ChatParty struct {
	// UserID is the provider's id for them. It is the identity, so it is never empty.
	UserID      string
	DisplayName string
	Username    string
	// Email is an address the provider vouches for this party. It corroborates
	// the account and never names the party. The capture ladder reads it only
	// when EmailMayMatch says the source declared the email merge key.
	Email string
	// Domain is Email's lower-cased domain, which the internal-domain gates key on.
	Domain        string
	EmailMayMatch bool
	// Alias is a second identity the provider named for the same human in this message.
	Alias *ChatAlias
}

// ChatAlias is one more identity of a ChatParty.
type ChatAlias struct {
	AccountScope string
	UserID       string
}

// ChatOwnKeys are keys a source persisted itself. They are used as given where
// an adapter would derive them from the connection's scopes. A source whose
// records exist keeps the keys they were stored under.
type ChatOwnKeys struct {
	// SourceID is the whole source id of the natural key.
	SourceID string
	// ThreadKey is the conversation key, namespaced by the source.
	ThreadKey string
	// SourceIDNamesAContact is true when SourceID embeds a human's account id,
	// so the pipeline trace stores a hash of it.
	SourceIDNamesAContact bool
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
	// Revision is the provider's edit time when this report already carries an
	// edited text, as a restated message does. It is zero for an original.
	Revision time.Time
	// StoredOriginal names the raw_capture row the adapter already wrote. Raw
	// carries the bytes when it has not.
	StoredOriginal ids.UUID
	Raw            []byte

	// Keys, when set, replaces the keys derived from the binding's scopes and
	// the ConversationID and IdempotencyKey above, which a message with Keys
	// need not carry.
	Keys *ChatOwnKeys
	// OccurredDay says OccurredAt is a date's anchor and not a moment. The
	// provider, or the member who filed the message by hand, knows the day only.
	OccurredDay bool
	// Subject is a subject line, for a source whose messages have one.
	Subject string
	// ProviderPrivate is the provider's own statement that the message is private.
	ProviderPrivate bool
	// Addresses lists the addresses the message names. Empty means the source
	// cannot enumerate its parties, which is never read as "internal".
	Addresses []string
	// Participants is the roster of a group conversation beyond the two ends.
	Participants []MessageParticipant
	// ChosenSubject is the contact or lead a member filed this message on. The
	// message is linked to it directly and its counterparty is not resolved by
	// identity. The caller has already checked the member may see it.
	ChosenSubject *datasource.EntityRef
	// Parts and PartDrops are the files the message carried and the ones the
	// source refused, already bounded and typed as the mail parser's are.
	Parts     []Part
	PartDrops []PartDrop
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
