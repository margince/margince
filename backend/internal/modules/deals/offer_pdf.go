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
// omitting the section entirely when absent; every other key (including a
// logo reference) is a decorative ref this offline, network-free renderer
// does not fetch or embed.

package deals

import (
	"bytes"
	"fmt"
	"strconv"

	"github.com/go-pdf/fpdf"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// The template layout keys the renderer prints, spelled as the offer-template
// form (frontend/src/screens/offertemplates.tsx) stores them;
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
}

// resolvePDFLabels maps a locale to its label set: de-DE (and the empty
// string, matching offer_template's own defaultOfferTemplateLocale) get
// German labels; everything else gets English — the DE/EN launch pair
// (WP7/OP-T02), not a general i18n catalog.
func resolvePDFLabels(locale string) pdfLabels {
	if locale == defaultOfferTemplateLocale || locale == "" {
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
	}
}

// pdfFormatMinor renders an integer minor-unit amount as a decimal
// display string with its currency code. Display-only: the money engine
// itself (offer_totals.go) never touches a float, and this function never
// re-derives the amount it formats.
//
// The decimal placement is values.MajorUnits' and not this file's, because the
// PDF is the document a buyer receives and signs. A hard-coded /100 states an
// offer priced in VND, JPY or KRW at a hundredth of its value — those
// currencies have no minor unit, so their integer IS the amount — and states
// a KWD offer at a hundred times, since a dinar has three.
func pdfFormatMinor(minor int64, currency string) string {
	return values.MajorUnits(minor, currency) + " " + currency
}

