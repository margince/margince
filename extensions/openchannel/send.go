// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package openchannel

// The transport half: what it takes to post a message back to the address a
// member registered.
//
// WHOSE CREDENTIAL SIGNS is the whole shape of this file. A message leaves under
// the MEMBER's own signing secret — the person whose endpoint it is — never under
// an installation credential, because this connector has none: it holds one
// sealed secret per member and nothing else. That is also why Live exists: the
// core has to be able to ask "can this member still send" without spending the
// credential to find out.
//
// THERE IS NO QUEUE HERE AND NO RETRY LADDER. The product stages a delivery,
// bounds it, retries it on its own ten-rung ladder and parks it when the ladder
// runs out; all of that is upstream of this file. A second ladder here would be a
// second answer to one question, and the two would disagree about how many times
// a customer heard from us. So Send POSTS SYNCHRONOUSLY and reports one outcome.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/margince/margince/backend/pkg/extension"
)

// departure is the document this connector posts. It is `arrival` (record.go)
// minus what this side cannot honestly fill in: the member's own address is not
// something this unit holds, and the receiver already knows whose endpoint it
// configured.
type departure struct {
	// MessageID is the product's own delivery id. The signature nonce is
	// derived from it (deliveryNonce, client.go) rather than equal to it: this
	// id is a UUID and the nonce header is documented and enforced as hex, so
	// signing the id verbatim would be a nonce this connector's own edge
	// refuses. The derivation is deterministic, so a receiver implementing this
	// scheme still deduplicates a re-posted attempt — on the derived value it
	// replay-checks, not on this field.
	MessageID string `json:"message_id"`
	// Attempt counts from 1, so a receiver's log can tell a first try from a
	// repeat even where it does not deduplicate.
	Attempt int `json:"attempt"`
	// ReplyTo anchors the message on the receiver's own id for an earlier one,
	// empty for a message that answers nothing.
	ReplyTo    string    `json:"reply_to,omitempty"`
	OccurredAt time.Time `json:"occurred_at"`
	Body       string    `json:"body"`
	// To names the party the product resolved: their account id at this
	// connector's provider, never a handle, because a handle can be released and
	// re-claimed.
	To party `json:"to"`
}

// send posts one message to the member's registered address.
func send(ctx context.Context, rt extension.Runtime, msg extension.OutboundMessage) (extension.Receipt, error) {
	return sendVia(ctx, rt, msg, newSender, time.Now)
}

// sendVia is send with its two boundaries injected: the address it dials, and
// the clock the signature is stamped from.
//
// The clock is a seam because the timestamp is SIGNED and the receiver checks it
// for freshness — so what a test needs to assert is that a fixed instant produces
// the signature that instant's material implies, which a real clock makes
// impossible to state.
func sendVia(ctx context.Context, rt extension.Runtime, msg extension.OutboundMessage,
	dial senderFactory, now func() time.Time,
) (extension.Receipt, error) {
	if msg.IdempotencyKey == "" {
		// Nothing left, so this is a definite answer. The product stages every
		// delivery with an id of its own; without one there is no replay key to
		// sign, and inventing one would make each attempt a new message to the
		// receiver.
		return extension.Receipt{}, fmt.Errorf("%w: this delivery carries no id, so a re-posted attempt would reach the receiver as a second message", extension.ErrInvalid)
	}
	target, secret, err := sendable(ctx, rt, msg.Member)
	if err != nil {
		return extension.Receipt{}, err
	}
	// Called ONCE and reused for both the document and the signature: a receiver
	// compares the two, and a real clock would put a marshalling-and-dialling gap
	// between them where a fixed instant is what both sides must agree on.
	at := now()
	body, err := json.Marshal(departure{
		MessageID: msg.IdempotencyKey, Attempt: msg.Attempt, ReplyTo: msg.ReplyTo,
		OccurredAt: at, Body: msg.Body,
		To: party{Account: msg.Recipient.ChannelUserID},
	})
	if err != nil {
		return extension.Receipt{}, err
	}
	post, err := dial(target.URL)
	if err != nil {
		// The address grammar or the egress guard's own parse. Nothing left, so
		// it is a definite answer however it reads.
		return extension.Receipt{}, err
	}
	return transmit(ctx, rt, transmission{target: target, msg: msg, post: post, at: at}, secret, body)
}

// transmission is one attempt's subject, gathered so transmit takes an argument
// list a reader can hold: which endpoint, which delivery, where it is going, and
// the instant the signature is stamped from.
type transmission struct {
	target endpoint
	msg    extension.OutboundMessage
	post   *sender
	at     time.Time
}

