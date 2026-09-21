// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package connector

// The OUTBOUND half of the connector seam: what a provider needs to transmit a
// message as the connected user, and what it reports back.
//
// Apart from the capture side because the two are optional in different ways —
// a capture-only provider implements none of this, and the frozen Connector
// interface names none of it — and because the message shape below is the one
// place the wire format of an outbound mail is described.

import (
	"context"
	"errors"
	"strings"

	"github.com/margince/margince/backend/pkg/extension"
)

// EmailSender is the OPTIONAL outbound seam a connector implements when its
// provider can transmit a message as the connected user. Type-asserted like
// Watcher and Backfiller, so the frozen Connector interface is unchanged and a
// capture-only provider simply does not implement it.
//
// SendEmail MUST be idempotent on msg.MessageID: job delivery is at-least-once,
// so a provider that retransmits on a retry mails the recipient twice. A
// connector able to look up a prior send by Message-ID must do so whenever
// msg.Attempt > 0 and return the existing receipt.
//
// An implementation MUST refuse a message failing EmailMessage.Validate before
// any provider I/O: an identity the lookup cannot search for makes that
// guarantee unkeepable, and sending anyway is the double-send this prevents.
type EmailSender interface {
	SendEmail(ctx context.Context, auth Auth, msg EmailMessage) (SendReceipt, error)
}

// AttachmentCarrier is how a sending connector declares whether it can transmit
// files.
//
// THERE IS NO DEFAULT, and that is the whole design. A files field the adapters
// may ignore compiles everywhere and silently sends the covering text without
// the file: the sender sees an attachment chip, because the timeline records
// what was STAGED, the recipient sees a reference to a file that is not there,
// and nobody is told.
//
// So a sender not implementing this carries nothing, and a staged message with
// files PARKS rather than going out stripped.
type AttachmentCarrier interface {
	// Carriage reports what this connector's provider can carry. A connector
	// that does not implement this interface carries nothing.
	Carriage() Carriage
}

// Carriage is the published capability descriptor, aliased for the same reason
// the file types are: a unit and a core connector must answer this question with
// the same type, or the bounds a gate checks are two sets that can disagree.
type Carriage = extension.Carriage

// CarriageOf asks a resolved sender what it can carry. A sender not implementing
// AttachmentCarrier answers the ZERO Carriage — the no-default rule in one line,
// beside the interface because the send seam, the registry and their tests all
// ask it, and a second spelling is a second place "presumably it carries" creeps
// in.
//
//craft:ignore naked-any the type assertion seam: a sender is whichever connector the resolver or the registry bound
func CarriageOf(sender any) Carriage {
	carrier, ok := sender.(AttachmentCarrier)
	if !ok {
		return Carriage{}
	}
	return carrier.Carriage()
}

// OutboundFile is one file to transmit — the published extension.OutboundFile,
// aliased for the reason part.go states.
type OutboundFile = extension.OutboundFile

// EmailMessage is one message to transmit, in provider-NEUTRAL form. The
// connector owns the wire encoding — Gmail takes base64url RFC822, Graph takes
// JSON — so no caller ever builds MIME. It is the mirror of Normalize, which
// owns decoding on the way in.
type EmailMessage struct {
	To []string
	Cc []string
	// Bcc receives the message and is rendered into NO header.
	//
	// The distinction lives here rather than in the renderer because it is a
	// fact about the addressees, not about one wire format: a provider that
	// takes an addressee list separately from the message (Graph, SES) needs
	// the same separation, and a renderer that had to infer it would have to
	// be told twice.
	Bcc []string
	// FromName is the sender's display name, or empty to send a bare address.
	//
	// A From header with no display name shows the address's LOCAL PART in every
	// mail client, so a message from lars@gradion.com arrives from "lars". The
	// address itself is the connector's own (the connected mailbox); this is the
	// human the CRM records as having sent it.
	FromName string

	Subject string
	Body    string // text/plain; always present, and the only part a text client reads

	// HTMLBody is the same message as markup, or empty for a plain-text send.
	//
	// It never REPLACES Body. A message carrying both goes out as
	// multipart/alternative with the plain part first, which is what lets a
	// client that cannot or will not render HTML — a screen reader, a terminal
	// client, a spam filter reading the cheapest part — still receive the words.
	// A mail with markup and no plain alternative is the shape that arrives as
	// a blank message.
	HTMLBody string

	// MessageID is the RFC822 message identity WITHOUT angle brackets —
	// "abc@host", never "<abc@host>". Stored and compared in this form because
	// that is how mail parsing yields it, so the copy the provider files back
	// into the mailbox carries a key that matches the one recorded at send.
	// The connector adds the brackets when it renders the header.
	MessageID string

	// InReplyTo threads onto an existing conversation, also unbracketed. Empty
	// starts a new thread.
	InReplyTo string

	// References is the unbracketed ancestry chain, oldest first.
	References []string

	// ListUnsubscribe and ListUnsubscribePost carry the RFC 8058 header pair for
	// a marketing send; both empty for a transactional purpose, which has nothing
	// to unsubscribe from.
	ListUnsubscribe     string
	ListUnsubscribePost string

	// Attempt is 0 on the first transmission and increments on every retry. It is
	// how a connector knows to run the prior-send lookup the contract requires.
	Attempt int

	// The attachments this message carries. A connector handed a non-empty set
	// has already been asked whether it carries them, so reaching here means
	// transmitting them.
	//
	// THE INVARIANT: no adapter may transmit a message whose attachment set
	// differs from the one it was handed — not a subset, not converted to links,
	// not dropped. If it cannot send all of them it errors and the delivery parks.
	Files []OutboundFile
}

