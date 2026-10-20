// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type ForwardedTriageBinding struct {
	Domain       string
	CompanyID    ids.CompanyID
	ReadID       ids.UUID
	EffectiveURL string
	Reason       string
}

// ForwardedTriageBindings reports candidates for review; it never changes identity ownership.
// Scope is applied before pagination so a cursor cannot name a private domain.
func (s *Store) ForwardedTriageBindings(ctx context.Context, afterDomain string, limit int) ([]ForwardedTriageBinding, string, error) {
	if err := auth.Require(ctx, entityCompany, principal.ActionRead); err != nil {
		return nil, "", err
	}
	limit = storekit.ClampLimit(&limit)
	var found []ForwardedTriageBinding
	next := ""
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var args []any
		bind := func(value any) int { args = append(args, value); return len(args) }
		arg := func(value any) string { return fmt.Sprintf("$%d", bind(value)) }
		scope, err := auth.ScopeClauseFor(ctx, entityCompany, "c", bind)
		if err != nil {
			return err
		}
		if scope == "" {
			scope = "true"
		}
		query := `SELECT d.domain, d.company_id, r.id, r.pages
   FROM company_domain_disposition d JOIN company c ON c.id = d.company_id
   JOIN site_read r ON r.id = d.site_read_id
   WHERE d.domain > ` + arg(afterDomain) + ` AND c.source LIKE 'domain_triage:%'
    AND r.target_kind = 'domain_triage' AND r.status IN ('done','partial') AND ` + scope + `
   ORDER BY d.domain LIMIT ` + arg(limit)
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return err
		}
		type scannedBinding struct {
			binding ForwardedTriageBinding
			pages   []byte
		}
		var scanned []scannedBinding
		for rows.Next() {
			var row scannedBinding
			if err := rows.Scan(&row.binding.Domain, &row.binding.CompanyID, &row.binding.ReadID, &row.pages); err != nil {
				rows.Close()
				return err
			}
			scanned = append(scanned, row)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		if len(scanned) == limit {
			next = scanned[len(scanned)-1].binding.Domain
		}
		for _, row := range scanned {
			var pages []SiteReadPage
			if err := json.Unmarshal(row.pages, &pages); err != nil {
				return err
			}
			for _, page := range pages {
				if page.Kind == "home" && DomainIdentityForwarded(row.binding.Domain, page.URL) {
					row.binding.EffectiveURL = page.URL
					row.binding.Reason = PendingForwardedIdentity
					found = append(found, row.binding)
					break
				}
			}
		}
		return nil
	})
	return found, next, err
}
