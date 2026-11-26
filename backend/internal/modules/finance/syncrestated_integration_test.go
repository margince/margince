// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package finance

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Only a source that restates what it said reaches the mirror's update arms.
// The before image is what the row held, and the after is what the source now says.
//
// The fixture source never changes its mind, so it cannot reach these paths.
// The double stands at the provider seam only; the store, SQL and audit writer are real.
func TestARestatedSourceAuditsTheUpdateWithBothImages(t *testing.T) {
	e := setupFinance(t)
	source := restatingSource(e.external)
	mark := auditWatermark(e.ctx, t, e)

	if _, err := e.store.SyncConnection(e.ctx, source); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	created := auditRowsByEntity(e.ctx, t, e, mark)

	// A tax correction that keeps gross at 11900 (net 10000 to 11000, tax 1900 to
	// 900), plus a restated payment and a renamed customer. With gross fixed, an
	// audit image narrower than the change key would miss every column that moved.
	// Its row would then say money changed without saying what.
	source.customer.DisplayName = "Ledger GmbH (Hamburg)"
	source.ledger.Invoices[0].NetMinor = 11000
	source.ledger.Invoices[0].TaxMinor = 900
	source.ledger.Payments[0].AmountMinor = 6000
	if _, err := e.store.SyncConnection(e.ctx, source); err != nil {
		t.Fatalf("second sync: %v", err)
	}

	for _, entity := range []string{entityInvoice, entityPayment, entityExternalCustomer} {
		if got, want := auditRowsByEntity(e.ctx, t, e, mark)[entity], created[entity]+1; got != want {
			t.Errorf("%d audit rows for %s after the source restated it, want %d",
				got, entity, want)
		}
	}

	// The before image has to be what the row held, not the source's new
	// figures rendered twice. A writer that rebuilt it from the source would
	// pass every count above and record a change that did not happen.
	before, after := updateImages(e.ctx, t, e, entityInvoice, mark)
	if before["net_minor"] != float64(10000) || after["net_minor"] != float64(11000) {
		t.Errorf("the invoice's update reads net %v → %v, want 10000 → 11000",
			before["net_minor"], after["net_minor"])
	}
	if before["gross_minor"] != after["gross_minor"] {
		t.Errorf("gross moved (%v → %v) and this case is about the columns that move UNDER a fixed gross",
			before["gross_minor"], after["gross_minor"])
	}
	before, after = updateImages(e.ctx, t, e, entityPayment, mark)
	if before["amount_minor"] != float64(5000) || after["amount_minor"] != float64(6000) {
		t.Errorf("the payment's update reads %v → %v, want 5000 → 6000",
			before["amount_minor"], after["amount_minor"])
	}
}

// updateImages reads the one update row this run wrote for entityType, fenced on
// the watermark auditRowsByEntity uses. The package's tests share a database.
// An unfenced read could return another test's update, and assert its images.
func updateImages(
	ctx context.Context, t *testing.T, e *financeEnv, entityType string, since ids.UUID,
) (before, after map[string]any) {
	t.Helper()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		var beforeRaw, afterRaw []byte
		if err := tx.QueryRow(ctx, `
			SELECT before, after FROM audit_log
			 WHERE entity_type = $1 AND action = 'update' AND id > $2`,
			entityType, since).Scan(&beforeRaw, &afterRaw); err != nil {
			return err
		}
		if err := json.Unmarshal(beforeRaw, &before); err != nil {
			return err
		}
		return json.Unmarshal(afterRaw, &after)
	}); err != nil {
		t.Fatalf("reading the %s update's images: %v", entityType, err)
	}
	return before, after
}

// restatingSource is one customer with one invoice and one payment against it,
// held in fields the test rewrites between passes.
func restatingSource(externalID string) *restatingProvider {
	issued := time.Date(2026, 3, 2, 9, 0, 0, 0, time.UTC)
	due := issued.AddDate(0, 0, 30)
	return &restatingProvider{
		customer: SourceCustomer{ExternalID: externalID, DisplayName: "Ledger GmbH"},
		ledger: SourceLedger{
			Invoices: []SourceInvoice{{
				ExternalID: "INV-RESTATED-1", Number: "2026-001",
				IssuedOn: issued, DueOn: &due, Currency: "EUR",
				NetMinor: 10000, TaxMinor: 1900, GrossMinor: 11900, PaidMinor: 5000,
			}},
			Payments: []SourcePayment{{
				ExternalID: "PAY-RESTATED-1", InvoiceExternalID: "INV-RESTATED-1",
				PaidAt: issued, Currency: "EUR", AmountMinor: 5000,
			}},
		},
	}
}

// restatingProvider answers whatever its fields currently hold, so a test can
// change the source's mind between two passes.
type restatingProvider struct {
	customer SourceCustomer
	ledger   SourceLedger
}

func (p *restatingProvider) Name() string { return fixtureProviderName }

func (p *restatingProvider) Check(context.Context) error { return nil }

func (p *restatingProvider) Customers(context.Context) ([]SourceCustomer, error) {
	return []SourceCustomer{p.customer}, nil
}

func (p *restatingProvider) InvoicesFor(context.Context, string) (SourceLedger, error) {
	return p.ledger, nil
}

func (p *restatingProvider) InvoiceDocument(context.Context, string) (SourceDocument, error) {
	return SourceDocument{}, apperrors.ErrUnsupportedBySoR
}
