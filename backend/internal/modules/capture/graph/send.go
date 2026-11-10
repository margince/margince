// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package graph

// Graph and Gmail accept the same MIME message.
// Shared mailwire keeps attachment, threading and sender rules consistent.

import (
	"context"
	"fmt"
	"slices"

	"github.com/margince/margince/backend/internal/modules/capture/graphconn"
	"github.com/margince/margince/backend/internal/platform/mailwire"
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
// The encoding is applied TWICE, which is the part that is easy to get wrong
// and was: mailwire.Build renders each attachment as base64 INSIDE the message,
// and SendMIME then base64-encodes the whole message again for the wire. A file
// of N bytes therefore costs roughly N × 4/3 × 4/3 ≈ 1.8N against a 4 MB
// request limit, before headers, boundaries and the covering text.
//
// 2 MiB leaves real headroom under that. It is an EARLY, legible refusal rather
// than the bound that matters — several files each under it can still exceed
// the whole-message ceiling, which is why SendMIME checks the rendered message
// too rather than trusting this.
const maxSendableFileBytes = 2 << 20

// SendEmail keeps a stable identity so retries can look up accepted mail.
// The handoff-aware seam separates submission from preparation and read-back.
func (c *Connector) SendEmail(ctx context.Context, auth connector.Auth, msg connector.EmailMessage) (connector.SendReceipt, error) {
	return c.sendWithHandoff(ctx, auth, msg, func(ctx context.Context, send func(context.Context) (connector.SendReceipt, error)) (connector.SendReceipt, error) {
		return send(ctx)
	})
}

// SendEmailWithHandoff leaves refresh and prior-send lookup outside stop ordering.
func (c *Connector) SendEmailWithHandoff(ctx context.Context, auth connector.Auth, msg connector.EmailMessage, handoff connector.SendHandoff) (connector.SendReceipt, error) {
	return c.sendWithHandoff(ctx, auth, msg, handoff)
}

func (c *Connector) sendWithHandoff(ctx context.Context, auth connector.Auth, msg connector.EmailMessage, handoff connector.SendHandoff) (connector.SendReceipt, error) {
	// Before anything reaches Microsoft: a message with no usable identity
	// cannot be found again by the prior-send lookup, so transmitting it would
	// mail the recipient once per retry with nothing able to tell that it had
	// already gone.
	if err := msg.Validate(); err != nil {
		return connector.SendReceipt{}, err
	}
	st, err := graphconn.Read(connectorName, auth)
	if err != nil {
		return connector.SendReceipt{}, err
	}
	if !slices.Contains(st.Granted, SendScope) {
		return connector.SendReceipt{}, ErrSendScopeMissing
	}
	refreshed, err := c.oauth.Refresh(ctx, st.RefreshToken, st.Granted)
	if err != nil {
		return connector.SendReceipt{}, err
	}
	access := refreshed.AccessToken
	if msg.Attempt > 0 {
		id, found, findErr := c.api.FindSentByMessageID(ctx, access, msg.MessageID)
		if findErr != nil {
			return connector.SendReceipt{}, findErr
		}
		if found {
			return connector.SendReceipt{ProviderMessageID: id}, nil
		}
	}
	raw := []byte(mailwire.Build(st.Owner, msg))
	if _, err := handoff(ctx, func(wire context.Context) (connector.SendReceipt, error) {
		return connector.SendReceipt{}, c.api.SendMIME(wire, access, raw)
	}); err != nil {
		return connector.SendReceipt{}, err
	}
	// Graph accepts asynchronously without a message id. A lookup here races
	// that filing; retries search by the stable identity after it has arrived.
	return connector.SendReceipt{}, nil
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
// The per-file limit is MICROSOFT'S, not this system's, and it is far lower than
// Gmail's — the one place the two providers genuinely differ on what they can
// carry. Graph caps a sendMail request at 4 MB and the payload is base64-encoded
// twice on the way there (see maxSendableFileBytes), where Gmail takes the full
// inbound 25 MiB. Declaring the real number is what makes an over-large message
// PARK with an honest reason instead of leaving as a request Microsoft answers
// with an opaque refusal that no retry can get under.
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
