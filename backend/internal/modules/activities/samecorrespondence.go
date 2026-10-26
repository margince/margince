// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/freemail"
	"github.com/margince/margince/backend/internal/platform/owndomain"
	"github.com/margince/margince/backend/internal/shared/kernel/correspondence"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// SameCorrespondenceSQL is the test that row belongs to anchor's correspondence.
// A thread key is read from headers a sender writes, so it is not enough. Row
// must be on anchor's thread, and:
//   - when anchor is proven (correspondence.ProvenSQL), row is our attested mail
//     to anchor's counterparty or its company domain, or authenticated mail from
//     them;
//   - when anchor is unproven, row is any mail with anchor's exact counterparty,
//     and, when deliveredTo is set, only if the reply goes to that counterparty.
//
// companyDomain is a placeholder holding CorrespondenceDomainTx's answer. An
// empty answer turns the company arm off. deliveredTo is a placeholder holding
// the folded address a reply is sent to, or "" when nothing is sent. Aliases
// are compile-time literals.
func SameCorrespondenceSQL(row, anchor, companyDomain, deliveredTo string) string {
	sameParty := row + `.counterparty_email = ` + anchor + `.counterparty_email`
	sameCompany := `(` + companyDomain + ` <> '' AND EXISTS (SELECT 1 FROM activity_participant counterparty
          WHERE counterparty.activity_id = ` + row + `.id AND lower(counterparty.address) = ` + row + `.counterparty_email
            AND ` + owndomain.CoversSQL("counterparty.address_domain_folded", companyDomain) + `))`
	proven := `coalesce(` + correspondence.ProvenSQL(anchor) + `, false)`
	unproven := `NOT ` + proven + ` AND ` + sameParty
	if deliveredTo != "" {
		unproven += ` AND ` + anchor + `.counterparty_email = ` + deliveredTo
	}
	return `(` + correspondence.ThreadSQL(row, anchor) + `
     AND coalesce(` + anchor + `.counterparty_email, '') <> ''
     AND ((` + proven + `
           AND (` + correspondence.OurOutboundSQL(row) + ` OR ` + correspondence.AuthenticatedInboundSQL(row) + `)
           AND (` + sameParty + ` OR ` + sameCompany + `))
       OR (` + unproven + `)))`
}

// CorrespondenceDomainTx is the company domain a counterparty's correspondence
// may widen to. It is "" for no valid domain, a consumer mail domain, one of the
// own domains, or the domain of a seat's mailbox.
func CorrespondenceDomainTx(ctx context.Context, tx pgx.Tx, counterparty string, consumer *freemail.Matcher, own []string) (string, error) {
	domain, ok := freemail.Hostname(owndomain.OfAddress(counterparty))
	if !ok || consumer.IsConsumer(domain) {
		return "", nil
	}
	seats, err := storekit.UnambiguousMailSeatIdentities(ctx, tx)
	if err != nil {
		return "", err
	}
	ours := slices.Clone(own)
	for _, addresses := range seats {
		for _, address := range addresses {
			ours = append(ours, owndomain.OfAddress(address))
		}
	}
	if owndomain.New(ours).CoversDomain(domain) {
		return "", nil
	}
	return domain, nil
}

// ConsumerMailReader builds the workspace's consumer-mail matcher on a
// transaction the caller owns. Compose injects capture's implementation.
type ConsumerMailReader func(context.Context, pgx.Tx) (*freemail.Matcher, error)

// WithConsumerMail wires the workspace's own consumer-mail list. Omitted, the
// shipped baseline decides.
func (s *Store) WithConsumerMail(read ConsumerMailReader) *Store {
	s.consumerMail = read
	return s
}

// correspondenceDomainTx is CorrespondenceDomain for anchor's counterparty. A
// store that cannot tell our own domains gives "", so nothing widens.
func (s *Store) correspondenceDomainTx(ctx context.Context, tx pgx.Tx, anchor ids.UUID) (string, error) {
	if s.ownDomains == nil {
		return "", nil
	}
	var counterparty string
	err := tx.QueryRow(ctx, `SELECT coalesce(counterparty_email, '') FROM activity WHERE id = $1 AND restricted_at IS NULL`, anchor).Scan(&counterparty)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("activities: reading who a conversation is with: %w", err)
	}
	own, err := s.ownDomainList(ctx, tx)
	if err != nil {
		return "", err
	}
	consumer := freemail.New(nil, nil)
	if s.consumerMail != nil {
		if consumer, err = s.consumerMail(ctx, tx); err != nil {
			return "", err
		}
	}
	return CorrespondenceDomainTx(ctx, tx, counterparty, consumer, own)
}

// WithConsumerMail is Store.WithConsumerMail for the store behind these handlers.
func (h Handlers) WithConsumerMail(read ConsumerMailReader) Handlers {
	h.store = h.store.WithConsumerMail(read)
	return h
}

// deliveredToSQL binds a reply's folded delivery address, or answers "" when
// the read sends nothing.
func deliveredToSQL(address *string, arg func(any) int) string {
	if address == nil {
		return ""
	}
	return sprintf("$%d::text", arg(correspondence.Fold(*address)))
}
