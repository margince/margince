// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package whatsapp

import "time"

// Provider names WhatsApp Business in channel_connection.provider and in
// raw_capture.source_system.
const Provider = "whatsapp"

// Direction says which side of the conversation a message came from.
type Direction string

const (
	// Inbound is a customer's message, from the `messages` webhook field.
	Inbound Direction = "inbound"
	// Outbound is a message the business sent from the WhatsApp Business app,
	// from the `smb_message_echoes` field.
	Outbound Direction = "outbound"
)

// Kind classifies a message by what a capture path can do with it.
type Kind string

const (
	// KindText carries Text.
	KindText Kind = typeText
	// KindMedia carries Media (image, video, audio, document, sticker).
	KindMedia Kind = "media"
	// KindReaction is an emoji reaction (Text) to the message ReplyToID.
	KindReaction Kind = "reaction"
	// KindEdit replaces the message OriginalID with the content in Text/Media.
	KindEdit Kind = "edit"
	// KindRevoke deletes the message OriginalID.
	KindRevoke Kind = "revoke"
	// KindPlaceholder is a message type with no body this layer reads
	// (location, shared contacts, unknown or unsupported types). It still has
	// an id, time and direction, so History can show that something was sent.
	KindPlaceholder Kind = "placeholder"
)

// Media references one attachment; the bytes come from Client.DownloadMedia.
type Media struct {
	// Type is image, video, audio, document or sticker.
	Type string
	ID   string
	// URL is the download address when the payload carries one (echoes do);
	// pass it to Client.DownloadURL.
	URL      string
	MimeType string
	// SHA256 is the checksum as the webhook states it.
	SHA256   string
	Caption  string
	Filename string
}

// Message is one chat message on either side of the conversation.
type Message struct {
	// WABAID is the WhatsApp Business Account id of the webhook entry.
	WABAID string
	// PhoneNumberID is the business phone number the message arrived on. It is
	// the key a connection is bound by.
	PhoneNumberID string
	// DisplayPhone is the business number in display form.
	DisplayPhone string
	// ID is Meta's message id (wamid.…).
	ID        string
	Direction Direction
	// CounterpartyPhone is the customer's phone-based wa_id without a leading
	// "+": `from` on an inbound message, `to` on an echo. Empty when the
	// customer uses a username and shares no phone number.
	CounterpartyPhone string
	// CounterpartyUserID is the customer's business-scoped user id (BSUID),
	// empty when Meta sent none. At least one of the two is set.
	CounterpartyUserID string
	// DisplayName is the customer's profile name; empty when Meta sent none.
	DisplayName string
	At          time.Time
	Kind        Kind
	// Type is Meta's own message type string, kept for placeholders.
	Type string
	Text string
	// Media is set for KindMedia.
	Media *Media
	// OriginalID is the message an edit or revoke refers to.
	OriginalID          string
	Forwarded           bool
	FrequentlyForwarded bool
	// ReplyToID is the id of the message this one replies or reacts to.
	ReplyToID string
	// ErrorCodes lists codes Meta attached to the message (unsupported types).
	ErrorCodes []int
}

// Delivery states a Status can carry.
const (
	StatusSent      = "sent"
	StatusDelivered = "delivered"
	StatusRead      = "read"
	StatusFailed    = "failed"
	StatusDeleted   = "deleted"
)

// Status is a delivery receipt for a message the business sent.
type Status struct {
	PhoneNumberID string
	MessageID     string
	// State is one of the Status* constants, or Meta's string if it adds one.
	State       string
	RecipientID string
	At          time.Time
	ErrorCodes  []int
}

// ItemError reports one payload item that could not be decoded; the rest of
// the batch is unaffected.
type ItemError struct {
	// Path locates the item, e.g. "entry[1].changes[0].value.messages[2]".
	Path string
	Err  error
}

// Result is everything one webhook POST carried.
type Result struct {
	Messages []Message
	Statuses []Status
	// UnhandledFields lists change fields this layer does not decode
	// (smb_app_state_sync, history, account_update, template updates, …). They
	// are acknowledged, never failed, so Meta does not retry them.
	UnhandledFields []string
	Errors          []ItemError
}
