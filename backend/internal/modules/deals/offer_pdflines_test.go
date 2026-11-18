// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"fmt"
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

// A long description wraps inside the margins instead of running off the page.
func TestRenderOfferPDF_ALongDescriptionWraps(t *testing.T) {
	description := "Opening " + strings.Repeat("implementation workshop ", 16) + "Closing"
	line := crmcontracts.OfferLineItem{Position: 1, Description: description, Quantity: 1, UnitPriceMinor: 100}
	pdf, err := RenderOfferPDF(testRenderOffer(100, 0, 100), []crmcontracts.OfferLineItem{line}, nil, issuedBy("Margince GmbH"), "en", nil)
	if err != nil {
		t.Fatal(err)
	}
	drawn := string(pdfDrawnText(t, pdf))
	opening, closing := strings.Index(drawn, "Opening"), strings.Index(drawn, "Closing")
	if opening < 0 || closing < opening || !strings.ContainsAny(drawn[opening:closing], "\r\n") {
		t.Fatalf("the description must print whole, over more than one line:\n%s", drawn)
	}
}

// A line's description and its price stay on one page, and every page says
// which page of how many it is.
func TestRenderOfferPDF_KeepsEachLineOnOnePageAndNumbersThePages(t *testing.T) {
	var lines []crmcontracts.OfferLineItem
	for position := 1; position <= 60; position++ {
		lines = append(lines, crmcontracts.OfferLineItem{
			Position: position, Description: fmt.Sprintf("Bulk line %02d", position), Quantity: 1, UnitPriceMinor: 100,
		})
	}
	pdf, err := RenderOfferPDF(testRenderOffer(6000, 0, 6000), lines, nil, issuedBy("Margince GmbH"), "de-DE", nil)
	if err != nil {
		t.Fatal(err)
	}
	drawn := string(pdfDrawnText(t, pdf))
	if !strings.Contains(drawn, "Seite 1 von 3") || !strings.Contains(drawn, "Seite 3 von 3") {
		t.Fatalf("the pages must be numbered 1 to 3 of 3:\n%s", drawn)
	}
	for position := 1; position <= 60; position++ {
		start := strings.Index(drawn, fmt.Sprintf("Bulk line %02d", position))
		price := strings.Index(drawn[start:], "1 x 1,00 EUR")
		if start < 0 || price < 0 || strings.Contains(drawn[start:start+price], "Seite ") {
			t.Fatalf("line %d and its price must share a page:\n%s", position, drawn)
		}
	}
}

// Amounts and quantities use the document's separators, and a currency with no
// minor unit prints none.
func TestRenderOfferPDF_FormatsNumbersForTheDocumentsLanguage(t *testing.T) {
	o := testRenderOffer(3938, 0, 3938)
	o.Currency = "JPY"
	line := crmcontracts.OfferLineItem{Position: 1, Description: "Licence", Quantity: 1234567.891, UnitPriceMinor: 3938}
	for locale, want := range map[string][]string{
		"de-DE": {"1.234.567,891 x 3.938 JPY", "Nettobetrag: 3.938 JPY"},
		"en":    {"1,234,567.891 x 3,938 JPY", "Net: 3,938 JPY"},
	} {
		pdf, err := RenderOfferPDF(o, []crmcontracts.OfferLineItem{line}, nil, issuedBy("Margince GmbH"), locale, nil)
		if err != nil {
			t.Fatal(err)
		}
		drawn := string(pdfDrawnText(t, pdf))
		for _, needle := range want {
			if !strings.Contains(drawn, needle) {
				t.Errorf("%s: the PDF must print %q:\n%s", locale, needle, drawn)
			}
		}
	}
}

func TestPDFLocalizeNumber(t *testing.T) {
	german := resolvePDFLabels("de-DE")
	for plain, want := range map[string]string{
		"0": "0", "999": "999", "1000": "1.000", "-1234567.891": "-1.234.567,891", "12.5": "12,5",
	} {
		if got := pdfLocalizeNumber(plain, german); got != want {
			t.Errorf("pdfLocalizeNumber(%q) = %q, want %q", plain, got, want)
		}
	}
}
