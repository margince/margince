// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The offer PDF renderer: a branded A4 document — title/buyer/line-items/
// totals, the offer's own intro and terms, and the selected template's
// header and footer — built purely from already-resolved, already-persisted
// inputs (offer_render.go's PrepareRender gathers them). The offer's
// Net/Tax/GrossMinor fields are the totals engine's already-persisted output
// (offer_totals.go, recomputed inside every mutating transaction); this file
// reads them off the Offer struct as-is and performs no money arithmetic of
// its own, so the rendered document can never disagree with what the API
// already shows.
//
// layout is a workspace-authored, loosely-typed jsonb bag (crm.yaml's
// OfferTemplate.layout). This renderer honors exactly the string keys in
// OfferTemplateLayoutTextKeys, printing each verbatim when present and
// omitting the section entirely when absent. It fetches no other key, a logo
// reference included. The logo it embeds is the issuing legal entity's, whose
// bytes the caller reads and hands in.

package deals

import (
	"bytes"
	"fmt"
	"strconv"

	"github.com/go-pdf/fpdf"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/mailcopy"
)

// The template layout keys the renderer prints, spelled as the offer-template
// form (frontend/src/features/deals/offertemplates.tsx) stores them;
// backend/gates/offertemplatelayout_test.go holds the two sides to one set.
const (
	offerLayoutHeader = "header"
	offerLayoutFooter = "footer"
)

// OfferTemplateLayoutTextKeys lists the layout keys RenderOfferPDF prints, for
// the gate that holds the template form's spelling to them.
func OfferTemplateLayoutTextKeys() []string {
	return []string{offerLayoutHeader, offerLayoutFooter}
}

// pdfLabels holds one locale's label set for the rendered PDF.
type pdfLabels struct {
	title     string
	issuer    string
	revision  string
	buyer     string
	lineItems string
	net       string
	tax       string
	gross     string
	totals    string
	terms     string
	// cadence holds one phrase per admitted billing interval, keyed by its
	// months. A cadence the schema does not admit prints nothing rather than a
	// guess at its name.
	cadence   map[int]string
	committed string
	periods   string
	months    string
	// The issuer block's own labels.
	vatID    string
	register string
	bank     string
	// A line's own figures, and the unit keys of unit.go by name.
	discount   string
	lineNet    string
	lineTotal  string
	units      map[string]string
	validUntil string
	// The page footer, with %d for the page and %s for the page count.
	page string
	// The document language's number separators and date layout.
	format mailcopy.Copy
}

// resolvePDFLabels maps a locale to its label set. A German locale gets German
// labels, and so does "", which languageOfLocale reads as German. Every other
// language gets English.
func resolvePDFLabels(locale string) pdfLabels {
	format := mailcopy.For(languageOfLocale(locale))
	if languageOfLocale(locale) == languageOfLocale(defaultOfferTemplateLocale) {
		return pdfLabels{
			title:     "Angebot",
			issuer:    "Aussteller",
			revision:  "Revision",
			buyer:     "Kunde",
			lineItems: "Positionen",
			net:       "Nettobetrag",
			tax:       "MwSt",
			gross:     "Gesamtbetrag",
			totals:    "Summe",
			terms:     "Bedingungen",
			cadence: map[int]string{
				1: "pro Monat", 3: "pro Quartal", 6: "pro Halbjahr", 12: "pro Jahr",
			},
			committed: "Vertragswert",
			periods:   "Laufzeit",
			months:    "Monate",
			vatID:     "USt-IdNr.",
			register:  "Handelsregister",
			bank:      "Bankverbindung",
			discount:  "Rabatt",
			lineNet:   "Netto",
			lineTotal: "Gesamt",
			units: map[string]string{
				unitPiece: "Stk.", unitHour: "Std.", unitDay: "Tag(e)", unitMonth: "Monat(e)", unitFlatFee: "pauschal",
			},
			validUntil: "Gültig bis",
			page:       "Seite %d von %s",
			format:     format,
		}
	}
	return pdfLabels{
		title:     "Offer",
		issuer:    "Issuer",
		revision:  "Revision",
		buyer:     "Buyer",
		lineItems: "Line items",
		net:       "Net",
		tax:       "Tax",
		gross:     "Total",
		totals:    "Totals",
		terms:     "Terms",
		cadence: map[int]string{
			1: "per month", 3: "per quarter", 6: "per half-year", 12: "per year",
		},
		committed: "Committed",
		periods:   "Term",
		months:    "months",
		vatID:     "VAT ID",
		register:  "Register",
		bank:      "Bank details",
		discount:  "Discount",
		lineNet:   "Net",
		lineTotal: "Total",
		units: map[string]string{
			unitPiece: "pc.", unitHour: "h", unitDay: "day(s)", unitMonth: "month(s)", unitFlatFee: "flat fee",
		},
		validUntil: "Valid until",
		page:       "Page %d of %s",
		format:     format,
	}
}