func pdfFormatQuantity(quantity float64) string {
	return strconv.FormatFloat(quantity, 'f', -1, 64)
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

// pdfTranslator converts a UTF-8 Go string to the byte sequence the core
// Helvetica font's built-in cp1252 encoding expects, so Cell/MultiCell
// render diacritics correctly instead of the raw UTF-8 bytes' mojibake
// (e.g. "ö" as two separate cp1252 code points, "Ã¶"). Every function in
// this file threads it through and applies it to every string it hands
// to Cell — labels included, since translating plain ASCII is a no-op.
type pdfTranslator func(string) string

// writeOfferPDFHeader writes the title/revision/issuer block, the
// template layout's header (when the layout carries one), and the buyer
// legal block underneath it. A block that NAMES no buyer — nil, as an unsent
// draft with no buyer company is, or one carrying neither display_name nor
// legal_name — omits the section entirely rather than printing an empty
// heading.
func writeOfferPDFHeader(pdf *fpdf.Fpdf, tr pdfTranslator, o crmcontracts.Offer, buyerBlock map[string]any, issuerName string, layout map[string]any, labels pdfLabels) {
	number := ""
	if o.OfferNumber != nil {
		number = *o.OfferNumber
	}
	revision := 0
	if o.Revision != nil {
		revision = *o.Revision
	}

	pdf.SetFont("Helvetica", "B", 18)
	pdf.Cell(0, 8, tr(labels.title+" "+number))
	pdf.Ln(10)
	pdf.SetFont("Helvetica", "", 11)
	pdf.Cell(0, 6, tr(labels.revision+" "+strconv.Itoa(revision)))
	pdf.Ln(7)
	pdf.Cell(0, 6, tr(labels.issuer+": "+issuerName))
	pdf.Ln(10)

	if headerText := pdfLayoutString(layout, offerLayoutHeader); headerText != "" {
		pdf.SetFont("Helvetica", "", 10)
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
	pdf.SetFont("Helvetica", "B", 12)
	pdf.Cell(0, 6, tr(labels.buyer))
	pdf.Ln(7)
	pdf.SetFont("Helvetica", "", 11)
	if displayName != "" {
		pdf.Cell(0, 6, tr(displayName))
		pdf.Ln(6)
	}
	if legalName != "" {
		pdf.Cell(0, 6, tr(legalName))
		pdf.Ln(6)
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
	pdf.SetFont("Helvetica", "", 10)
	pdf.MultiCell(0, 5, tr(text), "", "L", false)
	pdf.Ln(4)
}

// writeOfferPDFLineItems writes the line-items section. Each line's
// displayed unit price is its own persisted snapshot (offer_lines.go);
// this function shows it verbatim, deriving nothing.
func writeOfferPDFLineItems(pdf *fpdf.Fpdf, tr pdfTranslator, lines []crmcontracts.OfferLineItem, currency string, labels pdfLabels) {
	pdf.SetFont("Helvetica", "B", 12)
	pdf.Cell(0, 6, tr(labels.lineItems))
	pdf.Ln(7)
	pdf.SetFont("Helvetica", "", 10)
	for _, li := range lines {
		pdf.Cell(0, 5, tr(fmt.Sprintf("%d. %s", li.Position, li.Description)))
		pdf.Ln(5)
		pdf.Cell(0, 5, tr(fmt.Sprintf("%s x %s", pdfFormatQuantity(li.Quantity), pdfFormatMinor(li.UnitPriceMinor, currency))))
		pdf.Ln(5)
	}
	pdf.Ln(2)
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
	pdf.SetFont("Helvetica", "B", 12)
	pdf.Cell(0, 6, tr(labels.totals))
	pdf.Ln(7)
	pdf.SetFont("Helvetica", "", 11)
	pdf.Cell(0, 6, tr(labels.net+": "+pdfFormatMinor(net, o.Currency)))
	pdf.Ln(6)
	pdf.Cell(0, 6, tr(labels.tax+": "+pdfFormatMinor(tax, o.Currency)))
	pdf.Ln(6)
	pdf.Cell(0, 6, tr(labels.gross+": "+pdfFormatMinor(gross, o.Currency)))
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
	pdf.SetFont("Helvetica", "B", 12)
	pdf.Cell(0, 6, tr(labels.terms))
	pdf.Ln(7)
	pdf.SetFont("Helvetica", "", 10)
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
	pdf.SetFont("Helvetica", "I", 9)
	pdf.MultiCell(0, 5, tr(text), "", "L", false)
}

// RenderOfferPDF builds the branded offer PDF from already-resolved
// inputs (see offer_render.go's PrepareRender): o carries the offer
// header (intro and terms included) and its server-computed totals, lines are its accepted line
// items, buyerBlock is the frozen buyer_snapshot once sent or the live
// buyer company while still draft (nil when the offer has none),
// issuerName is the seller's display name, locale drives the DE/EN label
// set, and layout is the selected template's layout bag (nil or empty
// when the offer carries no template) — see this file's doc comment for
// the bounded set of layout keys honored. This function performs no
// database access and no money arithmetic — every figure it prints was
// computed and persisted before it was called, so the rendered document
// can never disagree with the server-computed totals.
func RenderOfferPDF(o crmcontracts.Offer, lines []crmcontracts.OfferLineItem, buyerBlock map[string]any, issuerName, locale string, layout map[string]any) ([]byte, error) {
	labels := resolvePDFLabels(locale)
	number := ""
	if o.OfferNumber != nil {
		number = *o.OfferNumber
	}

	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetCompression(false)
	pdf.SetMargins(16, 16, 16)
	pdf.SetAutoPageBreak(true, 16)
	// tr maps a UTF-8 string onto core Helvetica's built-in cp1252
	// encoding — the launch DE/EN locale pair's Latin diacritics (ö, ü,
	// ß, é, …) all round-trip through cp1252; a non-Latin locale (e.g.
	// Cyrillic, CJK) would need an embedded TTF font instead, not
	// attempted here since only DE/EN ship at launch.
	tr := pdf.UnicodeTranslatorFromDescriptor("")
	// The isUTF8=true argument (unlike Cell's cp1252 byte stream) tells
	// fpdf to UTF-16BE-encode the metadata itself, so the document
	// properties survive a non-ASCII issuer/title too.
	pdf.SetTitle(labels.title+" "+number, true)
	pdf.SetCreator("margince", true)
	pdf.SetAuthor(issuerName, true)
	pdf.SetSubject(labels.title+" PDF", true)
	pdf.AddPage()

	writeOfferPDFHeader(pdf, tr, o, buyerBlock, issuerName, layout, labels)
	writeOfferPDFIntro(pdf, tr, o)
	writeOfferPDFLineItems(pdf, tr, lines, o.Currency, labels)
	writeOfferPDFTotals(pdf, tr, o, labels)
	writeOfferPDFTerms(pdf, tr, o, labels)
	writeOfferPDFFooter(pdf, tr, layout)

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, fmt.Errorf("render offer pdf: %w", err)
	}
	return buf.Bytes(), nil
}
