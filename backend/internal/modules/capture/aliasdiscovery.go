// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// Discovering a forwarding alias from the mail that arrives at it.
//
// A seat's own address is known three ways: they declared it, their connection
// reports it, or they sign in with it. None reaches an address that DELIVERS
// into the mailbox but is never the From of anything the mailbox sends — a
// previous employer's address, a role address that forwards, a personal domain
// pointed at a work mailbox. So the seat keeps appearing in their own CRM as a
// contact, and every thread arriving at the alias reads as correspondence with
// a stranger.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// aliasSightingsBeforeClaiming is how many DISTINCT messages must carry the
// same delivery claim before it is written as one of the seat's own addresses.
//
// Two, not one. The position check in mailmap.TopDeliveredTo is what makes a
// claim trustworthy at all and it is sufficient on its own — a sender cannot
// reach a header position above the receiving hop's own Received. This second
// rung is not defence against that sender; it is refusing to let a single
// parsing surprise become a stored claim about who somebody IS.
//
// Not higher, because the cost of waiting is paid by the person the feature is
// for: every message that arrives before the threshold is one where their own
// address is still read as a stranger's.
const aliasSightingsBeforeClaiming = 2

// noteAliasSightingTx records that one message was delivered to an address this
// seat has not claimed, and promotes it once enough distinct messages agree.
//
// Inside the capture's own transaction: the sighting and the message that
// produced it land together, so a count can never be higher than the evidence
// actually stored.
//
// Everything about this path fails SILENT and OPEN. A claim it cannot place, an
// address it cannot fold, a seat it cannot resolve — each answers "no alias
// here" and the message captures exactly as it did before. The feature adds an
// address to a seat's self-set; it must never be able to subtract a message
// from the timeline.
func noteAliasSightingTx(ctx context.Context, tx pgx.Tx, seat ids.UUID, deliveredTo, source string) error {
	if seat == ids.Nil || deliveredTo == "" || source == "" {
		return nil
	}
	value, storable := storableAddress(deliveredTo)
	if !storable {
		return nil
	}
	known, err := seatAlreadyKnowsTx(ctx, tx, seat, value)
	if err != nil || known {
		return err
	}
	// ON CONFLICT DO NOTHING is the distinctness rule: a re-synced message, or
	// one replayed by a push notification, is the same sighting rather than a
	// second one. Without it the threshold could be reached by one message
	// arriving twice, which is no corroboration at all.
	if _, err := tx.Exec(ctx, `
		INSERT INTO capture_alias_sighting (user_id, value, source)
		VALUES ($1, $2, $3)
		ON CONFLICT (user_id, value, source) DO NOTHING`, seat, value, source); err != nil {
		return fmt.Errorf("capture: recording a delivery to %s: %w", value, err)
	}
	var sightings int
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FROM capture_alias_sighting WHERE user_id = $1 AND value = $2`,
		seat, value).Scan(&sightings); err != nil {
		return fmt.Errorf("capture: counting the deliveries to %s: %w", value, err)
	}
	if sightings < aliasSightingsBeforeClaiming {
		return nil
	}
	return claimDiscoveredAliasTx(ctx, tx, seat, value)
}

// seatAlreadyKnowsTx reports whether this address is already covered by one of
// the seat's own identities — the address itself, or a domain they claimed.
//
// Asked before the sighting is even recorded, so the ledger holds only
// addresses that are actually candidates. A seat's PRIMARY address is on every
// Delivered-To their mailbox receives, and counting it would fill the table
// with the one address nobody needs discovering.
func seatAlreadyKnowsTx(ctx context.Context, tx pgx.Tx, seat ids.UUID, value string) (bool, error) {
	rows, err := tx.Query(ctx,
		`SELECT kind, value FROM capture_owner_identity WHERE user_id = $1`, seat)
	if err != nil {
		return false, fmt.Errorf("capture: reading the seat's own addresses: %w", err)
	}
	addresses, domains, err := foldIdentityRows(rows)
	if err != nil {
		return false, err
	}
	// The seat's own self-set answers it, rather than a second comparison
	// written here: a domain claim covers every address under it, and two
	// spellings of that rule would disagree the first time one moved.
	return NewSelfSet(addresses, domains).Covers(value), nil
}

// foldIdentityRows collects one seat's identities into the two lists a SelfSet
// is built from.
func foldIdentityRows(rows pgx.Rows) (addresses, domains []string, err error) {
	defer rows.Close()
	for rows.Next() {
		var kind, value string
		if err := rows.Scan(&kind, &value); err != nil {
			return nil, nil, fmt.Errorf("capture: reading the seat's own addresses: %w", err)
		}
		if kind == IdentityKindDomain {
			domains = append(domains, value)
		} else {
			addresses = append(addresses, value)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("capture: reading the seat's own addresses: %w", err)
	}
	return addresses, domains, nil
}

// claimDiscoveredAliasTx writes the alias as one of the seat's own addresses.
//
// The source is its OWN word rather than 'user': the card has to be able to say
// how the product learned this, and a seat looking at an address they never
// typed deserves to see that it was inferred — and to remove it.
//
// It retires the open ledger questions about the address for the reason the
// seat's own claim does: without that, the gate would only bind mail arriving
// from now on, and an address deferred moments earlier would still become a
// contact through the very door this claim closes.
func claimDiscoveredAliasTx(ctx context.Context, tx pgx.Tx, seat ids.UUID, value string) error {
	var identity OwnerIdentity
	err := tx.QueryRow(ctx, `
		INSERT INTO capture_owner_identity (user_id, kind, value, source, created_by)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (user_id, kind, value) DO UPDATE SET value = EXCLUDED.value
		RETURNING id, user_id, kind, value, source, created_at`,
		seat, IdentityKindAddress, value, IdentitySourceDeliveredTo, aliasDiscoveryActor).
		Scan(&identity.ID, &identity.UserID, &identity.Kind,
			&identity.Value, &identity.Source, &identity.CreatedAt)
	if err != nil {
		return fmt.Errorf("capture: claiming %s as the seat's own: %w", value, err)
	}
	if err := retirePendingForIdentityTx(ctx, tx, seat, identity); err != nil {
		return err
	}
	return nil
}

// storableAddress folds a delivery header into the spelling this module stores,
// and reports whether it is one at all.
//
// A boolean rather than an error, because that is what the answer IS: a header
// carries whatever the writing server put there, and an unusable value says
// nothing about the message being captured. Returning an error here would offer
// a caller the chance to fail a capture over a header nobody reads.
func storableAddress(deliveredTo string) (string, bool) {
	value, err := ValidExclusionValue(IdentityKindAddress, deliveredTo)
	return value, err == nil
}
