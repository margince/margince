// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/owndomain"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func (s *Sink) suggestiblePartiesTx(ctx context.Context, tx pgx.Tx, parties []admissionParty) (map[string]bool, error) {
	addresses := make([]string, 0, len(parties))
	for _, party := range parties {
		addresses = append(addresses, normalizeEmail(party.Email))
	}
	seats, err := storekit.WorkspaceSeatAddresses(ctx, tx)
	if err != nil {
		return nil, err
	}
	self, err := ownerIdentitiesTx(ctx, tx)
	if err != nil {
		return nil, err
	}
	own, err := ownDomainsTx(ctx, tx)
	if err != nil {
		return nil, err
	}
	erased, err := storekit.SuppressedEmails(ctx, tx, addresses)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT candidate.email FROM unnest($1::text[]) candidate(email)
 WHERE NOT EXISTS(SELECT 1 FROM contact_email email JOIN contact c ON c.id=email.contact_id
 WHERE lower(email.email)=candidate.email AND email.archived_at IS NULL AND c.archived_at IS NULL)
 AND NOT EXISTS(SELECT 1 FROM lead candidate_lead WHERE lower(candidate_lead.email)=candidate.email AND `+storekit.LiveLeadClause("candidate_lead")+`)
 AND (`+priorDispositionExpression("candidate.email", "$2")+`) NOT IN ('noise','rejected','suppressed','known_noncontact')`, addresses, noiseVerdictReach.Seconds())
	if err != nil {
		return nil, err
	}
	candidates, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, err
	}
	_, asking := capturePrincipal(ctx)
	eligible := map[string]bool{}
	for _, email := range candidates {
		cp := connector.Counterparty{Email: email, Domain: owndomain.OfAddress(email)}
		if !s.recordWorthy(cp) || self.Covers(email) || seats.Covers(email, asking) || own.CoversDomain(cp.Domain) || erased[email] {
			continue
		}
		if s.suggestionMachine(cp) {
			continue
		}

		eligible[email] = true
	}
	return eligible, nil
}

func (s *Sink) suggestionMachine(cp connector.Counterparty) bool {
	if s.transactional == nil {
		return false
	}
	suppressed, _ := s.transactional.Suppress(transactionalInput(cp))
	return suppressed
}
