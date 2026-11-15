// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package finance

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type paymentPosition struct {
	Connection ids.UUID  `json:"c"`
	Due        time.Time `json:"d"`
	Invoice    ids.UUID  `json:"i"`
}

// CustomerPayments scopes every figure so hidden financial activity cannot affect counts.
func (s *Store) CustomerPayments(ctx context.Context, cursor *string, limit *int) (crmcontracts.CustomerPayments, error) {
	for _, object := range []string{"finance", "company"} {
		if err := auth.Require(ctx, object, principal.ActionRead); err != nil {
			return crmcontracts.CustomerPayments{}, err
		}
	}
	if limit != nil && *limit < 1 {
		return crmcontracts.CustomerPayments{}, httperr.Validation("limit", "out_of_range", "page size must be at least one")
	}
	after, err := paymentCursor(cursor)
	if err != nil {
		return crmcontracts.CustomerPayments{}, err
	}
	out := crmcontracts.CustomerPayments{
		State:  crmcontracts.FinanceSummaryStateNoConnection,
		Totals: []crmcontracts.CustomerPaymentTotal{}, Customers: []crmcontracts.CustomerPaymentCustomer{},
		Data: []crmcontracts.CustomerPaymentInvoice{},
	}
	err = s.tx(ctx, func(tx pgx.Tx) error {
		conn, connected, err := readConnection(ctx, tx)
		if err != nil || !connected {
			return err
		}
		if after.Connection != (ids.UUID{}) && after.Connection != conn.id {
			return httperr.Validation("cursor", "cursor_param_mismatch", "the accounting source changed; restart the list")
		}
		out.State, out.Provider, out.LastSyncedAt = connectionState(conn, s.now()), &conn.provider, conn.lastSuccessAt
		return s.readCustomerPayments(ctx, tx, conn.id, after, storekit.ClampLimit(limit), &out)
	})
	if err != nil {
		return crmcontracts.CustomerPayments{}, err
	}
	return out, nil
}

func paymentCursor(cursor *string) (paymentPosition, error) {
	if cursor == nil || *cursor == "" {
		return paymentPosition{}, nil
	}
	after, err := storekit.DecodeOpaque[paymentPosition](*cursor)
	if err != nil || after.Connection == (ids.UUID{}) || after.Invoice == (ids.UUID{}) || after.Due.IsZero() {
		return paymentPosition{}, &storekit.MalformedCursorError{}
	}
	return after, nil
}

func (s *Store) readCustomerPayments(ctx context.Context, tx pgx.Tx, connectionID ids.UUID,
	after paymentPosition, limit int, out *crmcontracts.CustomerPayments,
) error {
	rows, err := customerPaymentRows(ctx, tx, connectionID)
	if err != nil {
		return err
	}
	defer rows.Close()
	fold := newPaymentFold()
	now := s.now()
	for rows.Next() {
		row, err := scanPaymentRow(rows)
		if err != nil {
			return err
		}
		if !fold.admit(row, out) {
			continue
		}
		invoice, overdue := row.overdue(now)
		if !overdue {
			continue
		}
		if err := fold.add(invoice); err != nil {
			return err
		}
		if followsPayment(invoice, after) && len(out.Data) <= limit {
			out.Data = append(out.Data, invoice)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read customer payments: %w", err)
	}
	fold.fill(out, connectionID)
	out.AccessScope += now.Format("2006-01-02")
	if out.State == crmcontracts.FinanceSummaryStateSyncing {
		out.InvoiceCount, out.CompanyCount = 0, 0
		out.Totals, out.Customers = []crmcontracts.CustomerPaymentTotal{}, []crmcontracts.CustomerPaymentCustomer{}
		out.Data = []crmcontracts.CustomerPaymentInvoice{}
		return nil
	}
	if len(out.Data) > limit {
		out.Data = out.Data[:limit]
		last := out.Data[limit-1]
		token, err := storekit.EncodeOpaque(paymentPosition{connectionID, last.DueAt.Time, ids.UUID(last.InvoiceId)})
		if err != nil {
			return err
		}
		out.Page = crmcontracts.PageInfo{HasMore: true, NextCursor: &token}
	}
	return nil
}

func followsPayment(invoice crmcontracts.CustomerPaymentInvoice, after paymentPosition) bool {
	if after.Invoice == (ids.UUID{}) {
		return true
	}
	return invoice.DueAt.After(after.Due) ||
		(invoice.DueAt.Equal(after.Due) && invoice.InvoiceId.String() > after.Invoice.String())
}
