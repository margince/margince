// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A merge cannot name a customer link on an invoice mirrored before rows named
// one, and the link check binds the move. So the merge refuses with a conflict
// that says what to do, and moves nothing.
func TestAMergeRefusesToMoveAnInvoiceThatNamesNoCustomerLink(t *testing.T) {
	e := Setup(t)
	store := financeStoreFor(e, testBooksSource{})
	admin := connectAndSync(t, e, store)
	src, tgt := seedMergePair(t, e, "Unlinked ledger")
	if _, err := store.LinkCustomer(admin, "CUST-A", src); err != nil {
		t.Fatal(err)
	}
	syncOnce(admin, t, store)
	unlinkBeforeTheCheck(t, src)

	_, err := e.Contacts.MergeCompany(admin, src, tgt, nil)
	fault, ok := httperr.Classify(err)
	if !ok || fault.Status != http.StatusConflict {
		t.Fatalf("merge = %v (fault %+v), want a 409 conflict", err, fault)
	}
	if n := invoicesOn(t, e, src); n != 1 {
		t.Errorf("the merged-away company holds %d invoices after the refused merge, want its 1", n)
	}
}

// unlinkBeforeTheCheck clears the customer link on company's invoices with the
// invoice link check lifted, then restores it from its catalog definition. That
// is how an upgraded database holds an invoice mirrored before the check.
func unlinkBeforeTheCheck(t *testing.T, company ids.CompanyID) {
	t.Helper()
	ctx := context.Background()
	err := pgx.BeginFunc(ctx, OwnerConn(t), func(tx pgx.Tx) error {
		var definition string
		if err := tx.QueryRow(ctx, `SELECT pg_get_constraintdef(oid) FROM pg_constraint
			 WHERE conrelid = 'finance_invoice'::regclass AND conname = 'finance_invoice_names_its_customer_link'`).Scan(&definition); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `ALTER TABLE finance_invoice DROP CONSTRAINT finance_invoice_names_its_customer_link`); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE finance_invoice SET customer_link_id = NULL WHERE company_id = $1`, company); err != nil {
			return err
		}
		// A deferred key check the UPDATE queued blocks the ALTER below.
		if _, err := tx.Exec(ctx, `SET CONSTRAINTS ALL IMMEDIATE`); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `ALTER TABLE finance_invoice ADD CONSTRAINT finance_invoice_names_its_customer_link `+definition)
		return err
	})
	if err != nil {
		t.Fatalf("unlinking the invoices as before the link check: %v", err)
	}
}
