// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package finance

import (
	"testing"

	"github.com/jackc/pgx/v5"
)

// The source's word that an invoice is settled is final, however it was
// settled. A discount or write-off leaves payments short of the gross, and a
// settled dispute is still settled; the row must read paid and owe nothing.
func TestAnInvoiceTheSourceCallsSettledIsMirroredPaid(t *testing.T) {
	e := setupFinance(t)
	source := restatingSource(e.external)
	settled := source.ledger.Invoices[0].IssuedOn.AddDate(0, 0, 10)
	discounted := source.ledger.Invoices[0]
	discounted.ExternalID, discounted.FullyPaidAt = "INV-SETTLED-1", &settled
	disputed := discounted
	disputed.ExternalID, disputed.Number, disputed.Disputed = "INV-SETTLED-2", "2026-002", true
	source.ledger.Invoices = []SourceInvoice{discounted, disputed}
	source.ledger.Payments = nil

	if _, err := e.store.SyncConnection(e.ctx, source); err != nil {
		t.Fatalf("a sync over invoices the source calls settled failed: %v", err)
	}
	for _, externalID := range []string{discounted.ExternalID, disputed.ExternalID} {
		var status string
		var open int64
		if err := e.store.tx(e.ctx, func(tx pgx.Tx) error {
			return tx.QueryRow(e.ctx, `
				SELECT status, open_minor FROM finance_invoice
				 WHERE connection_id = $1 AND external_id = $2`,
				e.connection, externalID).Scan(&status, &open)
		}); err != nil {
			t.Fatalf("reading invoice %s: %v", externalID, err)
		}
		if status != statusPaid || open != 0 {
			t.Errorf("invoice %s reads %q with %d open, want paid with nothing open",
				externalID, status, open)
		}
	}
}