// transmit records the attempt, posts, and records what came back.
//
// THE ROW IS WRITTEN BEFORE THE REQUEST LEAVES, as `unknown`, which is the same
// discipline the product's own send seam applies one level up. A row written
// afterwards would be a row that does not exist for exactly the case it is most
// needed in — a worker killed mid-flight — and the member would see a message
// that never left rather than one whose outcome nobody knows.
//
// A FAILURE TO RECORD THE OUTCOME AFTER A SUCCESSFUL POST IS REPORTED AS
// UNKNOWN, deliberately, even though this side saw the acceptance. The two
// available answers are "unknown", which STOPS the delivery with the uncertainty
// on the record, and "failed", which puts the rep's message back on the ladder to
// be delivered a second time. Only one of those is recoverable by a human.
func transmit(ctx context.Context, rt extension.Runtime, t transmission, secret, body []byte) (extension.Receipt, error) {
	if err := recordAttempt(ctx, rt, t, outcomeUnknown, classDeliveryUnanswered); err != nil {
		return extension.Receipt{}, err
	}
	sendErr := t.post.post(ctx, secret, deliveryNonce(t.msg.IdempotencyKey), t.at, body)
	if err := recordAttempt(ctx, rt, t, outcomeOf(sendErr), attemptClass(sendErr)); err != nil {
		switch {
		case sendErr == nil:
			return extension.Receipt{}, fmt.Errorf("%w: it was accepted and this installation could not record that: %s", extension.ErrSendOutcomeUnknown, err.Error())
		case errors.Is(sendErr, errUnanswered):
			// The POST may already be at the recipient, and this write failing
			// too must not downgrade that into an ordinary ledger error: the
			// core retries every refusal it is not told is unanswerable, and
			// losing this signal here delivers the rep's message a second
			// time exactly as if transmissionRefusal had never run.
			return extension.Receipt{}, fmt.Errorf("%w: %s, and this installation could not record that either: %s", extension.ErrSendOutcomeUnknown, sendErr.Error(), err.Error())
		default:
			return extension.Receipt{}, err
		}
	}
	if sendErr != nil {
		return extension.Receipt{}, transmissionRefusal(sendErr)
	}
	// EMPTY, and the surface sanctions it: this connector's receiver returns no
	// id of its own, and a unit inventing one would mint a thread key that
	// matches nothing on any later reply.
	return extension.Receipt{ProviderMessageID: ""}, nil
}

// transmissionRefusal maps a failure of the POST ITSELF, which carries one class
// the pre-flight cannot: the message may already be at the recipient.
//
// This connector has no idempotency lookup at the far end, so a request whose
// answer never came back is a question no later attempt can settle. The core
// retries every refusal it is not told is unanswerable, so reporting one as an
// ordinary transport failure delivers the rep's message twice — the one failure a
// human cannot undo.
func transmissionRefusal(err error) error {
	if errors.Is(err, errUnanswered) {
		return fmt.Errorf("%w: %s", extension.ErrSendOutcomeUnknown, err.Error())
	}
	return err
}

// sendable resolves the member's own endpoint and the secret that signs for it,
// or says which of the four things is missing.
//
// Every refusal here transmitted NOTHING, so each is a definite answer. They are
// ErrNotFound rather than a transport failure because the product's pre-flight
// asks Live before it ever gets here: reaching this arm means the member's
// endpoint changed between the two, and the next attempt's Live answers false and
// parks the delivery where a human can see it.
func sendable(ctx context.Context, rt extension.Runtime, member extension.UserID) (endpoint, []byte, error) {
	var mine *endpoint
	if err := rt.Tx(ctx, func(ctx context.Context, tx extension.Tx) error {
		found, err := endpointOf(ctx, tx, string(member))
		mine = found
		return err
	}); err != nil {
		return endpoint{}, nil, err
	}
	switch {
	case mine == nil:
		return endpoint{}, nil, fmt.Errorf("%w: this member has not opened an openchannel endpoint", extension.ErrNotFound)
	case !mine.Enabled:
		return endpoint{}, nil, fmt.Errorf("%w: this member's openchannel endpoint is paused", extension.ErrNotFound)
	case mine.URL == "":
		return endpoint{}, nil, fmt.Errorf("%w: this member has registered no address for this connector to post to", extension.ErrNotFound)
	}
	secret, err := rt.Secrets().GetUser(ctx, member, inboundSecretKey)
	if err != nil {
		if errors.Is(err, extension.ErrSecretNotFound) {
			return endpoint{}, nil, fmt.Errorf("%w: this member has minted no signing secret, so nothing this connector posted could be verified", extension.ErrNotFound)
		}
		return endpoint{}, nil, err
	}
	return *mine, secret, nil
}

// live answers whether this member can send right now, and answers it WITHOUT
// SPENDING THE CREDENTIAL — nothing here dials anything, which is why it takes no
// sender factory at all.
//
// It is the same four facts sendable checks, asked one step earlier: an endpoint
// exists, it is enabled, it names an address, and a secret is on deposit to sign
// with. Asking anything of the far end would be spending a real request to learn
// something this side already knows, on a path the product runs before every
// delivery.
//
// A CONFIRMED "no" is false and a failure to find out is an error, and the core
// treats them differently — one parks the delivery, the other retries it — so
// collapsing them would either strand a deliverable message or re-send a refused
// one.
func live(ctx context.Context, rt extension.Runtime, member extension.UserID) (bool, error) {
	if _, _, err := sendable(ctx, rt, member); err != nil {
		if errors.Is(err, extension.ErrNotFound) {
			// A confirmed no: the member has nothing to send through, and there
			// is nothing to retry into.
			return false, nil
		}
		return false, fmt.Errorf("openchannel: could not confirm this member's endpoint: %w", err)
	}
	return true, nil
}
