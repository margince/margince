// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// The offer PDF's line items. Every figure a line prints is its persisted
// snapshot or server-computed total, so the lines add up to the offer's totals.

import (
	"fmt"
	"strings"

	"github.com/go-pdf/fpdf"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// writeOfferPDFLineItems writes the line-items section: each line's
// description, then its quantity, unit, unit price and discount, then its net,
// tax and total.
func writeOfferPDFLineItems(pdf *fpdf.Fpdf, tr pdfTranslator, lines []crmcontracts.OfferLineItem, currency string, labels pdfLabels) {
	pdf.SetFont(offerFontFamily, "B", 12)
	pdf.Cell(0, 6, tr(labels.lineItems))
	pdf.Ln(7)
	pdf.SetFont(offerFontFamily, "", 10)
	for _, li := range lines {
		texts := append([]string{fmt.Sprintf("%d. %s", li.Position, li.Description)}, pdfLineTexts(li, currency, labels)...)
		for i := range texts {
			texts[i] = tr(texts[i])
		}
		keepOnOnePage(pdf, texts, pdfLineHeight)
		for _, text := range texts {
			pdf.MultiCell(0, pdfLineHeight, text, "", "L", false)
		}
	}
	pdf.Ln(2)
}

const pdfLineHeight = 5

// keepOnOnePage starts a new page when the wrapped texts would not fit on this
// one. So a line's description and its price never split across pages. A block
// taller than a page is left to break where it must.
func keepOnOnePage(pdf *fpdf.Fpdf, texts []string, lineHeight float64) {
	width, pageHeight := pdf.GetPageSize()
	left, top, right, _ := pdf.GetMargins()
	_, bottom := pdf.GetAutoPageBreak()
	rows := 0
	for _, text := range texts {
		rows += len(pdf.SplitText(text, width-left-right))
	}
	height := float64(rows) * lineHeight
	if pdf.GetY()+height > pageHeight-bottom && height <= pageHeight-top-bottom {
		pdf.AddPage()
	}
}

// pdfLineTexts are the lines printed under a line's description. A line whose
// price could not be grounded prints its quantity alone: its zero price is a
// sentinel, never an amount to show a buyer.
func pdfLineTexts(li crmcontracts.OfferLineItem, currency string, labels pdfLabels) []string {
	quantity := strings.TrimSpace(pdfFormatQuantity(li.Quantity, labels) + " " + pdfUnitName(li.Unit, labels))
	if li.PriceGrounded != nil && !*li.PriceGrounded {
		return []string{quantity}
	}
	price := quantity + " x " + pdfFormatMinor(li.UnitPriceMinor, currency, labels)
	if li.DiscountPct != 0 {
		price += " | " + labels.discount + " " + pdfFormatPercent(li.DiscountPct, labels)
	}
	if cadence := pdfLineCadence(li, labels); cadence != "" {
		price += " " + cadence
	}
	texts := []string{price}
	if li.LineNetMinor != nil && li.LineTaxMinor != nil && li.LineTotalMinor != nil {
		texts = append(texts, fmt.Sprintf("%s %s | %s %s %s | %s %s",
			labels.lineNet, pdfFormatMinor(*li.LineNetMinor, currency, labels),
			labels.tax, pdfFormatPercent(li.TaxRate, labels), pdfFormatMinor(*li.LineTaxMinor, currency, labels),
			labels.lineTotal, pdfFormatMinor(*li.LineTotalMinor, currency, labels)))
	}
	if commitment := pdfLineCommitment(li, currency, labels); commitment != "" {
		texts = append(texts, commitment)
	}
	return texts
}

// pdfUnitName is a unit key in the document's language; a unit typed as free
// text prints as typed.
func pdfUnitName(unit string, labels pdfLabels) string {
	if name, ok := labels.units[unit]; ok {
		return name
	}
	return unit
}

// pdfLineCadence is how often a recurring line's price repeats, "" for a line
// that does not repeat or has not been classified.
//
// A nil billing model is not one_time. Nobody has said what the price does, so
// printing a cadence would answer a question the offer leaves open.
//
// An interval outside the four the schema admits prints nothing, for that
// reason.
func pdfLineCadence(li crmcontracts.OfferLineItem, labels pdfLabels) string {
	if li.BillingModel == nil || *li.BillingModel != crmcontracts.OfferLineItemBillingModelRecurring {
		return ""
	}
	if li.BillingIntervalMonths == nil {
		return ""
	}
	return labels.cadence[int(*li.BillingIntervalMonths)]
}

// pdfLineCommitment is the term a recurring line is committed for and what it
// is worth across that term, "" where either is unsettled.
//
// A recurring line with no interval_count has no committed total yet, the same
// answer NetTcvMinor gives it: nothing is committed until somebody says how
// long.
//
// The multiplication is CommittedNetMinor's, which is where the offer's own
// total gets it, so this figure comes from one spelling of that arithmetic.
//
// A figure that cannot be computed prints the term alone. An offer handed to a
// buyer with one line missing its total reads worse than one showing the term
// by itself.
func pdfLineCommitment(li crmcontracts.OfferLineItem, currency string, labels pdfLabels) string {
	if pdfLineCadence(li, labels) == "" || li.IntervalCount == nil || li.LineNetMinor == nil {
		return ""
	}
	term := fmt.Sprintf("%s: %d x %d %s", labels.periods, *li.IntervalCount,
		int(*li.BillingIntervalMonths), labels.months)
	committed, err := CommittedNetMinor(*li.LineNetMinor, *li.IntervalCount)
	if err != nil {
		return term
	}
	return term + " | " + labels.committed + ": " + pdfFormatMinor(committed, currency, labels)
}
