// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package finance

// The finance card's monthly series over a synced ledger. It covers twelve
// calendar months ending with the installation's own today, and each month
// totals the mirrored invoices issued in it.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// summaryIn reads the card with the installation in zone and the clock at now.
func (e *financeEnv) summaryIn(t *testing.T, zone string, now time.Time) crmcontracts.CompanyFinanceSummary {
	t.Helper()
	store := NewStore(e.store.db, e.store.baseCurrency, func(context.Context, pgx.Tx) (string, error) {
		return zone, nil
	}).WithClock(func() time.Time { return now })
	out, err := store.SummaryFor(e.ctx, e.company)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// mirroredByMonth totals the mirrored invoices per issue month straight off
// the rows, credit notes excluded and each credit taken off its invoice.
func (e *financeEnv) mirroredByMonth(t *testing.T, through time.Time) map[string][2]int64 {
	t.Helper()
	out := map[string][2]int64{}
	if err := e.store.tx(e.ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(e.ctx, `
			SELECT to_char(issued_at, 'YYYY-MM'), sum(net_minor - credited_minor)::bigint, count(*)
			  FROM finance_invoice
			 WHERE company_id = $1 AND credits_invoice_id IS NULL AND issued_at <= $2
			   AND status NOT IN ('draft', 'void')
			 GROUP BY 1`, e.company, through)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var month string
			var amount, count int64
			if err := rows.Scan(&month, &amount, &count); err != nil {
				return err
			}
			out[month] = [2]int64{amount, count}
		}
		return rows.Err()
	}); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestTheMonthlySeriesIsTwelveMonthsOfTheMirroredLedger(t *testing.T) {
	e := setupFinance(t)
	if _, err := e.store.SyncConnection(e.ctx, e.provider()); err != nil {
		t.Fatal(err)
	}

	card := e.summaryIn(t, "UTC", ledgerEpoch)
	if card.InvoicedByMonth == nil {
		t.Fatal("no monthly series on a synced, converted ledger")
	}
	series := *card.InvoicedByMonth
	if len(series) != InvoicedMonths {
		t.Fatalf("%d months, want %d", len(series), InvoicedMonths)
	}
	mirrored := e.mirroredByMonth(t, ledgerEpoch)
	invoiced := 0
	for i, m := range series {
		invoiced += m.Count
		want := ledgerEpoch.AddDate(0, i-(InvoicedMonths-1), 0).Format("2006-01")
		if m.Month != want {
			t.Fatalf("month %d is %s, want %s (oldest first, ending with today's)", i, m.Month, want)
		}
		if m.Currency != "EUR" {
			t.Errorf("%s is labelled %q, want the base currency EUR", m.Month, m.Currency)
		}
		if got := [2]int64{m.AmountMinor, int64(m.Count)}; got != mirrored[m.Month] {
			t.Errorf("%s = %d over %d invoices, the mirror holds %d over %d",
				m.Month, got[0], got[1], mirrored[m.Month][0], mirrored[m.Month][1])
		}
	}
	if invoiced == 0 {
		t.Fatal("the series counts no invoice, so the comparison above proved nothing")
	}
}

// The series ends on the installation's month, not UTC's: at noon UTC on
// 31 July it is already 1 August in a +14 zone.
func TestTheMonthlySeriesEndsOnTheInstallationsOwnMonth(t *testing.T) {
	e := setupFinance(t)
	if _, err := e.store.SyncConnection(e.ctx, e.provider()); err != nil {
		t.Fatal(err)
	}
	noon := time.Date(2026, time.July, 31, 12, 0, 0, 0, time.UTC)
	for zone, want := range map[string]string{"UTC": "2026-07", "Pacific/Kiritimati": "2026-08"} {
		card := e.summaryIn(t, zone, noon)
		if card.InvoicedByMonth == nil {
			t.Fatalf("%s: no monthly series", zone)
		}
		series := *card.InvoicedByMonth
		if got := series[len(series)-1].Month; got != want {
			t.Errorf("%s: the series ends on %s, want %s", zone, got, want)
		}
	}
}
