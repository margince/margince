// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// asSentByColleagueTx re-reads a colleague's mail to an outside party, received
// as a copy in this seat's mailbox, as the workspace's own outbound mail.
//
// Filed inbound, the copy made the customer's question look unanswered and
// stood as a question owed to the colleague. The From must be a seat's exact
// login address, never a domain: a domain names every mailbox at it.
// From is a header the sender types, so the row is not attested; it carries
// the colleague in SentByColleague instead, and no gate that trusts
// counterparty_outbound_attested trusts it.
//
// The counterparty is the ladder's stand-in for a colleague's mail, with the
// colleague's own address taken out of the candidates.
func (s *Sink) asSentByColleagueTx(
	ctx context.Context, tx pgx.Tx, rec connector.NormalizedRecord, fields ActivityFields,
) (connector.NormalizedRecord, ActivityFields, error) {
	cp := rec.Counterparty
	if fields.Kind != kindEmail || fields.HasCalendarPart || cp.Direction != connector.DirectionInbound || cp.Email == "" {
		return rec, fields, nil
	}
	colleague, err := colleagueTx(ctx, tx, cp.Email)
	if err != nil || colleague.IsZero() {
		return rec, fields, err
	}
	self, err := ownerIdentitiesTx(ctx, tx)
	if err != nil {
		return rec, fields, err
	}
	from := foldAddress(cp.Email)
	recipients := rec
	recipients.Addresses = slices.DeleteFunc(slices.Clone(rec.Addresses), func(a string) bool { return foldAddress(a) == from })
	stand, err := s.standInSubject(ctx, tx, recipients, self, connector.DirectionOutbound)
	if err != nil || stand.Email == "" {
		return rec, fields, err
	}
	// The counterparty is stamped as a participant of its own, as for the seat's
	// own alias mail.
	rec.Participants = slices.DeleteFunc(slices.Clone(rec.Participants), func(p connector.MessageParticipant) bool {
		return p.Role != connector.ParticipantRoleBCC && foldAddress(p.Email) == foldAddress(stand.Email)
	})
	rec.Counterparty = stand.WithDeclaredEmailMerge(cp.MayCorroborateByEmail())
	fields.Direction = connector.DirectionOutbound
	fields.SentByColleague = colleague
	rec.Fields = fields
	return rec, fields, nil
}

// colleagueTx is the seat other than the acting one whose login address is
// exactly address, or zero when there is none. Invited and deactivated seats
// count: most colleagues are invited and never sign in, and what a departed
// one wrote while here was still the workspace's mail. An archived seat does not.
func colleagueTx(ctx context.Context, tx pgx.Tx, address string) (ids.UUID, error) {
	_, seat := capturePrincipal(ctx)
	var colleague ids.UUID
	err := tx.QueryRow(ctx, `SELECT id FROM app_user
		 WHERE lower(email) = $1 AND id <> $2 AND archived_at IS NULL`, foldAddress(address), seat).Scan(&colleague)
	if errors.Is(err, pgx.ErrNoRows) {
		return ids.Nil, nil
	}
	if err != nil {
		return ids.Nil, fmt.Errorf("capture: reading whether a sender is a colleague: %w", err)
	}
	return colleague, nil
}

// stampMessageEnds stamps who wrote and who received a captured message. A
// colleague's mail is from the colleague; the seat whose mailbox held the copy
// received it, and that participant row is what lets the seat read it.
func stampMessageEnds(
	ctx context.Context, tx pgx.Tx, id ids.ActivityID, seat ids.UUID, fields ActivityFields, counterparty connector.Counterparty,
) error {
	if fields.SentByColleague.IsZero() {
		return stampCaptureParticipants(ctx, tx, id, seat, fields.Kind, fields.Direction, counterparty)
	}
	if err := stampCaptureParticipants(ctx, tx, id, fields.SentByColleague, fields.Kind, fields.Direction, counterparty); err != nil {
		return err
	}
	if seat.IsZero() {
		return nil
	}
	if err := insertParticipant(ctx, tx, id, roleTo, &seat, nil, "", ""); err != nil {
		return fmt.Errorf("capture: stamping the seat that held a colleague's mail: %w", err)
	}
	return nil
}
