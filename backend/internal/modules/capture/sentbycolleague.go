// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// asSentByColleagueTx re-reads a colleague's mail to an outside party, received
// as a copy in this seat's mailbox, as the workspace's own outbound mail.
//
// Filed inbound, the copy made the customer's question look unanswered and
// stood as a question owed to the colleague. The From must be a seat's exact
// login address, never a domain: a domain names every mailbox at it. And the
// receiving server must have authenticated it (fromAuthenticated), since a
// sender types From. The row is still not attested: no provider filed it as
// sent. It names the colleague in SentByColleague, which no correspondence
// gate reads.
//
// The counterparty is the ladder's stand-in, picked among the recipients only.
// rec.Addresses also lists every From address, so a co-author beside the
// colleague would otherwise stand in for the customer. Past the participant cap
// every recipient is withheld, so the withheld parties are candidates too.
func (s *Sink) asSentByColleagueTx(
	ctx context.Context, tx pgx.Tx, rec connector.NormalizedRecord, fields ActivityFields,
) (connector.NormalizedRecord, ActivityFields, error) {
	cp := rec.Counterparty
	if fields.Kind != kindEmail || fields.HasCalendarPart || cp.Direction != connector.DirectionInbound ||
		!fromAuthenticated(rec, cp.Email) {
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
	recipients.Addresses = make([]string, 0, len(rec.Participants)+len(rec.WithheldParties))
	for _, p := range slices.Concat(rec.Participants, rec.WithheldParties) {
		if foldAddress(p.Email) != from {
			recipients.Addresses = append(recipients.Addresses, p.Email)
		}
	}
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

// colleagueTx is the seat, other than the acting one, whose login address is
// address; zero when there is none. Invited and deactivated seats count. Most
// colleagues are invited and never sign in. A departed one's mail was still
// the workspace's mail. An archived seat does not count.
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
// colleague's mail is from the colleague. The seat whose mailbox held the copy
// received it, and that participant row lets the seat read it.
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

// fromAuthenticated reports that Gmail's receiving server passed DMARC for the
// domain of from. Gmail writes its result, signed mx.google.com, above every
// header the sender submitted, so the topmost one is Gmail's own. Other
// transports answer false: Microsoft writes no server name, and an IMAP
// server is whatever the seat chose.
func fromAuthenticated(rec connector.NormalizedRecord, from string) bool {
	auth := rec.SenderAuthentication
	domain := domainOfAddress(from)
	return rec.CapturedBy == "connector:"+providerGmail && auth.Server == gmailAuthServer &&
		domain != "" && strings.EqualFold(auth.DMARCPassDomain, domain)
}

// gmailAuthServer is the authserv-id Gmail signs Authentication-Results with.
const gmailAuthServer = "mx.google.com"
