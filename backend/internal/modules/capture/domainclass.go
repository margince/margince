// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
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
	// Company names the company for DomainRoleCompany. It is empty when the
	// caller may not read that company, so the answer never discloses it.
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

// ClassifyAddressTx answers whether a full address is a contact's on file.
func ClassifyAddressTx(ctx context.Context, tx pgx.Tx, address string) (DomainClass, error) {
	rows, err := tx.Query(ctx, `
		SELECT e.contact_id FROM contact_email e
		 WHERE lower(e.email) = $1 AND e.archived_at IS NULL LIMIT 20`,
		strings.ToLower(strings.TrimSpace(address)))
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

func companyOfDomainTx(ctx context.Context, tx pgx.Tx, domain string) (DomainClass, error) {
	var id ids.UUID
	var name string
	err := tx.QueryRow(ctx, `
		SELECT c.id, c.display_name
		  FROM company_domain d JOIN company c ON c.id = d.company_id
		 WHERE d.domain = ANY($1) AND d.archived_at IS NULL AND c.archived_at IS NULL
		 LIMIT 1`, owndomain.Spellings(domain)).Scan(&id, &name)
	if errors.Is(err, pgx.ErrNoRows) {
		return DomainClass{}, nil
	}
	if err != nil {
		return DomainClass{}, fmt.Errorf("capture: looking up a company domain: %w", err)
	}
	// A caller who may not read companies still gets the question, without the name.
	if !auth.ReadGranted(ctx, "company") {
		return DomainClass{Role: DomainRoleCompany}, nil
	}
	visible, err := auth.VisibleTo(ctx, tx, "company", id)
	if err != nil {
		return DomainClass{}, err
	}
	if !visible {
		name = ""
	}
	return DomainClass{Role: DomainRoleCompany, Company: name}, nil
}
