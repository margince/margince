// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package finance

import (
	"context"
	"fmt"
	"time"
)

// fixtureProviderName is the source the suite connects. The core ships no
// source of its own, so the suite brings one, at the accounting boundary.
const fixtureProviderName = "test_ledger"

// ledgerEpoch is the fixed day the fixture ledger is measured back from.
var ledgerEpoch = time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC)

const (
	fixtureInvoices    = 39
	fixtureCadenceDays = 14
	fixtureTermDays    = 30
	// fixtureOpenTail is how many of the newest invoices are unpaid. Four, so
	// the oldest of them is already past due at the epoch and the card has an
	// overdue figure to show.
	fixtureOpenTail = 4
)

// fixtureLateness cycles how many days past due a settled invoice was paid.
var fixtureLateness = []int{-3, 2, 7, 12, 0}

// fixtureSource answers one fixed eighteen-month ledger for any customer: a
// fortnightly invoice, most settled, one disputed, an open tail and one credit
// note. It reads no clock, so a second pass sees the same source.
type fixtureSource struct{}

func (fixtureSource) Name() string                { return fixtureProviderName }
func (fixtureSource) Check(context.Context) error { return nil }
func (fixtureSource) Customers(context.Context) ([]SourceCustomer, error) {
	return nil, nil
}

func (fixtureSource) InvoicesFor(_ context.Context, customer string) (SourceLedger, error) {
	var out SourceLedger
	for period := fixtureInvoices - 1; period >= 0; period-- {
		n := fixtureInvoices - period
		issued := ledgerEpoch.AddDate(0, 0, -period*fixtureCadenceDays)
		due := issued.AddDate(0, 0, fixtureTermDays)
		net := int64(1000+n*37) * 100
		invoice := SourceInvoice{
			ExternalID: fmt.Sprintf("%s-INV-%04d", customer, n),
			Number:     fmt.Sprintf("INV-%03d", n),
			IssuedOn:   issued, DueOn: &due, Currency: "EUR",
			NetMinor: net, TaxMinor: net / 5, GrossMinor: net + net/5,
		}
		switch {
		case period < fixtureOpenTail:
		case n == 20:
			invoice.Disputed = true
		default:
			paid := due.AddDate(0, 0, fixtureLateness[n%len(fixtureLateness)])
			invoice.FullyPaidAt, invoice.PaidMinor = &paid, invoice.GrossMinor
			out.Payments = append(out.Payments, SourcePayment{
				ExternalID: invoice.ExternalID + "-PMT", InvoiceExternalID: invoice.ExternalID,
				PaidAt: paid, Currency: "EUR", AmountMinor: invoice.GrossMinor,
			})
		}
		out.Invoices = append(out.Invoices, invoice)
	}
	first := out.Invoices[0]
	out.Invoices = append(out.Invoices, SourceInvoice{
		ExternalID: customer + "-CN-1", Number: "CN-" + first.Number,
		IssuedOn: first.IssuedOn.AddDate(0, 0, fixtureCadenceDays), Currency: "EUR",
		NetMinor: first.NetMinor / 4, GrossMinor: first.NetMinor / 4,
		CreditsExternalID: first.ExternalID,
	})
	return out, nil
}
