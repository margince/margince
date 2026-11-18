// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"strings"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func discountedTaxedLine() crmcontracts.OfferLineItem {
	net, tax, total := int64(8749), int64(1662), int64(10411)
	return crmcontracts.OfferLineItem{
		Position: 1, Description: "Consulting", Quantity: 3, Unit: unitHour, UnitPriceMinor: 3333,
		DiscountPct: 12.5, TaxRate: 19, LineNetMinor: &net, LineTaxMinor: &tax, LineTotalMinor: &total,
	}
}

// A discounted, taxed line prints its unit, discount, tax rate, net, tax and
// total. So the lines on the page add up to the offer's net.
func TestRenderOfferPDF_ADiscountedTaxedLinePrintsItsOwnFigures(t *testing.T) {
	o := testRenderOffer(8749, 1662, 10411)
	pdf, err := RenderOfferPDF(o, []crmcontracts.OfferLineItem{discountedTaxedLine()}, nil, issuedBy("Margince GmbH"), "en", nil)
	if err != nil {
		t.Fatal(err)
	}
	drawn := string(pdfDrawnText(t, pdf))
	for _, want := range []string{
		"3 h x 33.33 EUR", "Discount 12.5 %", "Net 87.49 EUR", "Tax 19 % 16.62 EUR", "Total 104.11 EUR",
	} {
		if !strings.Contains(drawn, want) {
			t.Errorf("the line must print %q:\n%s", want, drawn)
		}
	}
}

// A line whose price could not be grounded prints no amount: its zero price is
// a sentinel, not a price to offer.
func TestRenderOfferPDF_AnUngroundedLinePrintsNoAmount(t *testing.T) {
	grounded := false
	line := crmcontracts.OfferLineItem{Position: 1, Description: "Proposed by an agent", Quantity: 2, Unit: unitDay, PriceGrounded: &grounded}
	pdf, err := RenderOfferPDF(testRenderOffer(0, 0, 0), []crmcontracts.OfferLineItem{line}, nil, issuedBy("Margince GmbH"), "de-DE", nil)
	if err != nil {
		t.Fatal(err)
	}
	drawn := string(pdfDrawnText(t, pdf))
	if !strings.Contains(drawn, "2 Tag(e)") || strings.Contains(drawn, "2 Tag(e) x") {
		t.Fatalf("an ungrounded line prints its quantity and no price:\n%s", drawn)
	}
}

// The offer's valid-until day is on the page, in the document's language.
func TestRenderOfferPDF_PrintsValidUntil(t *testing.T) {
	o := testRenderOffer(100000, 19000, 119000)
	o.ValidUntil = &openapi_types.Date{Time: time.Date(2026, time.December, 31, 0, 0, 0, 0, time.UTC)}
	for locale, want := range map[string]string{"de-DE": "Gültig bis: 31.12.2026", "en": "Valid until: 31 December 2026"} {
		pdf, err := RenderOfferPDF(o, testRenderLines(), nil, issuedBy("Margince GmbH"), locale, nil)
		if err != nil {
			t.Fatal(err)
		}
		if drawn := string(pdfDrawnText(t, pdf)); !strings.Contains(drawn, want) {
			t.Errorf("%s: the PDF must print %q:\n%s", locale, want, drawn)
		}
	}
}
