// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Repairing company addresses stored before the splitter existed: the whole
// text in line 1 and every other part empty. Only text a writer derived is
// split. Line 1 must still equal the registered_address profile text. No edit
// of the company's own address fields may have happened since.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// unsplitCompanyAddress reads one company that still holds its derived text
// unsplit, and no row for any other company.
const unsplitCompanyAddress = `SELECT address_line1, address_country FROM company c
	 WHERE c.id = $1 AND c.address_line1 IS NOT NULL AND c.address_line2 IS NULL AND c.address_region IS NULL
	   AND c.address_postal_code IS NULL AND c.address_city IS NULL
	   AND EXISTS (SELECT 1 FROM company_profile_field f
	                WHERE f.company_id = c.id AND f.field = 'registered_address'
	                  AND btrim(f.value) = btrim(c.address_line1))
	   AND NOT EXISTS (SELECT 1 FROM audit_log a
	                    WHERE a.entity_type = 'company' AND a.entity_id = c.id AND a.after ? 'address_line1')`

// AddressRepairPage is one batch: every company id it inspected and how many
// addresses it split.
type AddressRepairPage struct {
	Scanned []ids.UUID
	Changed int
}

// RepairUnsplitCompanyAddressesTx inspects the next page of companies after the
// cursor and splits each unsplit address the splitter accepts. A split address
// leaves the selection, so a second walk changes nothing. A refused one stays
// whole.
func RepairUnsplitCompanyAddressesTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int) (AddressRepairPage, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return AddressRepairPage{}, err
	}
	if err := auth.Require(ctx, entityCompany, principal.ActionRead); err != nil {
		return AddressRepairPage{}, err
	}
	rows, err := tx.Query(ctx, `SELECT id FROM company WHERE id > $1 ORDER BY id LIMIT $2`, after, limit)
	if err != nil {
		return AddressRepairPage{}, fmt.Errorf("contacts: paging company addresses: %w", err)
	}
	scanned, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return AddressRepairPage{}, fmt.Errorf("contacts: reading company ids: %w", err)
	}
	page := AddressRepairPage{Scanned: scanned}
	for _, id := range scanned {
		split, err := repairUnsplitCompanyAddress(ctx, tx, ids.From[ids.CompanyKind](id))
		if err != nil {
			return AddressRepairPage{}, err
		}
		if split {
			page.Changed++
		}
	}
	return page, nil
}

// repairUnsplitCompanyAddress splits one company's address under its row
// lock. The cheap read without the lock skips the many companies that need
// nothing.
func repairUnsplitCompanyAddress(ctx context.Context, tx pgx.Tx, id ids.CompanyID) (bool, error) {
	if _, _, ok, err := unsplitProposal(ctx, tx, id); !ok || err != nil {
		return false, err
	}
	lock, err := storekit.LockRow(ctx, tx, entityCompany, id.UUID, storekit.IncludeArchived)
	if err != nil {
		return false, err
	}
	// Asked again under the lock: an edit committed since the first read wins.
	current, split, ok, err := unsplitProposal(ctx, tx, id)
	if !ok || err != nil {
		return false, err
	}
	p := storekit.NewPatch()
	if err := setAddress(p, &current, &split, storekit.AuditTrail{}); err != nil {
		return false, err
	}
	if err := p.ApplyLocked(ctx, tx, lock); err != nil {
		return false, fmt.Errorf("contacts: splitting a company address: %w", err)
	}
	return true, recordCompanyUpdate(ctx, tx, id, p, UpdateCompanyInput{}, "")
}

// unsplitProposal reads a company's unsplit address and its split. Nobody
// reviews this split, so it takes the stricter rule. A stored country the text
// contradicts is never replaced, so that address stays whole.
func unsplitProposal(ctx context.Context, tx pgx.Tx, id ids.CompanyID) (current, split crmcontracts.Address, ok bool, err error) {
	var line1 string
	err = tx.QueryRow(ctx, unsplitCompanyAddress, id).Scan(&line1, &current.Country)
	if errors.Is(err, pgx.ErrNoRows) {
		return current, split, false, nil
	}
	if err != nil {
		return current, split, false, fmt.Errorf("contacts: reading a company address to split: %w", err)
	}
	current.Line1 = &line1
	if split, ok = splitAddressBy(line1, splitWithStreetEvidence); !ok {
		return current, split, false, nil
	}
	switch {
	case current.Country == nil:
	case split.Country == nil:
		split.Country = current.Country
	case *split.Country != *current.Country:
		return current, split, false, nil
	}
	return current, split, true, nil
}

// CountCompanyAddressRepairRows counts, up to limit+1, the companies a repair
// walk inspects, so an estimate never reads past its cap.
func CountCompanyAddressRepairRows(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, limit int64,
) (int64, error) {
	if err := auth.Require(ctx, entityCompany, principal.ActionRead); err != nil {
		return 0, err
	}
	var count int64
	if err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT 1 FROM company LIMIT $1) inspected`, limit+1).Scan(&count); err != nil {
		return 0, fmt.Errorf("contacts: counting company addresses to inspect: %w", err)
	}
	return count, nil
}
