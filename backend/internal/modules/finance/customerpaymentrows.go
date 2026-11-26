// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package finance

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type paymentRow struct {
	linkedAt                 time.Time
	companyID                ids.UUID
	companyName, lifecycle   string
	invoiceID                *ids.UUID
	number, currency, status *string
	due                      *time.Time
	open                     *int64
	disputed                 *bool
}

//nolint:ireturn // pgx owns the streaming cursor; its concrete implementation is not public.
func customerPaymentRows(ctx context.Context, tx pgx.Tx, connectionID ids.UUID) (pgx.Rows, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	connPos := arg(connectionID)
	visible, err := LedgerVisibleClause(ctx, "c", arg)
	if err != nil {
		return nil, err
	}
	if visible != "" {
		visible = " AND " + visible
	}
	// The live link must still name this company; a remapped customer waits
	// for its next sync rather than showing its old attribution.
	rows, err := tx.Query(ctx, storekit.SQLf(`
  SELECT c.id, c.display_name, c.status, l.created_at, i.id, i.number, i.currency,
         i.status, i.due_at, i.open_minor, i.disputed_at IS NOT NULL
    FROM company c
    JOIN finance_customer_link l ON l.company_id = c.id
         AND l.connection_id = $%d AND l.archived_at IS NULL
    LEFT JOIN finance_invoice i ON i.customer_link_id = l.id
         AND i.company_id = c.id AND i.connection_id = l.connection_id
         AND i.archived_at IS NULL AND i.open_minor > 0
   WHERE c.archived_at IS NULL %s
   ORDER BY i.due_at ASC NULLS LAST, i.id ASC`, connPos, visible), args...)
	if err != nil {
		return nil, fmt.Errorf("read scoped customer payments: %w", err)
	}
	return rows, nil
}

func scanPaymentRow(rows pgx.Rows) (paymentRow, error) {
	var row paymentRow
	err := rows.Scan(&row.companyID, &row.companyName, &row.lifecycle, &row.linkedAt, &row.invoiceID,
		&row.number, &row.currency, &row.status, &row.due, &row.open, &row.disputed)
	if err != nil {
		return paymentRow{}, fmt.Errorf("scan customer payment: %w", err)
	}
	return row, nil
}

func (row paymentRow) overdue(now time.Time) (crmcontracts.CustomerPaymentInvoice, bool) {
	if row.invoiceID == nil || row.currency == nil || row.status == nil || row.open == nil || row.due == nil {
		return crmcontracts.CustomerPaymentInvoice{}, false
	}
	// Reuse eligibility while leaving amounts in their invoice currency.
	inv := Invoice{
		Status: *row.status, OpenMinorBase: *row.open, DueOn: row.due,
		Disputed: row.disputed != nil && *row.disputed,
	}
	if !invoiceOverdue(inv, now) {
		return crmcontracts.CustomerPaymentInvoice{}, false
	}
	return crmcontracts.CustomerPaymentInvoice{
		InvoiceId: openapi_types.UUID(*row.invoiceID), CompanyId: openapi_types.UUID(row.companyID),
		CompanyName: row.companyName, Number: row.number, Currency: *row.currency,
		OpenMinor: *row.open, DueAt: openapi_types.Date{Time: *row.due}, DaysOverdue: daysBetween(*row.due, now),
	}, true
}
