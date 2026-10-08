// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// Mail a seat sent from another address of theirs, stored as received before
// capture read it as outbound (asSentFromOwnAddressTx). The provider's sent
// filing was never stored, so the evidence a stored row still carries is its
// original: every hop that delivers mail prepends a Received header, and a
// message in the seat's own mailbox with none was written there by its owner.

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/emersion/go-message/mail"
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
	// OwnSentMailUnchanged: the seat may claim it, but the row no longer reads
	// as received from the seat, or was archived since it was offered.
	OwnSentMailUnchanged = "unchanged"
	// OwnSentMailDelivered: the original records a delivery hop, so the
	// mailbox received it and it stays received.
	OwnSentMailDelivered = "delivered"
	// OwnSentMailNotTheSeats: its sender is not an address the seat may claim
	// today, or it names nobody but the seat to have been written to.
	OwnSentMailNotTheSeats = "not_the_seats"
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
// The standing is the one a live claim checks (receivedFromTheSeatTx), read
// for the seat whose mailbox stored the row rather than for the acting pass.
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
	pass, ok := principal.Actor(ctx)
	if !ok {
		return "", fmt.Errorf("capture: re-reading %s as sent mail needs a bound pass: %w", row.Activity, apperrors.ErrPermissionDenied)
	}
	pass.OnBehalfOf = row.Seat
	ctx = principal.WithActor(ctx, pass)
	ours, err := receivedFromTheSeatTx(ctx, tx, row.Seat, row.Sender)
	if err != nil || !ours {
		return OwnSentMailNotTheSeats, err
	}
	self, err := ownerIdentitiesTx(ctx, tx)
	if err != nil {
		return "", err
	}
	at := sentToAt(row.Participants, self)
	if at < 0 {
		return OwnSentMailNotTheSeats, nil
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

// carriesDeliveryHop reports whether a stored original's header records a
// Received hop, and false for readable when the bytes are not a message.
func carriesDeliveryHop(original []byte) (delivered, readable bool) {
	reader, err := mail.CreateReader(bytes.NewReader(original))
	if err != nil {
		return false, false
	}
	delivered = reader.Header.Has("Received")
	if err := reader.Close(); err != nil {
		return false, false
	}
	return delivered, true
}
