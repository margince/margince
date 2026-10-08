// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// Mail a seat sent from another address of theirs, stored as received before
// capture read it as outbound (asSentFromOwnAddressTx). The provider's sent
// filing was never stored, so the evidence a stored row still carries is its
// original: every hop that delivers mail prepends a Received header, and a
// message in the seat's own mailbox with none was written there by its owner.

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/emersion/go-message/textproto"
	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// The verdicts a re-read of one stored message reaches.
const (
	// OwnSentMailClaimed: the claim turned the row round as the seat's
	// outbound mail.
	OwnSentMailClaimed = "claimed"
	// OwnSentMailUnchanged: the seat may claim it, but the row no longer read
	// as received from the seat, or was archived since it was offered.
	OwnSentMailUnchanged = "unchanged"
	// OwnSentMailDelivered: the original records a delivery hop, so the
	// mailbox received it and it stays received.
	OwnSentMailDelivered = "delivered"
	// OwnSentMailNotTheSeats: its sender was not an address the seat alone has
	// proven when it was judged.
	OwnSentMailNotTheSeats = "not_the_seats"
	// OwnSentMailNoRecipient: it names nobody but the seat to have been
	// written to.
	OwnSentMailNoRecipient = "no_recipient"
	// OwnSentMailUnreadable: the original does not parse as a message.
	OwnSentMailUnreadable = "unreadable"
	// OwnSentMailErased: the recipient's address was erased, and a claim would
	// write it back onto the row.
	OwnSentMailErased = "erased"
)

// StoredOwnSentMail is one row stored as received from sender, captured by
// seat's own Gmail or Graph connection. Participants are its original's
// further parties as mailmap reads them, To before Cc.
type StoredOwnSentMail struct {
	Activity     ids.ActivityID
	Seat         ids.UUID
	Sender       string
	Participants []connector.MessageParticipant
}

// ReclaimStoredOwnSentMailTx re-reads one stored received message against its
// original and, where the seat wrote it from another address of theirs, claims
// it as their outbound mail to the recipient live capture would pick
// (sentToAt).
//
// The sender must be an address this seat alone has proven by connecting its
// mailbox. A declared address is not enough here: without the provider's sent
// filing, the absent Received header is the only other evidence, and a seat can
// declare an address that is not theirs.
func ReclaimStoredOwnSentMailTx(
	ctx context.Context, tx pgx.Tx, claim OwnSentMailClaim, row StoredOwnSentMail, original []byte,
) (string, error) {
	delivered, readable := carriesDeliveryHop(original)
	if !readable {
		return OwnSentMailUnreadable, nil
	}
	if delivered {
		return OwnSentMailDelivered, nil
	}
	proved, err := ProvedUnambiguouslyTx(ctx, tx, row.Seat, row.Sender)
	if err != nil || !proved {
		return OwnSentMailNotTheSeats, err
	}
	pass, ok := principal.Actor(ctx)
	if !ok {
		return "", fmt.Errorf("capture: re-reading %s as sent mail needs a bound pass: %w", row.Activity, apperrors.ErrPermissionDenied)
	}
	pass.OnBehalfOf = row.Seat
	ctx = principal.WithActor(ctx, pass)
	self, err := ownerIdentitiesTx(ctx, tx)
	if err != nil {
		return "", err
	}
	at := sentToAt(row.Participants, self)
	if at < 0 {
		return OwnSentMailNoRecipient, nil
	}
	recipient := row.Participants[at].Email
	// The original can outlive an erasure of its recipient; the row must not
	// gain the address back from it.
	erased, err := storekit.EmailSuppressed(ctx, tx, recipient)
	if err != nil {
		return "", fmt.Errorf("capture: reading whether a recipient of %s was erased: %w", row.Activity, err)
	}
	if erased {
		return OwnSentMailErased, nil
	}
	claimed, err := claim(ctx, tx, row.Activity, row.Sender, recipient)
	switch {
	case errors.Is(err, apperrors.ErrNotFound):
		return OwnSentMailUnchanged, nil
	case err != nil:
		return "", err
	case !claimed:
		return OwnSentMailUnchanged, nil
	}
	return OwnSentMailClaimed, nil
}

// carriesDeliveryHop reports whether a stored original's header block records
// a Received hop, and false for readable when the header does not parse. Only
// the header is read: the body has no say in it.
func carriesDeliveryHop(original []byte) (delivered, readable bool) {
	header, err := textproto.ReadHeader(bufio.NewReader(bytes.NewReader(original)))
	if err != nil {
		return false, false
	}
	return header.Has("Received"), true
}
