// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// Erasing a subject who was a buyer in a Deal Room.
//
// A room participant is the one place in this product where a named outside
// person is stored WITHOUT a person row: the seat carries a full name and an
// address of its own, because a buyer is invited by email long before anybody
// decides they are a contact. Erasure resolves a subject by their person row
// and their addresses, so nothing it did reached these seats — an erased
// subject's name and address stayed legible in every room they were invited to,
// and their comments stayed attributed to them by name.
//
// The seat is anonymized IN PLACE rather than deleted, for the reason the rest
// of erasure anonymizes in place: a comment names its participant row, and
// deleting the seat would either cascade the conversation away or orphan it.
// What the two sides said to each other is the seller's business record; who
// said it stops being readable.
//
// The comment BODIES are left alone, deliberately and with the same reasoning
// the timeline's own erasure applies to a message: a buyer's sentence about a
// contract clause is the counterparty's record of a negotiation, not the
// subject's personal data merely because they typed it. Where a body does
// contain personal data, it is reached by the same free-text route every other
// body is, not by a rule special to rooms.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// erasedEmail is the address a wiped seat carries. It is a syntactically valid
// address in the reserved example domain, because the column is NOT NULL and
// carries a lowercase CHECK: a tombstone that cannot be stored is not a
// tombstone. Nothing can be delivered to it.
const erasedEmail = "erased@example.invalid"

// eraseDealRoomSeats is the whole Deal Room step of an erasure: wipe the seats
// carrying one of the subject's addresses, purge what those seats still betray,
// and tombstone each one's audit spine.
//
// It runs BEFORE deleteSubjectIdentifierRows, like the provider purge one step
// over: a seat holds no person id and is resolved by address, which that delete
// destroys.
func eraseDealRoomSeats(ctx context.Context, tx pgx.Tx, emails []string, reason string) error {
	seats, err := anonymizeDealRoomSeats(ctx, tx, emails)
	if err != nil {
		return err
	}
	if err := purgeDealRoomSeatTraces(ctx, tx, seats); err != nil {
		return err
	}
	// The invitation's own audit image stores the address in plain text, and
	// the field-history projection cuts a record's timeline at its own newest
	// erase row. Without this the audit log hands the "erased" address
	// straight back — an erasure the record itself contradicts.
	return tombstoneCollateralScrubs(ctx, tx, "deal_room_participant", seats, reason, causePersonErasure)
}

// anonymizeDealRoomSeats wipes the subject's name and address from every Deal
// Room seat that carries one of their addresses, and revokes the seat so the
// access it stood for cannot be exercised again.
//
// It matches on ADDRESS because a seat holds no person id — see the file
// header.
//
// An address is a weaker key than a person id, so the question "could this wipe
// somebody else's seat" has to be answered rather than assumed. It cannot,
// because the erasure refuses outright when a second live person still holds
// one of these addresses (refuseRivalIdentifierHolders, run before this): a
// shared mailbox is a conflict the operator resolves by merging, not something
// this function silently guesses at. What remains is a seat invited under a
// different DISPLAY NAME on the subject's own address, which is the subject
// under another spelling of their name and is theirs to have erased.
//
// It returns the seats it wiped so the caller can tombstone each one's audit
// spine, the same way the lead twins are handled.
func anonymizeDealRoomSeats(ctx context.Context, tx pgx.Tx, emails []string) ([]ids.UUID, error) {
	if len(emails) == 0 {
		// No address means no seat can be resolved. Said as a return rather
		// than run: `email = ANY('{}')` matches nothing, but a caller reading
		// this function should not have to work that out.
		return nil, nil
	}
	rows, err := tx.Query(ctx, `
		UPDATE deal_room_participant
		   SET full_name = $1, email = $2,
		       revoked_at = coalesce(revoked_at, now())
		 WHERE email = ANY($3)
		RETURNING id`, erasedName, erasedEmail, emails)
	if err != nil {
		return nil, fmt.Errorf("anonymize deal room seats: %w", err)
	}
	defer rows.Close()
	var wiped []ids.UUID
	for rows.Next() {
		var id ids.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan anonymized deal room seat: %w", err)
		}
		wiped = append(wiped, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("anonymize deal room seats: %w", err)
	}
	return wiped, nil
}

// purgeDealRoomSeatTraces removes what the wiped seats would still betray about
// the subject even with their name gone.
//
// Two things do. A live session is a credential that still admits somebody, and
// an erased subject's access ending only when the token expires is access they
// did not consent to keep. An engagement row says WHEN this person signed in
// and WHICH documents they took — a behavioural record of the subject, useful
// to the seller only as a claim about a person who has asked to be forgotten.
//
// The invitations stay: they are the seller's record that access was granted
// and when, and they name no addressee beyond the now-wiped seat.
//
// The seat's AUDIT spine is tombstoned by eraseDealRoomSeats above, not here.
func purgeDealRoomSeatTraces(ctx context.Context, tx pgx.Tx, seats []ids.UUID) error {
	if len(seats) == 0 {
		return nil
	}
	if _, err := tx.Exec(ctx,
		`DELETE FROM deal_room_session WHERE participant_id = ANY($1)`, seats); err != nil {
		return fmt.Errorf("purge deal room sessions of an erased subject: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`DELETE FROM deal_room_engagement WHERE participant_id = ANY($1)`, seats); err != nil {
		return fmt.Errorf("purge deal room engagement of an erased subject: %w", err)
	}
	return nil
}
