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
// A matching Message-ID is not enough on its own, and the replay that reaches
// here has already matched the stored content:
//   - a received reading is turned round when its sender is an address this
//     seat proved by connecting that mailbox, or one of the seat's own exact
//     addresses — declared or discovered — that no other seat holds. A seat's
//     known alias is theirs; an address two seats hold is nobody's to claim;
//   - an outbound reading is attested only when it already names this seat as
//     its sender.
func storedAsThisSeatsTx(ctx context.Context, tx pgx.Tx, id ids.ActivityID, direction, stored, recipient string) (bool, error) {
	_, seat := capturePrincipal(ctx)
	switch {
	case direction == connector.DirectionInbound:
		// Unambiguously: an address two seats proved, a shared mailbox, is
		// nobody's to claim, or whichever seat synced first would take it.
		proved, err := ProvedUnambiguouslyTx(ctx, tx, seat, stored)
		if err != nil || proved {
			return proved, err
		}
		return ownAddressHeldAloneTx(ctx, tx, seat, stored)
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

// ownAddressHeldAloneTx reports that address is one of the acting seat's own
// exact addresses and that no other seat holds it.
//
// Under the address's own lock, which every identity writer also takes, so a
// seat adding the same alias in a concurrent transaction is seen or waits.
func ownAddressHeldAloneTx(ctx context.Context, tx pgx.Tx, seat ids.UUID, address string) (bool, error) {
	self, err := ownerIdentitiesTx(ctx, tx)
	if err != nil || !self.CoversAddressExactly(address) {
		return false, err
	}
	if err := lockOwnAddressTx(ctx, tx, address); err != nil {
		return false, err
	}
	held, err := addressHeldByAnotherSeatTx(ctx, tx, seat, address)
	return !held, err
}

// addressHeldByAnotherSeatTx reports whether any other seat holds address as one
// of its own: an address identity, or the account a connected mailbox of theirs
// was granted for. Read the way ownerIdentitiesTx reads a seat's own set —
// bareAddress and foldAddress in Go, and a discovered machine address skipped —
// so two spellings of one address are one address here as well.
func addressHeldByAnotherSeatTx(ctx context.Context, tx pgx.Tx, seat ids.UUID, address string) (bool, error) {
	rows, err := tx.Query(ctx, `
		SELECT value, source FROM capture_owner_identity WHERE kind = $2 AND user_id <> $1
		 UNION ALL
		SELECT account_label, '' FROM capture_connection
		 WHERE user_id <> $1 AND status = 'connected' AND archived_at IS NULL
		   AND coalesce(account_label, '') <> ''`, seat, IdentityKindAddress)
	if err != nil {
		return false, fmt.Errorf("capture: reading whether another seat holds %s: %w", address, err)
	}
	defer rows.Close()
	want := foldAddress(address)
	for rows.Next() {
		var value, source string
		if err := rows.Scan(&value, &source); err != nil {
			return false, fmt.Errorf("capture: reading whether another seat holds %s: %w", address, err)
		}
		if discoveredMachineAddress(source, value) {
			continue
		}
		if foldAddress(bareAddress(value)) == want {
			return true, nil
		}
	}
	return false, rows.Err()
}

// lockOwnAddressTx serializes everything that decides or changes which seats
// hold one address, keyed on its folded form.
func lockOwnAddressTx(ctx context.Context, tx pgx.Tx, address string) error {
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('margince:own-address:' || $1)::bigint)`,
		foldAddress(bareAddress(address))); err != nil {
		return fmt.Errorf("capture: locking who holds %s: %w", address, err)
	}
	return nil
}