// pdfFormatMinor prints a stored minor-unit amount and never re-derives it.
// Money places the decimals by values.MajorUnits, so VND and KWD print at value.
func pdfFormatMinor(minor int64, currency string, labels pdfLabels) string {
	return labels.format.Money(minor, currency)
}

func pdfFormatQuantity(quantity float64, labels pdfLabels) string {
	return labels.format.Number(strconv.FormatFloat(quantity, 'f', -1, 64))
}

func pdfFormatPercent(pct float64, labels pdfLabels) string {
	return labels.format.Number(strconv.FormatFloat(pct, 'f', -1, 64)) + " %"
}

// pdfOfferText reads one of the offer's own optional texts; null and an
// emptied field both answer "", the caller's cue to draw nothing.
func pdfOfferText(text *string) string {
	if text == nil {
		return ""
	}
	return *text
}

func pdfBuyerBlockString(buyerBlock map[string]any, key string) string {
	v, _ := buyerBlock[key].(string)
	return v
}

// pdfLayoutString reads one bounded string key off a template's layout
// bag (see this file's doc comment for the honored keys); an absent key,
// or one holding a non-string value, answers "" — the caller's cue to
// omit that section entirely rather than print an empty heading.
func pdfLayoutString(layout map[string]any, key string) string {
	v, _ := layout[key].(string)
	return v
}

// pdfTranslator is applied to every string a writer draws, labels included. It
// hands the string back unchanged and records what the font cannot print.
type pdfTranslator func(string) string

