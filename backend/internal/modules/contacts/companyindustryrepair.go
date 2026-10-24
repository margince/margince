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

// IndustryRemapPage is one batch of the stored-data repair: every company row
// it inspected and how many it changed.
type IndustryRemapPage struct {
	Scanned []ids.UUID
	Changed int
}

// companiesWithLegacyIndustry selects the rows that still hold text instead of
// a code, live and archived. A row holding a code is excluded here, so a second
// run inspects nothing.
const companiesWithLegacyIndustry = `industry IS NOT NULL AND industry <> ALL($1::text[])`

// RemapCompanyIndustriesTx gives each company holding industry text the code
// that text names, or no code with the text kept in the description. It writes
// through the same patch and the same audit and event tail as a company edit,
// so the search index and every listener see an ordinary update. It leaves
// field_provenance alone, so a description a contact wrote stays theirs.
func RemapCompanyIndustriesTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int) (IndustryRemapPage, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return IndustryRemapPage{}, err
	}
	if err := auth.Require(ctx, entityCompany, principal.ActionRead); err != nil {
		return IndustryRemapPage{}, err
	}
	rows, err := tx.Query(ctx,
		`SELECT id FROM company WHERE id > $2 AND `+companiesWithLegacyIndustry+` ORDER BY id LIMIT $3`,
		IndustryCodes(), after, limit)
	if err != nil {
		return IndustryRemapPage{}, fmt.Errorf("contacts: paging companies with industry text: %w", err)
	}
	page := IndustryRemapPage{}
	for rows.Next() {
		var id ids.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return IndustryRemapPage{}, fmt.Errorf("contacts: reading companies with industry text: %w", err)
		}
		page.Scanned = append(page.Scanned, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return IndustryRemapPage{}, fmt.Errorf("contacts: paging companies with industry text: %w", err)
	}
	for _, id := range page.Scanned {
		if err := remapCompanyIndustry(ctx, tx, ids.From[ids.CompanyKind](id)); err != nil {
			return IndustryRemapPage{}, err
		}
		page.Changed++
	}
	return page, nil
}

func remapCompanyIndustry(ctx context.Context, tx pgx.Tx, id ids.CompanyID) error {
	lock, err := storekit.LockRow(ctx, tx, "company", id.UUID, storekit.IncludeArchived)
	if err != nil {
		return err
	}
	current, err := readCompany(ctx, tx, id, storekit.IncludeArchived, nil)
	if err != nil {
		return fmt.Errorf("contacts: reading a company before its industry is mapped: %w", err)
	}
	if current.Industry == nil {
		return nil
	}
	raw := string(*current.Industry)
	p := storekit.NewPatch()
	write := resolveIndustryWrite(&raw)
	if write.code != nil {
		p.Set("industry", current.Industry, *write.code)
	} else {
		p.Set("industry", current.Industry, nil)
		if next, changed := withIndustrySource(current.Description, raw); changed {
			p.Set("description", current.Description, next)
		}
	}
	if err := p.ApplyLocked(ctx, tx, lock); err != nil {
		return fmt.Errorf("contacts: mapping a company's industry: %w", err)
	}
	return recordCompanyUpdate(ctx, tx, id, p, UpdateCompanyInput{}, "")
}

// CountCompaniesWithLegacyIndustry counts, up to limit+1, the rows
// RemapCompanyIndustriesTx would inspect, so an estimate never reads past its cap.
func CountCompaniesWithLegacyIndustry(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, limit int64,
) (int64, error) {
	if err := auth.Require(ctx, entityCompany, principal.ActionRead); err != nil {
		return 0, err
	}
	var count int64
	err := q.QueryRow(ctx, `SELECT count(*) FROM (
		SELECT 1 FROM company WHERE `+companiesWithLegacyIndustry+` LIMIT $2) candidates`,
		IndustryCodes(), limit+1).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("contacts: counting companies with industry text: %w", err)
	}
	return count, nil
}
