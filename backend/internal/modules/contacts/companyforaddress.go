// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/freemail"
	"github.com/margince/margince/backend/internal/platform/owndomain"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// CompanyForAddress is the live, readable company whose domain covers the
// address's domain most narrowly. found is false when there is none.
//
// Which domains cover an address is the daily plant's rule; the choice among
// several is this read's own. Consumer mail and a suppressed domain never answer.
func (s *Store) CompanyForAddress(ctx context.Context, email string) (crmcontracts.ContactEmployer, bool, error) {
	if err := auth.Require(ctx, companyEntity, principal.ActionRead); err != nil {
		return crmcontracts.ContactEmployer{}, false, err
	}
	host := owndomain.OfAddress(email)
	base, ok := freemail.Hostname(host)
	if !ok {
		return crmcontracts.ContactEmployer{}, false, nil
	}
	var out crmcontracts.ContactEmployer
	var found bool
	err := s.txSnapshot(ctx, func(tx pgx.Tx) error {
		matcher, err := s.consumerMailMatcher(ctx, tx)
		if err != nil || matcher.IsConsumer(host) {
			return err
		}
		suppressed, err := domainSuppressedTx(ctx, tx, base)
		if err != nil || suppressed {
			return err
		}
		covering, err := companiesCoveringTx(ctx, tx, host)
		if err != nil {
			return err
		}
		for _, company := range covering {
			if !matcher.IsConsumer(company.domain) {
				out, found = company.employer, true
				return nil
			}
		}
		return nil
	})
	if err != nil {
		return crmcontracts.ContactEmployer{}, false, err
	}
	return out, found, nil
}

type coveringCompany struct {
	employer crmcontracts.ContactEmployer
	domain   string
}

// companiesCoveringTx reads the live companies the caller can read whose live
// domain covers host, the narrowest domain first.
func companiesCoveringTx(ctx context.Context, tx pgx.Tx, host string) ([]coveringCompany, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	hostPos := arg(host)
	scope, err := auth.ScopeClauseFor(ctx, companyEntity, companyEntity, arg)
	if err != nil {
		return nil, err
	}
	if scope == "" {
		scope = scopeAllRows
	}
	rows, err := tx.Query(ctx, `
		SELECT company.id, company.display_name, company.logo_object_key, d.domain_folded
		  FROM (SELECT `+storekit.SQLf("$%d::text", hostPos)+` AS domain_folded) asked
		  JOIN company_domain d ON `+owndomain.CoversSQL("asked.domain_folded", "d.domain_folded")+`
		  JOIN company ON company.id = d.company_id
		 WHERE d.archived_at IS NULL AND d.domain_folded IS NOT NULL
		   AND company.archived_at IS NULL AND company.merged_into_id IS NULL
		   AND `+scope+`
		 ORDER BY length(d.domain_folded) DESC, company.id`, args...)
	if err != nil {
		return nil, fmt.Errorf("contacts: reading the companies on an address's domain: %w", err)
	}
	out, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (coveringCompany, error) {
		var one coveringCompany
		var companyID ids.UUID
		var logoKey *string
		if err := row.Scan(&companyID, &one.employer.CompanyName, &logoKey, &one.domain); err != nil {
			return one, err
		}
		one.employer.CompanyId = openapi_types.UUID(companyID)
		one.employer.LogoUrl = LogoURL(companyID, logoKey, LogoWide)
		return one, nil
	})
	if err != nil {
		return nil, fmt.Errorf("contacts: reading the companies on an address's domain: %w", err)
	}
	return out, nil
}
