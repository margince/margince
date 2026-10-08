// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// readAgainstTheSeatsAddressesTx settles what the seat's own addresses decide
// about a message before it is captured. The sighting comes FIRST, so a
// message that completes an alias's corroboration is judged under the claim it
// just proved rather than being the last one read as mail from a stranger.
func (s *Sink) readAgainstTheSeatsAddressesTx(
	ctx context.Context, tx pgx.Tx, seat ids.UUID, rec connector.NormalizedRecord, fields ActivityFields,
) (connector.NormalizedRecord, ActivityFields, error) {
	if err := s.noteAliasSightingTx(ctx, tx, seat, rec.DeliveredTo, rec.Source); err != nil {
		return rec, fields, err
	}
	return s.asSentFromOwnAddressTx(ctx, tx, rec, fields)
}

// asSentFromOwnAddressTx re-reads mail the seat sent from another address of
// theirs as the outbound mail it is.
//
// The connector compares From with the one address the grant names, so mail
// the seat sent from a former or second address arrives INBOUND with that
// address as its counterparty. The provider still filed it as sent, and the
// seat's own addresses are in reach here: the two together are the same proof
// the connector takes from the grant address. Exact addresses only — a domain
// claim names colleagues too, and a seat declares one without proving control.
//
// Only Gmail and Graph, where the PROVIDER filed the message and sends only
// from addresses the account verified. An IMAP \Sent folder is whatever the
// server the seat chose says it is, so its From proves nothing
// (SeatProvedAddressTx makes the same cut for the same reason).
//
// Without this, everybody the seat wrote to from that address reads as a
// stranger, and the notice duty for mail they already held is owed again.
func (s *Sink) asSentFromOwnAddressTx(
	ctx context.Context, tx pgx.Tx, rec connector.NormalizedRecord, fields ActivityFields,
) (connector.NormalizedRecord, ActivityFields, error) {
	cp := rec.Counterparty
	// An invitation is left as it is: whom the seat invited is not whom they
	// wrote to (mailmap.recordCounterparty).
	if !providerFiledTransport(rec.CapturedBy) || fields.Kind != kindEmail || fields.HasCalendarPart ||
		cp.Direction != connector.DirectionInbound || !cp.FiledAsSent() {
		return rec, fields, nil
	}
	self, err := ownerIdentitiesTx(ctx, tx)
	if err != nil {
		return rec, fields, err
	}
	if !self.CoversAddressExactly(cp.Email) {
		return rec, fields, nil
	}
	at := sentToAt(rec.Participants, self)
	if at < 0 {
		return rec, fields, nil
	}
	recipient := rec.Participants[at].Email
	rec.Participants = slices.Delete(slices.Clone(rec.Participants), at, at+1)
	rec.Counterparty = cp.AsSentBySeat(recipient, domainOfAddress(recipient))
	fields.Direction = connector.DirectionOutbound
	rec.Fields = fields
	return rec, fields, nil
}

// sentToAt is where in participants the seat's own mail names whom it was
// written to, or -1 when it names nobody but the seat. Participants are in
// header order, To before Cc, which is where the connector takes an outbound
// counterparty from as well.
func sentToAt(participants []connector.MessageParticipant, self SelfSet) int {
	return slices.IndexFunc(participants, func(p connector.MessageParticipant) bool {
		return p.Role != connector.ParticipantRoleBCC && p.Email != "" && !self.Covers(p.Email)
	})
}

// ProviderFiledMailTransports names the mail connectors whose sent filing the
// provider made, as captured_by spells them after `connector:`.
func ProviderFiledMailTransports() []string {
	return []string{providerGmail, providerGraph}
}

// providerFiledTransport reports a mail connector whose sent filing the provider
// made. capturedBy is the authenticated connector's own id (admitRecord).
func providerFiledTransport(capturedBy string) bool {
	name, ok := strings.CutPrefix(capturedBy, "connector:")
	return ok && slices.Contains(ProviderFiledMailTransports(), name)
}
