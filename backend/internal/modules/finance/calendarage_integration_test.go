// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package finance

import (
	"testing"
	"time"
)

func TestOpenInvoiceLatenessAgreesWithItsCalendarDates(t *testing.T) {
	e := setupFinance(t)
	if _, err := e.store.SyncConnection(e.ctx, e.provider()); err != nil {
		t.Fatal(err)
	}
	zone, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 4, 0, 5, 0, 0, zone)
	store := NewStore(e.store.db, e.store.rates).WithClock(func() time.Time { return now })
	summary, err := store.SummaryFor(e.ctx, e.company)
	if err != nil {
		t.Fatal(err)
	}
	if summary.RecentInvoices == nil {
		t.Fatal("no synced invoices")
	}
	for _, invoice := range *summary.RecentInvoices {
		if invoice.DueAt != nil && invoice.DueAt.Format(time.DateOnly) == "2026-08-03" {
			if invoice.DaysLate == nil || *invoice.DaysLate != 1 {
				t.Fatalf("days late = %v, want 1 after local midnight", invoice.DaysLate)
			}
			return
		}
	}
	t.Fatal("the synced open invoice is missing")
}
