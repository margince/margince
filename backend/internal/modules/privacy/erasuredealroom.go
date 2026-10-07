// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// Erasing a subject who was a buyer in a Deal Room.
//
// A room participant is the one place in this product where a named outside
// contact is stored WITHOUT a contact row: the seat carries a full name and an
// address of its own, because a buyer is invited by email long before anybody
// decides they are a contact. Erasure resolves a subject by their contact row
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
// The comments the SUBJECT wrote go with them, and this is the one place the seat and
// the text part company. The case for keeping a body is real — a buyer's sentence
// about a contract clause is the seller's record of a negotiation, not personal data
// merely because the subject typed it — but it rested on such a body being reached by
// the same free-text route every other body is, and no route reached this table at
// all. A safety net named in a comment and absent from the tree is worse than none,
// because the next author reads it and stops looking.
//
// So the row goes, and the cost is stated rather than hidden: the seller keeps their
// own side of the conversation and loses the buyer's. Nothing automatic can separate
// the clause from the contact inside one sentence, and of the two ways to be wrong,
// keeping an erased subject's words is the one the subject did not consent to.

import (
	"context"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// erasedEmail is the address a wiped seat carries. It is a syntactically valid
// address in the reserved example domain, because the column is NOT NULL and
// carries a lowercase CHECK: a tombstone that cannot be stored is not a
// tombstone. Nothing can be delivered to it.
// erasedEmail is the reserved address, read from the type that owns it: values refuses
// it at every parse, so no live record can hold what this writes.
const erasedEmail = values.ErasedEmail

// eraseDealRoomSeats is the whole Deal Room step of an erasure: wipe the seats
// carrying one of the subject's addresses, purge what those seats still betray,
// and tombstone each one's audit spine.
//
// It runs BEFORE deleteSubjectIdentifierRows, like the provider purge one step
// over: a seat holds no contact id and is resolved by address, which that delete
// destroys.
func eraseDealRoomSeats(ctx context.Context, tx pgx.Tx, emails []string, reason string) error {
	// A subject holding the tombstone address is refused rather than half-erased.
	// values.ParseEmail refuses that address now, so no contact written since can carry
	// it — but one written before can, and a seat is resolved by ADDRESS alone, so after
	// one erasure that address names every seat
	// any erasure has ever wiped. Proceeding would either destroy other subjects'
	// comments or skip this subject's own seat, leaving their sessions live; there is no
	// third answer, because the data no longer distinguishes them.
	if slices.Contains(emails, erasedEmail) {
		return fmt.Errorf("this subject's addresses include the one an erasure writes over "+
			"a Deal Room seat, which no erasure can then tell from an already-erased seat; "+
			"change that address on the contact record first: %w", apperrors.ErrConflict)
	}
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
	return tombstoneCollateralScrubs(ctx, tx, "deal_room_participant", seats, reason, causeContactErasure)
}

// anonymizeDealRoomSeats wipes the subject's name and address from every Deal
// Room seat that carries one of their addresses, and revokes the seat so the
// access it stood for cannot be exercised again.
//
// It matches on ADDRESS because a seat holds no contact id — see the file
// header.
//
// An address is a weaker key than a contact id, so the question "could this wipe
// somebody else's seat" has to be answered rather than assumed. It cannot,
// because the erasure refuses outright when a second live contact still holds
// one of these addresses (refuseRivalIdentifierHolders, run before this): a
// shared mailbox is a conflict the operator resolves by merging, not something
// this function silently guesses at. What remains is a seat invited under a
// different DISPLAY NAME on the subject's own address, which is the subject
// under another spelling of their name and is theirs to have erased.
//
// It returns the seats it wiped so the caller can tombstone each one's audit
// spine, the same way the lead twins are handled.
// It can match on the address safely because eraseDealRoomSeats refuses a subject who
// holds the tombstone address at all: once erased, every seat shares that one address,
// so it stops being a key and no predicate can tell this subject's seats from another
// erased subject's.
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
// Three things do. A live session is a credential that still admits somebody, and
// an erased subject's access ending only when the token expires is access they
// did not consent to keep. A comment is the subject's own words. An engagement row
// says WHEN this contact signed in
// and WHICH documents they took — a behavioural record of the subject, useful
// to the seller only as a claim about a contact who has asked to be forgotten.
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
	// The subject's own comments, which the anonymized seat keeps: the seat is wiped
	// rather than deleted, and the comment references it ON DELETE RESTRICT, so the
	// buyer's text outlives the erasure that was meant to reach it. Blanking is not
	// available -- body carries a non-empty CHECK -- and nothing references a comment,
	// so the row goes. The seller's own comments stay in the thread, the same split
	// the invitations above are kept under.
	if _, err := tx.Exec(ctx,
		`DELETE FROM deal_room_comment WHERE author_participant_id = ANY($1)`, seats); err != nil {
		return fmt.Errorf("purge deal room comments of an erased subject: %w", err)
	}
	return nil
}
