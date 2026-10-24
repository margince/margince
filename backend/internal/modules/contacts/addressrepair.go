// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Repairing addresses stored before the splitter existed: the whole text in
// line 1 and every other part empty. Only text a writer derived is split. A
// company's line 1 must still equal its registered_address profile text, and a
// contact's must come from a business card. No edit of the record's own
// address fields may have happened since, by anybody.

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

// AddressRepairKind names the record whose addresses one repair step walks.
type AddressRepairKind struct {
	entity string
	// pageIDs is the next page of every record id, live or archived.
	pageIDs string
	// unsplit reads the six columns of one record that still holds its
	// derived text unsplit, and no row for any other record.
	unsplit string
	// count is how many records a walk inspects, read no further than its cap.
	count string
}

// CompanyAddressRepair and ContactAddressRepair are the two steps' kinds.
var (
	CompanyAddressRepair = AddressRepairKind{
		entity:  entityCompany,
		pageIDs: `SELECT id FROM company WHERE id > $1 ORDER BY id LIMIT $2`,
		unsplit: `SELECT address_line1, address_country FROM company c
		 WHERE c.id = $1 AND ` + unsplitColumns + `
		   AND EXISTS (SELECT 1 FROM company_profile_field f
		                WHERE f.company_id = c.id AND f.field = 'registered_address'
		                  AND btrim(f.value) = btrim(c.address_line1))
		   AND NOT EXISTS (SELECT 1 FROM audit_log a
		                    WHERE a.entity_type = 'company' AND a.entity_id = c.id AND a.after ? 'address_line1')`,
		count: `SELECT count(*) FROM (SELECT 1 FROM company LIMIT $1) inspected`,
	}
	ContactAddressRepair = AddressRepairKind{
		entity:  entityContact,
		pageIDs: `SELECT id FROM contact WHERE id > $1 ORDER BY id LIMIT $2`,
		unsplit: `SELECT address_line1, address_country FROM contact c
		 WHERE c.id = $1 AND ` + unsplitColumns + ` AND c.source = '` + vcardSource + `'
		   AND NOT EXISTS (SELECT 1 FROM audit_log a
		                    WHERE a.entity_type = 'contact' AND a.entity_id = c.id AND a.after ? 'address_line1')`,
		count: `SELECT count(*) FROM (SELECT 1 FROM contact LIMIT $1) inspected`,
	}
)

// unsplitColumns holds a line 1 and no other part except the country.
const unsplitColumns = `c.address_line1 IS NOT NULL AND c.address_line2 IS NULL AND c.address_region IS NULL
		   AND c.address_postal_code IS NULL AND c.address_city IS NULL`

// AddressRepairPage is one batch: every record id it inspected and how many
// addresses it split.
type AddressRepairPage struct {
	Scanned []ids.UUID
	Changed int
}

// RepairUnsplitAddressesTx inspects the next page of records after the cursor
// and splits each unsplit address the splitter accepts. A split address leaves
// the selection, so a second walk changes nothing. A refused one stays whole.
func RepairUnsplitAddressesTx(ctx context.Context, tx pgx.Tx, kind AddressRepairKind, after ids.UUID, limit int) (AddressRepairPage, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return AddressRepairPage{}, err
	}
	if err := requireAddressRead(ctx, kind); err != nil {
		return AddressRepairPage{}, err
	}
	rows, err := tx.Query(ctx, kind.pageIDs, after, limit)
	if err != nil {
		return AddressRepairPage{}, fmt.Errorf("contacts: paging %s addresses: %w", kind.entity, err)
	}
	scanned, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return AddressRepairPage{}, fmt.Errorf("contacts: reading %s ids: %w", kind.entity, err)
	}
	page := AddressRepairPage{Scanned: scanned}
	for _, id := range scanned {
		split, err := repairUnsplitAddress(ctx, tx, kind, id)
		if err != nil {
			return AddressRepairPage{}, err
		}
		if split {
			page.Changed++
		}
	}
	return page, nil
}

