// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package comms

// One staged outbound message, as a row: what it is, as distinct from how the
// store reads and writes it (store.go).

import (
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Delivery is one staged message as the dispatcher sees it, in EITHER shape:
// the row is mail-shaped or channel-shaped and never half of each
// (comms_outbound_shape, 0155).
//
// On a channel delivery the mail fields read as their ZERO VALUES rather than as
// pointers, because a channel genuinely has no subject and no RFC822 identity —
// there is no second fact for a nil to carry that an empty string does not
// already say. ChannelUserID is the one exception, and its own comment says why.
// IsChannel is the ONE spelling of "which shape is this".
type Delivery struct {
	ID         ids.UUID
	ActivityID ids.ActivityID
	// UserID is the human whose mailbox sends this. ZERO for a controller
	// message, which is not sent BY anybody: the installation writes in its own
	// name, so there is no seat to check and no mailbox grant to intersect.
	UserID ids.UserID
	// SenderKind is "user" or "controller", and it decides which gates apply.
	// See senderProfile: a controller row has no seat, no OAuth scope and no
	// attachments, so asking those questions about it would refuse every
	// message for the absence of things it was never meant to have.
	SenderKind string
	// TemplateKey and TemplateVersion name the registered wording a controller
	// message was rendered from. A controller send carries no caller-supplied
	// subject or body.
	TemplateKey     string
	TemplateVersion int
	// PayloadRef points at the one-time link material in the key vault. The
	// transport that fetches it at dispatch, renders it into the body in memory
	// and retires it afterwards lands with the controller relay; what holds
	// today is that Art. 17 destroys the material and clears this reference.
	PayloadRef       string
	PayloadExpiresAt *time.Time
	// LinkID is the confirm_token this message carries, when it carries one.
	LinkID     ids.UUID
	Provider   string
	MessageID  string
	Recipients []string
	Cc         []string
	Subject    string
	Body       string
	// Bcc receives the message and appears in no header.
	Bcc []string
	// HTMLBody is the markup alternative, empty for a plain-text send.
	HTMLBody string
	// FromName is the sender's display name; empty sends a bare address.
	FromName string
	// Attachments is what this message was staged to carry. Empty for the
	// overwhelming majority of deliveries; non-empty is what the carriage gate
	// asks the channel about before anything reaches the wire.
	Attachments []OutboundFile
	// ChannelUserID is the recipient's account id at the provider for a channel
	// delivery, and NIL for a mail one.
	//
	// A pointer because NULL and empty are DIFFERENT facts on this column. NULL
	// is the row declaring itself mail-shaped; empty is a channel delivery whose
	// recipient a privacy scrub removed, emptying it exactly as it empties
	// mail's address lists (privacy/deliveries.go) — the column is also the
	// shape discriminator, so nulling it there would re-declare the row as mail
	// with every mail column missing. Collapsed into one string, a scrubbed
	// channel delivery would read as mail addressed to nobody.
	ChannelUserID  *string
	ConsentPurpose string
	// InReplyTo names the message this one replies to, in the vocabulary of its
	// own transport: an unbracketed RFC822 identity for mail, the provider's own
	// message id for a channel. Empty starts an unanchored message either way.
	InReplyTo       string
	References      []string
	ListUnsubscribe string
	// InFlightAt is when a PREVIOUS attempt handed this delivery to the provider
	// without recording what came back — nil when none did. It is set only on
	// the shapes whose retries cannot detect a prior send
	// (comms_outbound_inflight_is_channel, 0150), and reading it non-nil is what
	// makes the difference between an unsent message and a second copy of one
	// the customer already has.
	InFlightAt *time.Time
	Status     string
	Attempts   int
	CreatedAt  time.Time
}

// IsChannel reports whether this delivery leaves through a messaging channel
// rather than mail. It reads the row's shape discriminator, which the schema
// guarantees is present for exactly the channel-shaped rows.
func (d Delivery) IsChannel() bool { return d.ChannelUserID != nil }

// ChannelRecipient is the account id to deliver to: empty for a mail delivery,
// and empty for a channel delivery a privacy scrub has emptied — which no gate
// accepts as a recipient, so an erased subject cannot be messaged by a delivery
// that outlived them.
func (d Delivery) ChannelRecipient() string {
	if d.ChannelUserID == nil {
		return ""
	}
	return *d.ChannelUserID
}

// SenderController and SenderUser are the two kinds of outbound sender.
const (
	// SenderUser is a message from a human's own mailbox.
	SenderUser = "user"
	// SenderController is a message from the installation itself.
	SenderController = "controller"
)

// IsController reports whether this delivery is the installation writing in its
// own name rather than a person writing from their mailbox.
func (d Delivery) IsController() bool { return d.SenderKind == SenderController }
