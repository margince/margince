// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package financedemo

// The generator. The seed is sha256(connection | customer), and every date is
// measured back from the connection's creation day, never from the clock. A
// sync on a later day gets the same records, and the mirror writes nothing.
//
// The anchor is stored, not a constant: the card's figures fold over windows
// measured back from today, which a constant would drift out of.

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/margince/margince/backend/pkg/extension/accounting"
)

const (
	// Thirty-nine fortnights is eighteen months. Fortnightly billing puts about
	// a dozen settled invoices inside the 180-day timeliness window, well past
	// the five the median needs.
	invoiceCount        = 39
	cadenceDays         = 14
	creditNoteDelayDays = 14
	paymentTermDays     = 30
	vatPercent          = 19
	amountFloor         = 800
	amountSpread        = 9000
)

// archetype is how a customer pays. Three, because the card's purpose is
// telling them apart.
type archetype struct {
	// lateLow and lateHigh bound the days past due a customer settles in; a
	// negative low end means they sometimes pay early.
	lateLow, lateHigh int
	// disputeRate is per thousand invoices.
	disputeRate int
	// openTail is how many of the newest invoices are still unpaid.
	openTail int
}

var archetypes = []archetype{
	{lateLow: -6, lateHigh: 2, disputeRate: 0, openTail: 1},
	{lateLow: -2, lateHigh: 12, disputeRate: 30, openTail: 2},
	{lateLow: 5, lateHigh: 38, disputeRate: 40, openTail: 3},
}

func generate(conn accounting.Connection, customerID string) (accounting.Ledger, error) {
	if customerID == "" {
		return accounting.Ledger{}, errors.New("finance-demo: a ledger needs a customer id")
	}
	anchor := conn.ConnectedAt.UTC().Truncate(24 * time.Hour)
	// #nosec G404 -- a seeded PCG is the requirement: the same customer must get
	// the same ledger on every run, and nothing here is a secret.
	rng := rand.New(rand.NewPCG(seedFor(conn.ID, customerID)))
	kind := archetypes[rng.IntN(len(archetypes))]
	const currency = "EUR"

	var out accounting.Ledger
	// Oldest first, so the open tail is the recent end.
	for period := invoiceCount - 1; period >= 0; period-- {
		issued := anchor.AddDate(0, 0, -period*cadenceDays)
		due := issued.AddDate(0, 0, paymentTermDays)
		invoice := accounting.Invoice{
			ExternalID: fmt.Sprintf("%s-INV-%04d", customerID, invoiceCount-period),
			Number:     fmt.Sprintf("INV-%s-%03d", issued.Format("2006"), invoiceCount-period),
			IssuedOn:   issued,
			DueOn:      &due,
			Currency:   currency,
		}
		// The ledger is all EUR, the literal above, so whole euros become cents
		// by a fixed hundred.
		net := int64(rng.IntN(amountSpread)+amountFloor) * 100
		invoice.NetMinor = net
		invoice.TaxMinor = net * vatPercent / 100 // money-scale-exempt: a percentage, not a unit scale
		invoice.GrossMinor = invoice.NetMinor + invoice.TaxMinor

		if period >= kind.openTail {
			settle(&invoice, kind, rng, due, anchor)
		}
		out.Invoices = append(out.Invoices, invoice)
		if invoice.FullyPaidAt != nil {
			out.Payments = append(out.Payments, accounting.Payment{
				ExternalID:        invoice.ExternalID + "-PMT",
				InvoiceExternalID: invoice.ExternalID,
				PaidAt:            *invoice.FullyPaidAt,
				Currency:          currency,
				AmountMinor:       invoice.GrossMinor,
			})
		}
	}
	out.Invoices = append(out.Invoices, creditNoteFor(out.Invoices, customerID, rng, anchor))
	return out, nil
}

// settle pays an invoice the way this customer pays, or disputes it. A dispute
// stays open and is left out of the timeliness reading. A payment that would
// land after the anchor has not happened yet, so that invoice stays open.
func settle(invoice *accounting.Invoice, kind archetype, rng *rand.Rand, due, anchor time.Time) {
	if rng.IntN(1000) < kind.disputeRate {
		invoice.Disputed = true
		return
	}
	paid := due.AddDate(0, 0, kind.lateLow+rng.IntN(kind.lateHigh-kind.lateLow+1))
	if paid.After(anchor) {
		return
	}
	invoice.FullyPaidAt = &paid
	invoice.PaidMinor = invoice.GrossMinor
}

// creditNoteFor issues a partial credit against an invoice old enough that the
// note still lands on or before the anchor. The oldest invoice always
// qualifies, so there is at least one target.
func creditNoteFor(invoices []accounting.Invoice, customerID string, rng *rand.Rand, anchor time.Time) accounting.Invoice {
	eligible := 0
	for _, inv := range invoices {
		if inv.IssuedOn.AddDate(0, 0, creditNoteDelayDays).After(anchor) {
			break
		}
		eligible++
	}
	target := invoices[rng.IntN(eligible)]
	amount := target.NetMinor / 4
	return accounting.Invoice{
		ExternalID:        customerID + "-CN-1",
		Number:            "CN-" + target.Number,
		IssuedOn:          target.IssuedOn.AddDate(0, 0, creditNoteDelayDays),
		Currency:          target.Currency,
		NetMinor:          amount,
		GrossMinor:        amount,
		CreditsExternalID: target.ExternalID,
	}
}

func seedFor(connectionID, customerID string) (uint64, uint64) {
	sum := sha256.Sum256([]byte(connectionID + "|" + customerID))
	return binary.BigEndian.Uint64(sum[0:8]), binary.BigEndian.Uint64(sum[8:16])
}