// repairUnsplitAddress splits one record's address under its row lock. The
// cheap read without the lock skips the many records that need nothing.
func repairUnsplitAddress(ctx context.Context, tx pgx.Tx, kind AddressRepairKind, id ids.UUID) (bool, error) {
	if _, _, ok, err := unsplitProposal(ctx, tx, kind, id); !ok || err != nil {
		return false, err
	}
	lock, err := lockAddressRow(ctx, tx, kind, id)
	if err != nil {
		return false, err
	}
	// Asked again under the lock: an edit committed since the first read wins.
	current, split, ok, err := unsplitProposal(ctx, tx, kind, id)
	if !ok || err != nil {
		return false, err
	}
	p := storekit.NewPatch()
	if err := setAddress(p, &current, &split, storekit.AuditTrail{}); err != nil {
		return false, err
	}
	if err := p.ApplyLocked(ctx, tx, lock); err != nil {
		return false, fmt.Errorf("contacts: splitting a %s address: %w", kind.entity, err)
	}
	return true, recordAddressRepair(ctx, tx, kind, id, p)
}

// requireAddressRead asks the read grant of the kind's record, named as a
// literal so the reader gates can see which grant guards which table.
func requireAddressRead(ctx context.Context, kind AddressRepairKind) error {
	if kind.entity == entityCompany {
		return auth.Require(ctx, entityCompany, principal.ActionRead)
	}
	return auth.Require(ctx, entityContact, principal.ActionRead)
}

// lockAddressRow names each table as a literal, which the table-ownership
// gate attributes to this package.
func lockAddressRow(ctx context.Context, tx pgx.Tx, kind AddressRepairKind, id ids.UUID) (storekit.RowLock, error) {
	if kind.entity == entityCompany {
		return storekit.LockRow(ctx, tx, entityCompany, id, storekit.IncludeArchived)
	}
	return storekit.LockRow(ctx, tx, entityContact, id, storekit.IncludeArchived)
}

// unsplitProposal reads a record's unsplit address and its split. A stored
// country the text contradicts is never replaced, so that address stays whole.
func unsplitProposal(ctx context.Context, tx pgx.Tx, kind AddressRepairKind, id ids.UUID) (current, split crmcontracts.Address, ok bool, err error) {
	var line1 string
	err = tx.QueryRow(ctx, kind.unsplit, id).Scan(&line1, &current.Country)
	if errors.Is(err, pgx.ErrNoRows) {
		return current, split, false, nil
	}
	if err != nil {
		return current, split, false, fmt.Errorf("contacts: reading a %s address to split: %w", kind.entity, err)
	}
	current.Line1 = &line1
	if split, ok = SplitAddressText(line1); !ok {
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

// recordAddressRepair writes the audit row and the update event an edit of the
// same record writes.
func recordAddressRepair(ctx context.Context, tx pgx.Tx, kind AddressRepairKind, id ids.UUID, p *storekit.Patch) error {
	if kind.entity == entityCompany {
		return recordCompanyUpdate(ctx, tx, ids.From[ids.CompanyKind](id), p, UpdateCompanyInput{}, "")
	}
	auditID, err := storekit.AuditWithTrail(ctx, tx, storekit.AuditTrail{}, entityContact, id, p.Before(), p.After())
	if err != nil {
		return fmt.Errorf("audit contact address split: %w", err)
	}
	if err := storekit.EmitEvent(ctx, tx, auditID, id, crmcontracts.PublicEventContactUpdated{ChangedFields: p.After()}); err != nil {
		return fmt.Errorf("emit contact.updated: %w", err)
	}
	return nil
}

// CountAddressRepairRows counts, up to limit+1, the records a repair walk
// inspects, so an estimate never reads past its cap.
func CountAddressRepairRows(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, kind AddressRepairKind, limit int64,
) (int64, error) {
	if err := requireAddressRead(ctx, kind); err != nil {
		return 0, err
	}
	var count int64
	if err := q.QueryRow(ctx, kind.count, limit+1).Scan(&count); err != nil {
		return 0, fmt.Errorf("contacts: counting %s addresses to inspect: %w", kind.entity, err)
	}
	return count, nil
}