// ErrInvalidMessageID marks an outbound message carrying no usable RFC822
// identity. It is the idempotency contract failing its precondition: Send is
// required to be idempotent on MessageID, and an identity the provider's
// prior-send lookup cannot search for makes that guarantee unkeepable. A
// message sent under one would mail its recipient again on every retry, and
// the copy the provider files back would key onto no activity.
var ErrInvalidMessageID = errors.New("connector: outbound message carries no usable RFC822 message identity")

// maxMessageIDLen bounds a message identity at a length a header can carry. RFC
// 5322 caps a header line at 998 octets and a References chain holds several
// identities at once, so the usable ceiling is well below that; 512 is already
// an order of magnitude above what providers mint. The bound matters because an
// identity is READ BACK from a provider response and adopted as a natural key,
// a thread key and a log field — unbounded, it is a remote party choosing how
// many bytes this installation stores per sent message.
const maxMessageIDLen = 512

// ValidMessageID reports whether id is a usable RFC822 identity in the
// UNBRACKETED form this system stores: an addr-spec with exactly one '@', both
// sides non-empty, no whitespace, brackets or ASCII control character, and no
// longer than a header line can carry. Control characters are rejected wholesale
// because any of them renders a malformed header, and a provider that mangles
// one breaks the retry path's rfc822msgid: lookup — the search that stops a
// redelivery mailing the recipient twice.
//
// The ONE spelling of that question, so the identity a send transmits under, the
// one a threading header derives from, and the one a provider reports back
// cannot disagree.
func ValidMessageID(id string) bool {
	if len(id) > maxMessageIDLen {
		return false
	}
	local, domain, found := strings.Cut(id, "@")
	if !found || local == "" || domain == "" || strings.Contains(domain, "@") {
		return false
	}
	for _, r := range id {
		switch {
		case r == ' ' || r == '<' || r == '>':
			return false
		case r <= 0x1F || r == 0x7F: // the full ASCII control range (C0 + DEL)
			return false
		}
	}
	return true
}

// Validate refuses a message no provider should be handed. It is the sender
// boundary's own precondition — checked before any provider I/O, so a message
// that cannot be retried safely is never transmitted a first time.
func (m EmailMessage) Validate() error {
	if !ValidMessageID(m.MessageID) {
		return ErrInvalidMessageID
	}
	return nil
}

// SendReceipt is what the provider confirmed: its own message identity, and the
// RFC822 identity the transmitted copy actually carries.
//
// The provider's CONVERSATION id is deliberately absent. This system threads on
// the RFC822 identity, which is what capture keys reply detection on; a
// provider's conversation id lives in another namespace, joins nothing here, and
// would invite a reader to key on a value no query reads.
//
// A Message-ID is a REQUEST, not a guarantee — Gmail discards the client's and
// mints its own — so the identity recorded has to be the one the wire carries.
type SendReceipt struct {
	ProviderMessageID string
	// The unbracketed Message-ID on the transmitted copy.
	//
	// EMPTY covers two facts, deliberately not distinguished here: no re-key is
	// OWED (the provider honoured the identity, or reports none), or none is
	// POSSIBLE (the read-back went unanswered). The second is a degradation — on
	// a provider that rewrites, the message stays keyed on an identity the wire
	// does not carry and its captured echo lands as a second timeline row.
	//
	// It stays a plain identity because a receipt reporting "unknown" would ask
	// every caller to carry a recovery path that is not theirs. ProviderMessageID
	// is durable on the delivery, so a later pass can re-ask the provider.
	RFC822MessageID string
}
