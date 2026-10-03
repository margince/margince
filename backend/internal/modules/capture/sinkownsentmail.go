// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// OwnSentMailClaim rewrites a stored received message as the seat's own sent
// mail. activities.ClaimOwnSentMailTx is the implementation; compose injects
// it, because capture never imports a sibling.
type OwnSentMailClaim func(ctx context.Context, tx pgx.Tx, activityID ids.ActivityID, sender, recipient string) error

// WithOwnSentMailClaim returns a copy that corrects a colleague's earlier
// reading of this seat's sent mail. Without one, the first reading stands.
func (s *Sink) WithOwnSentMailClaim(claim OwnSentMailClaim) *Sink {
	out := *s
	out.claimOwnSentMail = claim
	return &out
}

// claimOwnSentMailTx runs on a proven replay: the seat's own mailbox delivered,
// from its sent filing, a message another mailbox stored first. When that
// first reading is "received from an address this seat proved", it was this
// seat's outbound mail all along; when it is already this seat's outbound mail
// to the same recipient but unattested, the filing attests it. Gmail and Graph
// only, as in asSentFromOwnAddressTx.
func (s *Sink) claimOwnSentMailTx(ctx context.Context, tx pgx.Tx, id ids.ActivityID, rec connector.NormalizedRecord) error {
	cp := rec.Counterparty
	if s.claimOwnSentMail == nil || !providerFiledTransport(rec.CapturedBy) ||
		cp.Direction != connector.DirectionOutbound || !cp.SentByOwner() || cp.Email == "" {
		return nil
	}
	var direction, stored string
	err := tx.QueryRow(ctx, `
		SELECT coalesce(direction, ''), coalesce(counterparty_email, '') FROM activity
		 WHERE id = $1 AND kind = 'email' AND archived_at IS NULL`, id).Scan(&direction, &stored)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("capture: reading how %s was first filed: %w", id, err)
	}
	self, err := ownerIdentitiesTx(ctx, tx)
	if err != nil {
		return err
	}
	if self.CoversAddressExactly(cp.Email) {
		return nil
	}
	ours, err := storedAsThisSeatsTx(ctx, tx, id, direction, stored, cp.Email)
	if err != nil || !ours {
		return err
	}
	// Archived between the read above and the claim's lock, by an erasure
	// racing this capture: nothing to correct, and the capture goes on.
	if err := s.claimOwnSentMail(ctx, tx, id, stored, cp.Email); err != nil && !errors.Is(err, apperrors.ErrNotFound) {
		return err
	}
	return nil
}

// storedAsThisSeatsTx is the standing to rewrite a row another mailbox stored.
// A seat can declare any address and plant a sent copy through the provider's
// API, so neither a declared address nor a matching Message-ID is enough:
//   - a received reading is turned round only when its sender is an address
//     this seat PROVED, by connecting that mailbox (SeatProvedAddressTx);
//   - an outbound reading is attested only when it already names this seat as
//     its sender.
func storedAsThisSeatsTx(ctx context.Context, tx pgx.Tx, id ids.ActivityID, direction, stored, recipient string) (bool, error) {
	_, seat := capturePrincipal(ctx)
	switch {
	case direction == connector.DirectionInbound:
		// Unambiguously: an address two seats proved, a shared mailbox, is
		// nobody's to claim, or whichever seat synced first would take it.
		return ProvedUnambiguouslyTx(ctx, tx, seat, stored)
	case direction == connector.DirectionOutbound && foldAddress(stored) == foldAddress(recipient):
		var sentBySeat bool
		err := tx.QueryRow(ctx, `
			SELECT EXISTS (SELECT 1 FROM activity_participant
			  WHERE activity_id = $1 AND role = 'from' AND user_id = $2)`, id, seat).Scan(&sentBySeat)
		if err != nil {
			return false, fmt.Errorf("capture: reading who sent %s: %w", id, err)
		}
		return sentBySeat, nil
	}
	return false, nil
}
