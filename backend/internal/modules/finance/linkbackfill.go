// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package finance

// Naming the customer link on a row mirrored before rows named one. The sync
// names it on every row of a live link; this reaches the rows of a customer
// already unlinked, which no sync reads again.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// MirrorTable is one table of mirrored rows that name a customer link.
type MirrorTable int

// The mirrored tables a backfill walks.
const (
	MirrorInvoices MirrorTable = iota
	MirrorPayments
)

// tableAndEntity answers the table name and the audit entity of t. Both are
// compile-time literals.
func (t MirrorTable) tableAndEntity() (string, string) {
	if t == MirrorPayments {
		return "finance_payment", entityPayment
	}
	return "finance_invoice", entityInvoice
}

// CountUnlinkedMirrorRows counts the rows of t that name no customer link,
// reading at most maximum+1.
func CountUnlinkedMirrorRows(ctx context.Context, q datatask.Querier, t MirrorTable, maximum int64) (int64, error) {
	table, _ := t.tableAndEntity()
	var count int64
	if err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT 1 FROM `+table+`
		 WHERE customer_link_id IS NULL LIMIT $1) unlinked`, maximum+1).Scan(&count); err != nil {
		return 0, fmt.Errorf("count the %s rows that name no customer link: %w", table, err)
	}
	return count, nil
}

// BackfillCustomerLinksTx reads the next limit rows of t after the cursor and
// names the customer link on each that names none. It answers every id it
// inspected, for the cursor, and how many it named.
//
// A row carries no source customer id. Its link is one that maps the row's
// connection onto the row's company: the live one first, then the newest
// archived one. Attribution does not move, and a row whose connection never
// mapped its company keeps NULL.
func BackfillCustomerLinksTx(ctx context.Context, tx pgx.Tx, t MirrorTable, after ids.UUID, limit int) ([]ids.UUID, int, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return nil, 0, err
	}
	table, entity := t.tableAndEntity()
	rows, err := tx.Query(ctx, `
		SELECT r.id, CASE WHEN r.customer_link_id IS NULL THEN
		         (SELECT l.id FROM finance_customer_link l
		           WHERE l.connection_id = r.connection_id AND l.company_id = r.company_id
		           ORDER BY l.archived_at IS NULL DESC, l.created_at DESC, l.id DESC LIMIT 1) END
		  FROM `+table+` r
		 WHERE r.id > $1 ORDER BY r.id LIMIT $2 FOR UPDATE OF r`, after, limit)
	if err != nil {
		return nil, 0, fmt.Errorf("read the %s rows to link: %w", table, err)
	}
	type naming struct{ row, link ids.UUID }
	var inspected []ids.UUID
	var named []naming
	for rows.Next() {
		var id ids.UUID
		var link *ids.UUID
		if err := rows.Scan(&id, &link); err != nil {
			rows.Close()
			return nil, 0, fmt.Errorf("read the %s rows to link: %w", table, err)
		}
		inspected = append(inspected, id)
		if link != nil {
			named = append(named, naming{row: id, link: *link})
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("read the %s rows to link: %w", table, err)
	}
	for _, each := range named {
		id, link := each.row, each.link
		if _, err := tx.Exec(ctx, `UPDATE `+table+` SET customer_link_id = $2 WHERE id = $1 AND customer_link_id IS NULL`, id, link); err != nil {
			return nil, 0, fmt.Errorf("name the customer link on %s %s: %w", table, id, err)
		}
		if _, err := storekit.Audit(ctx, tx, "update", entity, id,
			map[string]any{"customer_link_id": nil}, map[string]any{"customer_link_id": link}); err != nil {
			return nil, 0, fmt.Errorf("record the customer link on %s %s: %w", table, id, err)
		}
	}
	return inspected, len(named), nil
}