// writeOfferPDFHeader writes the title/revision/issuer block, the
// template layout's header (when the layout carries one), and the buyer
// legal block underneath it. A block that NAMES no buyer — nil, as an unsent
// draft with no buyer company is, or one carrying neither display_name nor
// legal_name — omits the section entirely rather than printing an empty
// heading.
func writeOfferPDFHeader(pdf *fpdf.Fpdf, tr pdfTranslator, o crmcontracts.Offer, buyerBlock map[string]any, issuer PDFIssuer, layout map[string]any, labels pdfLabels) {
	number := ""
	if o.OfferNumber != nil {
		number = *o.OfferNumber
	}
	revision := 0
	if o.Revision != nil {
		revision = *o.Revision
	}

	pdf.SetFont(offerFontFamily, "B", 18)
	pdf.Cell(0, 8, tr(labels.title+" "+number))
	pdf.Ln(10)
	pdf.SetFont(offerFontFamily, "", 11)
	pdf.Cell(0, 6, tr(labels.revision+" "+strconv.Itoa(revision)))
	pdf.Ln(7)
	if o.ValidUntil != nil {
		pdf.Cell(0, 6, tr(labels.validUntil+": "+labels.format.Date(o.ValidUntil.Time)))
		pdf.Ln(7)
	}
	pdf.Cell(0, 6, tr(labels.issuer+": "+issuer.Name))
	pdf.Ln(7)
	writeOfferPDFIssuerBlock(pdf, tr, issuer.OfferIssuer, labels)
	pdf.Ln(3)

	if headerText := pdfLayoutString(layout, offerLayoutHeader); headerText != "" {
		pdf.SetFont(offerFontFamily, "", 10)
		pdf.MultiCell(0, 5, tr(headerText), "", "L", false)
		pdf.Ln(4)
	}

	// The buyer is identified by NAME. The snapshot also carries our internal
	// company_id, and printing it put a UUID — under a hardcoded English
	// label, on an otherwise translated document — as the first line the
	// customer read about themselves. It identifies the record to us and
	// nothing to them.
	//
	// So a block holding neither name draws no section at all, rather than a
	// heading over blank paper: an offer that cannot say who it is for says
	// nothing there, and the reader sees a document missing its buyer instead
	// of one whose buyer is an empty line.
	//
	// Reachable through the FROZEN snapshot, which resolveRenderBuyerBlock
	// returns verbatim from jsonb (offer_render.go). Both of today's writers
	// set display_name beside the id, and company.display_name is NOT
	// NULL, so a block this release builds always names its buyer — but the
	// stored bag is not constrained to that shape, and this renderer's job is
	// to print what it was handed rather than to assume what wrote it.
	displayName := pdfBuyerBlockString(buyerBlock, "display_name")
	legalName := pdfBuyerBlockString(buyerBlock, "legal_name")
	if buyerBlock == nil || (displayName == "" && legalName == "") {
		return
	}
	pdf.SetFont(offerFontFamily, "B", 12)
	pdf.Cell(0, 6, tr(labels.buyer))
	pdf.Ln(7)
	pdf.SetFont(offerFontFamily, "", 11)
	if displayName != "" {
		pdf.MultiCell(0, 6, tr(displayName), "", "L", false)
	}
	if legalName != "" {
		pdf.MultiCell(0, 6, tr(legalName), "", "L", false)
	}
	pdf.Ln(4)
}

// writeOfferPDFIntro writes the offer's own intro text ahead of the lines it
// introduces, when the offer carries one.
func writeOfferPDFIntro(pdf *fpdf.Fpdf, tr pdfTranslator, o crmcontracts.Offer) {
	text := pdfOfferText(o.IntroText)
	if text == "" {
		return
	}
	pdf.SetFont(offerFontFamily, "", 10)
	pdf.MultiCell(0, 5, tr(text), "", "L", false)
	pdf.Ln(4)
}

// writeOfferPDFTotals writes the money summary straight off o's already-
// persisted Net/Tax/GrossMinor — the ONE totals figure this function (or
// any part of this renderer) ever shows; nothing here re-sums the lines.
func writeOfferPDFTotals(pdf *fpdf.Fpdf, tr pdfTranslator, o crmcontracts.Offer, labels pdfLabels) {
	var net, tax, gross int64
	if o.NetMinor != nil {
		net = *o.NetMinor
	}
	if o.TaxMinor != nil {
		tax = *o.TaxMinor
	}
	if o.GrossMinor != nil {
		gross = *o.GrossMinor
	}
	pdf.SetFont(offerFontFamily, "B", 12)
	pdf.Cell(0, 6, tr(labels.totals))
	pdf.Ln(7)
	pdf.SetFont(offerFontFamily, "", 11)
	pdf.Cell(0, 6, tr(labels.net+": "+pdfFormatMinor(net, o.Currency, labels)))
	pdf.Ln(6)
	pdf.Cell(0, 6, tr(labels.tax+": "+pdfFormatMinor(tax, o.Currency, labels)))
	pdf.Ln(6)
	pdf.Cell(0, 6, tr(labels.gross+": "+pdfFormatMinor(gross, o.Currency, labels)))
	pdf.Ln(6)
}

