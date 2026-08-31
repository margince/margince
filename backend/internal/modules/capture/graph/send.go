// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package graph

// The OUTBOUND half of the Outlook connector: transmitting one message as the
// connected mailbox owner. Split from graph.go the way Gmail's send is split
// from its capture, because it answers a different question and rides a
// different scope.
//
// The bytes are the SHARED renderer (capture/mailwire), not a second one:
// Microsoft's sendMail accepts a complete RFC822 message under a `text/plain`
// content type, which is the same wire format Gmail's messages.send takes. What
// a multipart/alternative puts first, whether an attachment's filename rides in
// one header or two, where base64 folds — those have one right answer, and two
// renderers would answer them twice.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"

	"github.com/margince/margince/backend/internal/modules/capture/mailmap"
	"github.com/margince/margince/backend/internal/modules/capture/mailwire"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// SendScope permits transmission only — it cannot read, modify, or delete,
// which is why it rides alongside the read-only capture scopes rather than
// replacing them.
//
// Exported so the OAuth consent that REQUESTS it names the same string this
// connector RE-CHECKS. A send whose consent asked for a scope the connector
// then does not find parks every message with "not granted the send scope",
// and nothing at compile time would say why.
const SendScope = "Mail.Send"

// ErrSendScopeMissing marks a connection whose Microsoft grant does not include
// the send permission: the mailbox was connected for capture and declined (or
// predates) sending. Reconnecting is the fix, so the caller parks rather than
// retries.
var ErrSendScopeMissing = fmt.Errorf("graph: this connection was not granted the send permission: %w", connector.ErrAuthRejected)

var (
	_ connector.EmailSender       = (*Connector)(nil)
	_ connector.AttachmentCarrier = (*Connector)(nil)
)

// maxSendableFiles is the most files this connector will put in one message.
//
// It is the contract's own attachment_ids cap, restated here because the
// carriage gate can only enforce a bound a connector DECLARES: nothing in this
// stack validates a request against the schema, so an undeclared cap would let
// a caller name fifty files and have every one of them transmitted.
const maxSendableFiles = 10

// maxSendableFileBytes is what survives Microsoft's MIME submit ceiling.
//
// Graph accepts 4 MB of base64 for a sendMail, and base64 costs a third — so
// the raw budget for one message is about 3 MiB, and a single file may not
// exceed it. Stated as the file bound because that is the seam the dispatcher
// enforces; a message carrying several smaller files can still exceed the
// whole-message ceiling, and Microsoft refuses that one.
const maxSendableFileBytes = 3 << 20

// SendEmail transmits one message as the connected mailbox owner.
//
// On a retry it first asks Microsoft whether this message identity already
// exists in Sent Items. Job delivery is at-least-once and Graph offers no
// idempotency key, so without that lookup a crash between a successful
// transmission and its recorded outcome mails the recipient twice.
//
// UNLIKE GMAIL, THE LOOKUP HAS SOMETHING TO SEARCH FOR. Gmail discards a
// client-supplied Message-ID and files the message under one of its own, which
// leaves its own guard inoperative (gmail/send.go says so). Graph exposes the
// identity as the filterable `internetMessageId` property, so the search is
// against the message's own field rather than a full-text index.
//
// It still does not CLOSE the window, and the reasons are worth stating rather
// than leaving to be discovered. Exchange Online may rewrite the identity on
// submission depending on tenant configuration, in which case the filter
// matches nothing and the retry mails twice — the same failure Gmail has
// always had, but conditional rather than certain. Nor does anything here
// serialize concurrent attempts at the same delivery: that guarantee lives
// outside this package, in River delivering one job per delivery.
func (c *Connector) SendEmail(ctx context.Context, auth connector.Auth, msg connector.EmailMessage) (connector.SendReceipt, error) {
	// Before anything reaches Microsoft: a message with no usable identity
	// cannot be found again by the prior-send lookup, so transmitting it would
	// mail the recipient once per retry with nothing able to tell that it had
	// already gone.
	if err := msg.Validate(); err != nil {
		return connector.SendReceipt{}, err
	}
	var st authState
	if err := json.Unmarshal(auth, &st); err != nil {
		return connector.SendReceipt{}, fmt.Errorf("graph: malformed auth bundle: %w", err)
	}
	if !slices.Contains(st.Granted, SendScope) {
		return connector.SendReceipt{}, ErrSendScopeMissing
	}
	access, err := c.oauth.AccessToken(ctx, st.RefreshToken)
	if err != nil {
		return connector.SendReceipt{}, err
	}
	if msg.Attempt > 0 {
		id, found, findErr := c.api.FindSentByMessageID(ctx, access, msg.MessageID)
		if findErr != nil {
			return connector.SendReceipt{}, findErr
		}
		if found {
			return connector.SendReceipt{ProviderMessageID: id}, nil
		}
	}
	if err := c.api.SendMIME(ctx, access, []byte(mailwire.Build(st.Owner, msg))); err != nil {
		return connector.SendReceipt{}, err
	}
	// sendMail returns 202 Accepted with no body: Microsoft names no message id
	// at submission, so the sent copy has to be looked up by the identity it was
	// asked to carry. That lookup is also the read-back — one call answers both
	// "which message is it" and "what identity does it actually carry".
	return c.sentReceipt(ctx, access, st.Owner, msg.MessageID), nil
}

