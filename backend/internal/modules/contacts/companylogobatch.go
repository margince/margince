// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// CompanyLogoURLsBatch answers the wide logo URL of each of these companies
// that wears one, for THIS caller, inside the caller's own transaction — the
// URL a company record carries, spelled by LogoURL so the two cannot differ.
//
// The company's own read gate is re-derived rather than taken on the caller's
// word, as LivePartnerCompaniesBatch does and for its reason: an exported
// reader that trusts its caller is one call site away from leaking. Absent
// from the map means no logo this caller may see.
func CompanyLogoURLsBatch(ctx context.Context, tx pgx.Tx, companyIDs []ids.CompanyID) (map[ids.CompanyID]string, error) {
	if err := auth.Require(ctx, "company", principal.ActionRead); err != nil {
		return nil, err
	}
	logos := make(map[ids.CompanyID]string, len(companyIDs))
	if len(companyIDs) == 0 {
		return logos, nil
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	idsPos := arg(companyIDs)
	visible, err := visibleCompanySQL(ctx, "c", arg)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, storekit.SQLf(`
		SELECT c.id, c.logo_object_key
		  FROM company c
		 WHERE c.id = ANY($%d)
		   AND c.logo_object_key IS NOT NULL
		   AND %s`, idsPos, visible), args...)
	if err != nil {
		return nil, fmt.Errorf("contacts: reading company logos: %w", err)
	}
	type companyLogo struct {
		id  ids.CompanyID
		key string
	}
	found, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (companyLogo, error) {
		var logo companyLogo
		err := row.Scan(&logo.id, &logo.key)
		return logo, err
	})
	if err != nil {
		return nil, fmt.Errorf("contacts: collecting company logos: %w", err)
	}
	for _, logo := range found {
		if url := LogoURL(logo.id.UUID, &logo.key, LogoWide); url != nil {
			logos[logo.id] = *url
		}
	}
	return logos, nil
}
