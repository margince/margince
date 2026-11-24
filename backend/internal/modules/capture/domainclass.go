// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/freemail"
	"github.com/margince/margince/backend/internal/platform/mailidentity"
	"github.com/margince/margince/backend/internal/platform/owndomain"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// DomainRole is what a domain or address already is to this workspace.
type DomainRole string

const (
	// DomainRoleNone is a domain nothing on file mentions.
	DomainRoleNone DomainRole = ""
	// DomainRoleOwn is a domain an admin or the own company vouched for.
	DomainRoleOwn DomainRole = "own"
	// DomainRoleConsumerShipped is consumer mail by the shipped baseline.
	DomainRoleConsumerShipped DomainRole = "consumer_shipped"
	// DomainRoleConsumerAdded is consumer mail only because the workspace added it.
	DomainRoleConsumerAdded DomainRole = "consumer_added"
	// DomainRoleCompany is the domain of a company on file.
	DomainRoleCompany DomainRole = "company"
	// DomainRoleContactAddress is the address of a contact on file.
	DomainRoleContactAddress DomainRole = "contact_address"
)

// IsConsumer reports whether the role is consumer mail, shipped or added.
func (r DomainRole) IsConsumer() bool {
	return r == DomainRoleConsumerShipped || r == DomainRoleConsumerAdded
}

// DomainClass is the answer to "what is this domain to us".
type DomainClass struct {
	Role DomainRole
	// Company names the company for DomainRoleCompany.
	Company string
}

// ClassifyDomainTx is the one answer to what a domain is to this workspace.
// The own-domain list, the consumer-mail list and the other-address list ask
// it. None of them judges a domain its own way.
func ClassifyDomainTx(ctx context.Context, tx pgx.Tx, domain string) (DomainClass, error) {
	domain = owndomain.Fold(domain)
	if domain == "" {
		return DomainClass{}, nil
	}
	own, err := TrustedOwnDomainsTx(ctx, tx)
	if err != nil {
		return DomainClass{}, err
	}
	if own.CoversDomain(domain) {
		return DomainClass{Role: DomainRoleOwn}, nil
	}
	consumer, err := MatcherTx(ctx, tx)
	if err != nil {
		return DomainClass{}, err
	}
	switch {
	case consumer.IsShipped(domain):
		return DomainClass{Role: DomainRoleConsumerShipped}, nil
	case consumer.IsConsumer(domain):
		return DomainClass{Role: DomainRoleConsumerAdded}, nil
	}
	return companyOfDomainTx(ctx, tx, domain)
}

// ClassifyAddressTx answers whether a full address is one the caller may see
// on a contact. An address on a contact the caller cannot read classifies as
// none, so the answer never confirms that the contact exists.
func ClassifyAddressTx(ctx context.Context, tx pgx.Tx, address string) (DomainClass, error) {
	rows, err := tx.Query(ctx, `
		SELECT e.contact_id FROM contact_email e
		 WHERE e.email = ANY($1) AND e.archived_at IS NULL LIMIT 20`,
		mailidentity.AddressSpellings(address))
	if err != nil {
		return DomainClass{}, fmt.Errorf("capture: looking up a contact address: %w", err)
	}
	contactIDs, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return DomainClass{}, fmt.Errorf("capture: looking up a contact address: %w", err)
	}
	for _, id := range contactIDs {
		if visible, err := auth.VisibleTo(ctx, tx, "contact", id); err != nil || visible {
			return DomainClass{Role: DomainRoleContactAddress}, err
		}
	}
	return DomainClass{}, nil
}

// companyOfDomainTx finds a company the caller may read whose domain equals the
// value, sits above it or below it. A company the caller cannot read classifies
// as none, so the answer never confirms that the company exists.
func companyOfDomainTx(ctx context.Context, tx pgx.Tx, domain string) (DomainClass, error) {
	if !auth.ReadGranted(ctx, "company") {
		return DomainClass{}, nil
	}
	held := append(owndomain.Spellings(domain), freemail.Ancestors(domain)...)
	rows, err := tx.Query(ctx, `
		SELECT c.id, c.display_name
		  FROM company_domain d JOIN company c ON c.id = d.company_id
		 WHERE (d.domain = ANY($1) OR right(d.domain, length($2) + 1) = '.' || $2)
		   AND d.archived_at IS NULL AND c.archived_at IS NULL
		 ORDER BY (d.domain = ANY($1)) DESC, d.domain, c.id
		 LIMIT 20`, held, owndomain.Fold(domain))
	if err != nil {
		return DomainClass{}, fmt.Errorf("capture: looking up a company domain: %w", err)
	}
	type match struct {
		id   ids.UUID
		name string
	}
	matches, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (match, error) {
		var m match
		return m, row.Scan(&m.id, &m.name)
	})
	if err != nil {
		return DomainClass{}, fmt.Errorf("capture: looking up a company domain: %w", err)
	}
	for _, m := range matches {
		if visible, err := auth.VisibleTo(ctx, tx, "company", m.id); err != nil || visible {
			return DomainClass{Role: DomainRoleCompany, Company: m.name}, err
		}
	}
	return DomainClass{}, nil
}
