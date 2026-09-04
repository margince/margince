// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package comms

// The transport for a message the INSTALLATION sends, and the one-time material
// it carries.
//
// Its own file beside sendseam.go, which owns the branch on provider class. The
// controller arm lives here because everything that makes it different from a
// user send is here: there is no connected mailbox to resolve, no OAuth grant
// to intersect, and the body is not final until the link is substituted into
// it.
//
// Past the arm this is an ordinary delivery. The consent gate, the retry
// ladder, the age bound and the four dispositions all apply unchanged, which is
// the point of resolving it as a seam rather than teaching the dispatcher a
// second path.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// ControllerRelay transmits a message the installation sends as itself.
//
// Separate from the mail connectors because there is no person behind it: the
// relay is deployment configuration, its from-address belongs to the
// installation, and no subject's mailbox grant is involved.
type ControllerRelay interface {
	SendControllerMail(ctx context.Context, msg ControllerMessage) error
}

// ControllerMessage is one rendered controller mail, link already substituted.
type ControllerMessage struct {
	To        string
	Subject   string
	TextBody  string
	MessageID string
}

// PayloadVault holds the one-time material a controller message carries, so the
// plaintext link never sits on the delivery row.
type PayloadVault interface {
	Get(ctx context.Context, ref string) (string, error)
	Delete(ctx context.Context, ref string) error
}

// ErrNoControllerRelay reports that this installation has no relay configured.
// It is a DEPLOYMENT fact, not an answer about the message, which is why the
// dispatcher parks on it rather than retrying forever.
var ErrNoControllerRelay = errors.New("comms: no controller relay is configured")

// controllerSeam binds the relay call for a controller delivery.
//
// detectsPriorSend is FALSE, and that is the honest answer rather than a
// conservative one: an SMTP relay hands a message on and returns, with no
// identity to search and no read-back to ask. So the in-flight marker is what
// bounds this lane to at most one transmission, exactly as it does for a
// messaging channel.
func (d *Dispatcher) controllerSeam(ctx context.Context, del Delivery) (sendSeam, error) {
	if d.relay == nil {
		return sendSeam{}, ErrNoControllerRelay
	}
	// StageControllerTx refuses an empty addressee and writes exactly one, so
	// an empty list here is a row that did not come through it.
	if len(del.Recipients) == 0 {
		return sendSeam{}, ErrNoAddressee
	}
	to := del.Recipients[0]
	body, err := d.substituteLink(ctx, del)
	if err != nil {
		return sendSeam{}, err
	}
	return sendSeam{
		// No granted scopes and no carriage: a controller message carries no
		// attachments and holds no OAuth grant. profileFor already turns both
		// gates off; leaving these zero means the seam agrees with it rather
		// than restating it.
		transmit: func(ctx context.Context, _ []connector.OutboundFile) (connector.SendReceipt, error) {
			if err := d.relay.SendControllerMail(ctx, ControllerMessage{
				To:        to,
				Subject:   del.Subject,
				TextBody:  body,
				MessageID: del.MessageID,
			}); err != nil {
				return connector.SendReceipt{}, err
			}
			// The row's own message id, echoed back. A relay does not rewrite
			// the identity it was handed, so nothing is owed a re-key.
			return connector.SendReceipt{RFC822MessageID: del.MessageID}, nil
		},
	}, nil
}

// substituteLink puts the one-time link into the body, in memory, once.
//
// The plaintext never returns to the row. StageControllerTx already held the
// body and the material to the same story — exactly one placeholder when there
// is a reference, none when there is not — so a disagreement here is a vault
// that lost the entry rather than a caller mistake, and it fails the attempt
// rather than sending a message with "{{confirmation-link}}" in it.
func (d *Dispatcher) substituteLink(ctx context.Context, del Delivery) (string, error) {
	if del.PayloadRef == "" {
		return del.Body, nil
	}
	if d.payloads == nil {
		return "", fmt.Errorf("comms: this delivery carries link material and no vault is configured: %w", ErrNoControllerRelay)
	}
	link, err := d.payloads.Get(ctx, del.PayloadRef)
	if err != nil {
		return "", fmt.Errorf("comms: reading the one-time link material: %w", err)
	}
	if placeholderCount(del.Body) != 1 {
		return "", fmt.Errorf("comms: the staged body carries %d placeholder(s) for its link material: %w",
			placeholderCount(del.Body), ErrTemplateShape)
	}
	return strings.Replace(del.Body, LinkPlaceholder, link, 1), nil
}

// closePayload retires the one-time material once the message can no longer be
// sent — accepted, or parked for good.
//
// The REFERENCE is cleared and the row is kept, because the delivery is still
// the record that a message went out. What must not survive is a live
// credential pointing at somebody's mailbox.
//
// A failure here is logged by the caller and never changes the delivery's
// disposition: the message really was accepted, and reporting a retry because
// the cleanup stumbled would send it a second time.
func (d *Dispatcher) closePayload(ctx context.Context, del Delivery) error {
	if del.PayloadRef == "" {
		return nil
	}
	if d.payloads != nil {
		if err := d.payloads.Delete(ctx, del.PayloadRef); err != nil {
			return fmt.Errorf("comms: destroying the one-time link material: %w", err)
		}
	}
	return d.store.ClearPayloadRef(ctx, del.ID)
}

// carriesLiveLink reports whether this delivery holds one-time material that a
// terminal outcome must retire.
func (d Delivery) carriesLiveLink() bool { return d.IsController() && d.PayloadRef != "" }

// dispatchClosingLink runs the attempt and retires the link if the message
// reached a state it will never be sent from.
//
// PARKED only. A retry or a deferral keeps the material, because the message is
// still going out; the accepted path retires it after RecordSent, on the other
// terminal outcome. This wrapper exists because park() takes an id and cannot
// see the material, and this is the one place holding both the row and the
// outcome.
func (d *Dispatcher) dispatchClosingLink(ctx context.Context, del Delivery) (Outcome, time.Duration, error) {
	outcome, wait, err := d.dispatchLoaded(ctx, del)
	if outcome == OutcomeParked {
		d.retireLink(ctx, del)
	}
	return outcome, wait, err
}

// retireLink destroys the one-time material and reports a failure to do so
// WITHOUT changing the delivery's disposition.
//
// That is the rule this function exists to hold. A cleanup that stumbled must
// never turn an accepted send into a retry: the message really was accepted and
// the row says so, and re-running it would put a second copy on the wire to
// protect a credential. The sweep is the backstop, and Art. 17 erasure destroys
// the material regardless of what happens here.
func (d *Dispatcher) retireLink(ctx context.Context, del Delivery) {
	if err := d.closePayload(ctx, del); err != nil {
		slog.ErrorContext(ctx, "retiring the one-time link material failed",
			"delivery_id", del.ID)
	}
}