// Carriage declares what this connector transmits (connector.AttachmentCarrier).
//
// There is no default for this and that is the design: a message with files
// staged against a connector that does not declare the capability PARKS rather
// than going out stripped, because a recipient seeing fewer files than the
// timeline records is a wrong record nobody is told about. Graph's sendMail
// takes a complete RFC822 message, so the files ride in the multipart/mixed
// envelope mailwire.Build renders — nothing is uploaded separately and nothing
// is linked.
//
// The per-file limit is MICROSOFT'S, not this system's, and it is lower than
// Gmail's — the one place the two providers genuinely differ on what they can
// carry. Graph's MIME submit ceiling is 4 MB of BASE64, which is about 3 MiB of
// actual bytes, where Gmail takes the full inbound 25 MiB. Declaring the real
// number is what makes an over-large message PARK with an honest reason instead
// of leaving as a request Microsoft answers with an opaque refusal that a retry
// can never get under.
func (c *Connector) Carriage() connector.Carriage {
	return connector.Carriage{
		Carries:         true,
		MaxBytesPerFile: maxSendableFileBytes,
		MaxFiles:        maxSendableFiles,
		// Mail carries the body as the body, never as a caption, so there is no
		// extra bound to declare.
		MaxBodyWithFiles: 0,
	}
}

// sentReceipt resolves what Microsoft actually filed, after a submission it
// acknowledged without naming.
//
// EVERY failure returns an empty receipt rather than an error. The message has
// already been transmitted when this runs, and returning an error would hand
// the delivery back to a retry — mailing the recipient a second time to fix a
// bookkeeping problem. An unread identity costs one duplicate timeline row; a
// re-mail costs the recipient's trust.
//
// What comes back is CHECKED before it is reported. These are remote bytes, and
// the string parsed out of them becomes a natural key, a thread key and a log
// field on the strength of this return alone — so it must satisfy
// connector.ValidMessageID, the same predicate SendEmail refuses to transmit
// without.
//
// The identity is parsed with mailmap, the same function capture parses the
// echo with, so the identity recorded here and the identity derived there are
// one function of one set of bytes.
func (c *Connector) sentReceipt(ctx context.Context, access, owner, askedFor string) connector.SendReceipt {
	providerID, found, err := c.api.FindSentByMessageID(ctx, access, askedFor)
	if err != nil || !found {
		// Not a fault to report: Exchange may have rewritten the identity, or
		// the sent copy may not have been filed yet. Either way the message is
		// gone and the delivery must not be retried over a lookup.
		slog.WarnContext(ctx, "graph: could not resolve the sent copy of a transmitted message",
			"err", err, "found", found)
		return connector.SendReceipt{}
	}
	raw, err := c.api.GetMIME(ctx, access, providerID)
	if err != nil {
		slog.WarnContext(ctx, "graph: reading back the sent message identity",
			"err", err, "provider_message_id", providerID)
		return connector.SendReceipt{ProviderMessageID: providerID}
	}
	parsed, err := mailmap.Parse(raw, owner)
	if err != nil {
		slog.WarnContext(ctx, "graph: parsing the sent message identity", "err", err)
		return connector.SendReceipt{ProviderMessageID: providerID}
	}
	id := parsed.ID()
	if !connector.ValidMessageID(id) {
		// The rejected value is deliberately NOT logged: it is unbounded
		// provider input, and the two facts that diagnose this — that the
		// read-back answered with something unusable, and how big it was —
		// carry no risk of writing megabytes or control bytes into a log line.
		slog.WarnContext(ctx, "graph: the sent copy carries no usable message identity",
			"provider_message_id", providerID, "identity_bytes", len(id))
		return connector.SendReceipt{ProviderMessageID: providerID}
	}
	return connector.SendReceipt{ProviderMessageID: providerID, RFC822MessageID: id}
}