// writeOfferPDFTerms writes the offer's own terms text as a dedicated
// section, when it carries one — omitted entirely otherwise, never an empty
// heading. Terms come from the offer and never from its template: they are
// edited per offer under Edit header, the template form has no terms field,
// and a template default beneath them would be a second answer nobody can
// author.
func writeOfferPDFTerms(pdf *fpdf.Fpdf, tr pdfTranslator, o crmcontracts.Offer, labels pdfLabels) {
	text := pdfOfferText(o.TermsText)
	if text == "" {
		return
	}
	pdf.Ln(2)
	pdf.SetFont(offerFontFamily, "B", 12)
	pdf.Cell(0, 6, tr(labels.terms))
	pdf.Ln(7)
	pdf.SetFont(offerFontFamily, "", 10)
	pdf.MultiCell(0, 5, tr(text), "", "L", false)
}

// writeOfferPDFFooter writes the template layout's footer at the
// bottom of the rendered content, when the layout carries one.
func writeOfferPDFFooter(pdf *fpdf.Fpdf, tr pdfTranslator, layout map[string]any) {
	text := pdfLayoutString(layout, offerLayoutFooter)
	if text == "" {
		return
	}
	pdf.Ln(6)
	pdf.SetFont(offerFontFamily, "", 9)
	pdf.MultiCell(0, 5, tr(text), "", "L", false)
}

// RenderOfferPDF builds the branded offer PDF from already-resolved inputs
// (see offer_render.go's PrepareRender).
//   - o carries the offer header, intro and terms, and its server-computed totals.
//   - lines are its accepted line items.
//   - buyerBlock is the frozen buyer_snapshot once sent, the live buyer company
//     while still draft, and nil when the offer has none.
//   - issuer is the issuing legal entity's block and logo.
//   - locale picks the DE or EN labels.
//   - layout is the selected template's layout bag, nil or empty without one.
//     This file's doc comment lists the layout keys it honors.
//
// It performs no database access and no money arithmetic. Every figure it
// prints was computed and persisted before it was called, so the rendered
// document can never disagree with the server-computed totals.
func RenderOfferPDF(o crmcontracts.Offer, lines []crmcontracts.OfferLineItem, buyerBlock map[string]any, issuer PDFIssuer, locale string, layout map[string]any) ([]byte, error) {
	labels := resolvePDFLabels(locale)
	number := ""
	if o.OfferNumber != nil {
		number = *o.OfferNumber
	}

	check, err := newPDFTextCheck()
	if err != nil {
		return nil, err
	}
	tr := check.text

	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetCompression(false)
	pdf.SetMargins(16, 16, 16)
	pdf.SetAutoPageBreak(true, 16)
	registerOfferFont(pdf)
	pdf.AliasNbPages("")
	pdf.SetFooterFunc(func() {
		pdf.SetY(-12)
		pdf.SetFont(offerFontFamily, "", 8)
		pdf.CellFormat(0, 5, tr(fmt.Sprintf(labels.page, pdf.PageNo(), "{nb}")), "", 0, "C", false, 0, "")
	})
	pdf.SetTitle(labels.title+" "+number, true)
	pdf.SetCreator("margince", true)
	pdf.SetAuthor(issuer.Name, true)
	pdf.SetSubject(labels.title+" PDF", true)
	pdf.AddPage()

	writeOfferPDFLogo(pdf, issuer.Logo)
	writeOfferPDFHeader(pdf, tr, o, buyerBlock, issuer, layout, labels)
	writeOfferPDFIntro(pdf, tr, o)
	if err := writeOfferPDFLineItems(pdf, tr, lines, o.Currency, labels); err != nil {
		return nil, err
	}
	writeOfferPDFTotals(pdf, tr, o, labels)
	writeOfferPDFTerms(pdf, tr, o, labels)
	writeOfferPDFFooter(pdf, tr, layout)
	if err := check.err(); err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, fmt.Errorf("render offer pdf: %w", err)
	}
	return buf.Bytes(), nil
}
