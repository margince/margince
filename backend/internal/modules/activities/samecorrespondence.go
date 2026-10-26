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
// A thread key is read from headers a sender writes, so it is not enough. Anchor
// must be proven (correspondence.ProvenSQL), or it binds no other row. Row must
// be on anchor's thread and be one of two kinds of mail:
//   - our own mail to anchor's counterparty or to anyone at its company domain;
//   - mail from that counterparty or company domain whose sender was
//     authenticated.
//
// companyDomain is a placeholder holding CorrespondenceDomainTx's answer. An
// empty answer turns the company arm off. Aliases are compile-time literals.
func SameCorrespondenceSQL(row, anchor, companyDomain string) string {
	sameParty := row + `.counterparty_email = ` + anchor + `.counterparty_email`
	sameCompany := `(` + companyDomain + ` <> '' AND EXISTS (SELECT 1 FROM activity_participant counterparty
          WHERE counterparty.activity_id = ` + row + `.id AND lower(counterparty.address) = ` + row + `.counterparty_email
            AND ` + owndomain.CoversSQL("counterparty.address_domain_folded", companyDomain) + `))`
	return `(` + correspondence.ThreadSQL(row, anchor) + `
     AND coalesce(` + anchor + `.counterparty_email, '') <> '' AND ` + correspondence.ProvenSQL(anchor) + `
     AND (` + correspondence.OurOutboundSQL(row) + ` OR ` + correspondence.AuthenticatedInboundSQL(row) + `)
     AND (` + sameParty + ` OR ` + sameCompany + `))`
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
