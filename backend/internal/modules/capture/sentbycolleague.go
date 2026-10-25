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

	"github.com/margince/margince/backend/internal/platform/mailidentity"
	"github.com/margince/margince/backend/internal/platform/owndomain"
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
// gate reads. The stamp also lets that colleague open this one copy
// (auth.activityAttendanceArm).
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
	from := foldAddress(cp.Email)
	recipients := rec
	recipients.Addresses = make([]string, 0, len(rec.Participants)+len(rec.WithheldParties))
	for _, p := range slices.Concat(rec.Participants, rec.WithheldParties) {
		if foldAddress(p.Email) != from {
			recipients.Addresses = append(recipients.Addresses, p.Email)
		}
	}
	us, err := workspacePartiesTx(ctx, tx, recipients.Addresses...)
	if err != nil {
		return rec, fields, err
	}
	stand, err := s.standInSubject(ctx, tx, recipients, us, connector.DirectionOutbound)
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
//
// The address must be at one of the workspace's own trusted domains. A seat can
// be invited at any address, and the stamp lets that seat open the copy. A seat
// at a customer's domain must never be read as the sender of its mail.
func colleagueTx(ctx context.Context, tx pgx.Tx, address string) (ids.UUID, error) {
	own, err := TrustedOwnDomainsTx(ctx, tx)
	if err != nil || !own.Covers(address) {
		return ids.Nil, err
	}
	_, seat := capturePrincipal(ctx)
	var colleague ids.UUID
	err = tx.QueryRow(ctx, `SELECT id FROM app_user
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
	if err := stampOwnMailRecipient(ctx, tx, id, fields); err != nil {
		return err
	}
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
// domain of from. Gmail writes its result, under the authserv-id
// mx.google.com, above every header the sender submitted, so the topmost one
// is Gmail's own. The trust comes from that position; nothing here verifies a
// signature on the header. Other
// transports answer false: Microsoft writes no server name, and an IMAP
// server is whatever the seat chose.
func fromAuthenticated(rec connector.NormalizedRecord, from string) bool {
	auth := rec.SenderAuthentication
	domain := owndomain.OfAddress(from)
	return rec.CapturedBy == "connector:"+providerGmail && auth.Server == gmailAuthServer &&
		domain != "" && strings.EqualFold(auth.DMARCPassDomain, domain)
}

// inboundSenderAuthenticated is the activity's inbound_sender_authenticated
// stamp: an inbound message whose From the receiving server authenticated.
func inboundSenderAuthenticated(rec connector.NormalizedRecord, fields ActivityFields) bool {
	return fields.Direction == connector.DirectionInbound && fromAuthenticated(rec, rec.Counterparty.Email)
}

// senderIsMachine is the activity's sender_is_machine stamp: an inbound mail
// whose sender is a sending system. On inbound mail the counterparty is the
// From address; an outbound mail's sender is the mailbox owner.
func senderIsMachine(rec connector.NormalizedRecord, fields ActivityFields) bool {
	return mailidentity.InboundFromSendingSystem(fields.Kind, fields.Direction, rec.Counterparty.Email)
}

// gmailAuthServer is the authserv-id Gmail names in its Authentication-Results.
const gmailAuthServer = "mx.google.com"

// insertCounterpartyParticipant stamps the other end of a message. On mail the
// owner's provider attested as sent, the owner typed that address. So the
// colleague whose exact login it is gets bound, as StampFurtherParticipants
// binds a Cc on such a mail. They may then read their copy even when a
// members_only hold keeps the workspace out. No other seat address is recorded.
// The attendance arm reads only rows with a user_id.
func insertCounterpartyParticipant(
	ctx context.Context, tx pgx.Tx, activityID ids.ActivityID, role, kind string,
	counterparty connector.Counterparty, address, channelUserID string, owner ids.UUID, us workspaceParties,
) error {
	var bound *ids.UUID
	if kind == kindEmail && counterparty.SentByOwner() && address != "" {
		seat, err := recipientColleagueTx(ctx, tx, address, owner)
		if err != nil {
			return err
		}
		if !seat.IsZero() {
			bound = &seat
		}
	}
	if bound == nil && address != "" && us.Covers(address) {
		address = ""
		if channelUserID == "" {
			return nil
		}
	}
	return insertParticipant(ctx, tx, activityID, role, bound, nil, address, channelUserID)
}

// recipientColleagueTx is the seat, other than the owner, whose login is
// address; zero when there is none. An archived seat does not count.
func recipientColleagueTx(ctx context.Context, tx pgx.Tx, address string, owner ids.UUID) (ids.UUID, error) {
	var seat ids.UUID
	err := tx.QueryRow(ctx, `SELECT id FROM app_user WHERE lower(email) = $1 AND id <> $2 AND archived_at IS NULL`,
		address, owner).Scan(&seat)
	if errors.Is(err, pgx.ErrNoRows) {
		return ids.Nil, nil
	}
	if err != nil {
		return ids.Nil, fmt.Errorf("capture: reading whether a recipient is a colleague: %w", err)
	}
	return seat, nil
}
