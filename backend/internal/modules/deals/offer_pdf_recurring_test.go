// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// A recurring line's PDF says what the price repeats at and what the buyer
// committed to. Without it the document hands a buyer a per-period figure with
// no cadence beside it, which reads as the whole price.

import (
	"bytes"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// monthlyCommittedLine is one monthly line committed for twelve periods: 1,000.00 a
// month, 12,000.00 across the term.
func monthlyCommittedLine() crmcontracts.OfferLineItem {
	monthly := crmcontracts.OfferLineItemBillingIntervalMonthsN1
	model := crmcontracts.OfferLineItemBillingModelRecurring
	periods := 12
	net := int64(100000)
	return crmcontracts.OfferLineItem{
		Position: 1, Description: "Platform subscription", Quantity: 1, UnitPriceMinor: 100000,
		BillingModel: &model, BillingIntervalMonths: &monthly, IntervalCount: &periods,
		LineNetMinor: &net,
	}
}

func TestRenderOfferPDF_ARecurringLineShowsItsCadenceAndCommitment(t *testing.T) {
	o := testRenderOffer(1200000, 0, 1200000)
	pdf, err := RenderOfferPDF(o, []crmcontracts.OfferLineItem{monthlyCommittedLine()}, nil, "Margince GmbH", "en-GB", nil)
	if err != nil {
		t.Fatalf("RenderOfferPDF() error = %v", err)
	}
	drawn := pdfDrawnText(t, pdf)
	for _, needle := range []string{
		"1 x 1000.00 EUR per month", // the cadence beside the price
		"Term: 12 x 1 months",       // the periods committed to
		"Committed: 12000.00 EUR",   // net across the term
	} {
		if !bytes.Contains(drawn, []byte(needle)) {
			t.Errorf("PDF missing %q", needle)
		}
	}
}

// A sent offer's PDF records what the counterparty was shown. So a line
// carrying none of the billing fields renders as it always did.
//
// That is every line of every offer sent before those fields existed, and its
// bytes do not move.
func TestRenderOfferPDF_AnUnclassifiedLineRendersUnchanged(t *testing.T) {
	o := testRenderOffer(123456, 23456, 146912)
	drawn := func(t *testing.T, lines []crmcontracts.OfferLineItem) []byte {
		t.Helper()
		pdf, err := RenderOfferPDF(o, lines, nil, "Margince GmbH", "de-DE", nil)
		if err != nil {
			t.Fatalf("RenderOfferPDF() error = %v", err)
		}
		return pdfDrawnText(t, pdf)
	}
	plain := testRenderLines()
	before := drawn(t, plain)

	// The same lines with a ONE-TIME classification, which is what a line
	// says when somebody has looked at it and it does not repeat.
	oneTime := crmcontracts.OfferLineItemBillingModelOneTime
	classified := testRenderLines()
	classified[0].BillingModel = &oneTime
	if got := drawn(t, classified); !bytes.Equal(got, before) {
		t.Error("a one-time line changed the drawn document; only a repeating price adds a cadence")
	}
}

// A recurring line nobody has given a term yet. NetTcvMinor counts nothing for
// it, and the PDF says nothing about a commitment for the same reason: nothing
// is committed until somebody says how long.
func TestRenderOfferPDF_ARecurringLineWithNoTermShowsOnlyItsCadence(t *testing.T) {
	line := monthlyCommittedLine()
	line.IntervalCount = nil
	o := testRenderOffer(100000, 0, 100000)
	pdf, err := RenderOfferPDF(o, []crmcontracts.OfferLineItem{line}, nil, "Margince GmbH", "en-GB", nil)
	if err != nil {
		t.Fatalf("RenderOfferPDF() error = %v", err)
	}
	drawn := pdfDrawnText(t, pdf)
	if !bytes.Contains(drawn, []byte("per month")) {
		t.Error("the cadence is missing, and it is settled whatever the term is")
	}
	for _, absent := range []string{"Term:", "Committed:"} {
		if bytes.Contains(drawn, []byte(absent)) {
			t.Errorf("PDF carries %q for a line with no settled term", absent)
		}
	}
}
