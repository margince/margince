// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"regexp"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/platform/pdftext"
)

// testRenderOffer builds a minimal but complete offer for the PDF unit
// tests: two lines, an explicit currency, and the money triple already
// "persisted" as the totals engine would leave it.
func testRenderOffer(net, tax, gross int64) crmcontracts.Offer {
	number := "A-1042"
	revision := 2
	return crmcontracts.Offer{
		OfferNumber: &number,
		Revision:    &revision,
		Currency:    "EUR",
		NetMinor:    &net,
		TaxMinor:    &tax,
		GrossMinor:  &gross,
	}
}

func testRenderLines() []crmcontracts.OfferLineItem {
	return []crmcontracts.OfferLineItem{
		{Position: 1, Description: "Consulting Day", Quantity: 2, UnitPriceMinor: 50000},
		{Position: 2, Description: "Setup Fee", Quantity: 1, UnitPriceMinor: 23456},
	}
}

// pdfReader reads the offer PDFs back through the engine the product reads
// uploaded PDFs with. One per package: each compiles a 5 MB WebAssembly module.
var pdfReader = pdftext.New()

// pdfDrawnText returns the text a reader of the document sees, and never the
// container around it. The /CreationDate stamp is not on the page, so no
// assertion here reads the wall clock.
func pdfDrawnText(t *testing.T, pdf []byte) []byte {
	t.Helper()
	text, err := pdfReader.Read(t.Context(), pdf, 1_000_000)
	if err != nil {
		t.Fatalf("read the offer PDF's text: %v", err)
	}
	return []byte(text)
}

func TestRenderOfferPDF_IncludesOfferDataAndStoredTotals(t *testing.T) {
	o := testRenderOffer(123456, 23456, 146912)
	buyerBlock := map[string]any{"company_id": "company-1", "display_name": "Acme GmbH"}

	pdf, err := RenderOfferPDF(o, testRenderLines(), buyerBlock, issuedBy("Margince GmbH"), "de-DE", nil)
	if err != nil {
		t.Fatalf("RenderOfferPDF() error = %v", err)
	}
	if len(pdf) == 0 {
		t.Fatal("RenderOfferPDF() returned an empty PDF")
	}
	drawn := pdfDrawnText(t, pdf)

	mustContain := []string{
		"A-1042", "Revision 2", "Acme GmbH", "Margince GmbH",
		"Consulting Day", "2 x 500,00 EUR", "Setup Fee",
		"1.234,56 EUR", "234,56 EUR", "1.469,12 EUR",
	}
	for _, needle := range mustContain {
		if !bytes.Contains(drawn, []byte(needle)) {
			t.Fatalf("PDF missing %q", needle)
		}
	}
}

func TestRenderOfferPDF_LocaleDrivesLabels(t *testing.T) {
	o := testRenderOffer(100000, 19000, 119000)
	lines := testRenderLines()

	dePDF, err := RenderOfferPDF(o, lines, nil, issuedBy("Margince GmbH"), "de-DE", nil)
	if err != nil {
		t.Fatalf("RenderOfferPDF(de-DE) error = %v", err)
	}
	enPDF, err := RenderOfferPDF(o, lines, nil, issuedBy("Margince GmbH"), "en", nil)
	if err != nil {
		t.Fatalf("RenderOfferPDF(en) error = %v", err)
	}

	deDrawn, enDrawn := pdfDrawnText(t, dePDF), pdfDrawnText(t, enPDF)

	if !bytes.Contains(deDrawn, []byte("Angebot")) || !bytes.Contains(deDrawn, []byte("Nettobetrag")) {
		t.Fatalf("de-DE PDF missing German labels:\n%s", deDrawn)
	}
	if bytes.Contains(deDrawn, []byte("Offer ")) {
		t.Fatalf("de-DE PDF unexpectedly contains the English title label:\n%s", deDrawn)
	}

	if !bytes.Contains(enDrawn, []byte("Offer ")) || !bytes.Contains(enDrawn, []byte("Net: ")) {
		t.Fatalf("en PDF missing English labels:\n%s", enDrawn)
	}
	if bytes.Contains(enDrawn, []byte("Angebot")) {
		t.Fatalf("en PDF unexpectedly contains a German label:\n%s", enDrawn)
	}
}

// TestRenderOfferPDF_UsesStoredTotalsNeverRecomputes is the OFFER-AC-12a
// no-drift proof: the offer's stored NetMinor/TaxMinor/GrossMinor are
// deliberately set to a figure that does NOT match what re-summing the
// two lines below would produce (2×500.00 + 234.56 = 1234.56 net, not
// the 999999 minor units set here). The rendered PDF must show the
// STORED figure verbatim — proving RenderOfferPDF never re-derives
// totals from the lines, only reads what the caller already computed
// and persisted.
func TestRenderOfferPDF_UsesStoredTotalsNeverRecomputes(t *testing.T) {
	mismatchedNet := int64(999999)
	o := testRenderOffer(mismatchedNet, 1, 1000000)

	pdf, err := RenderOfferPDF(o, testRenderLines(), nil, issuedBy("Margince GmbH"), "de-DE", nil)
	if err != nil {
		t.Fatalf("RenderOfferPDF() error = %v", err)
	}
	drawn := pdfDrawnText(t, pdf)
	if !bytes.Contains(drawn, []byte("9.999,99 EUR")) {
		t.Fatalf("PDF must render the offer's STORED net total (9.999,99 EUR), got:\n%s", drawn)
	}
	if bytes.Contains(drawn, []byte("1.234,56 EUR")) {
		t.Fatalf("PDF must NOT contain a freshly re-derived total from the lines (1.234,56 EUR); it must use the stored figure only:\n%s", drawn)
	}
}

// Names and descriptions print as typed: German, Vietnamese and Central
// European letters come back from the page, never a substitute.
func TestRenderOfferPDF_PrintsLatinCyrillicAndGreekTextAsTyped(t *testing.T) {
	o := testRenderOffer(100000, 19000, 119000)
	intro := "Kính gửi Quý khách, Đề nghị của chúng tôi"
	o.IntroText = &intro
	lines := []crmcontracts.OfferLineItem{
		{Position: 1, Description: "Dịch vụ tư vấn Łódź", Quantity: 1, UnitPriceMinor: 100000},
		{Position: 2, Description: "Prüfgebühr Größe Москва Αθήνα", Quantity: 1, UnitPriceMinor: 100000},
	}
	buyerBlock := map[string]any{"display_name": "Nguyễn Thị Minh Khai", "legal_name": "Công ty TNHH Một Thành Viên Štěpánek"}

	pdf, err := RenderOfferPDF(o, lines, buyerBlock, issuedBy("Straße Verträge GmbH"), "de-DE", nil)
	if err != nil {
		t.Fatalf("RenderOfferPDF() error = %v", err)
	}
	drawn := string(pdfDrawnText(t, pdf))
	for _, want := range []string{
		"Dịch vụ tư vấn Łódź", "Prüfgebühr Größe Москва Αθήνα", "Nguyễn Thị Minh Khai",
		"Công ty TNHH Một Thành Viên Štěpánek", "Straße Verträge GmbH", intro,
	} {
		if !strings.Contains(drawn, want) {
			t.Errorf("the PDF must print %q as typed:\n%s", want, drawn)
		}
	}
}

// A character the font cannot print refuses the render and is named. A
// right-to-left letter counts as one, since fpdf would lay it out reversed:
// the font draws Hebrew, and still "ש" is refused.
func TestRenderOfferPDF_RefusesTextItCannotPrint(t *testing.T) {
	o := testRenderOffer(100000, 19000, 119000)
	intro, terms := "Wrapped intro 😀", "Wrapped terms 🎉"
	o.IntroText, o.TermsText = &intro, &terms
	lines := []crmcontracts.OfferLineItem{
		{Position: 1, Description: "Tư vấn 李 محمد ש 🚀", Quantity: 1, UnitPriceMinor: 100000},
	}

	_, err := RenderOfferPDF(o, lines, nil, issuedBy("Margince GmbH"), "de-DE", nil)
	fault, ok := httperr.Classify(fmt.Errorf("render offer pdf: %w", err))
	if !ok || fault.Status != http.StatusUnprocessableEntity || fault.Code != "offer_pdf_unprintable_text" {
		t.Fatalf("an unprintable line must refuse with 422 offer_pdf_unprintable_text, got %+v (err %v)", fault, err)
	}
	for _, named := range []string{"李", "م", "ש", "🚀", "😀", "🎉"} {
		if !strings.Contains(fault.Detail, named) {
			t.Errorf("the refusal must name %q: %s", named, fault.Detail)
		}
	}
	if strings.Contains(fault.Detail, "ư") {
		t.Errorf("a printable Vietnamese letter must not be named: %s", fault.Detail)
	}
}

func TestRenderOfferPDF_OmitsBuyerSectionWhenBuyerBlockNil(t *testing.T) {
	o := testRenderOffer(100000, 19000, 119000)

	withBuyer, err := RenderOfferPDF(o, testRenderLines(), map[string]any{"display_name": "Acme GmbH"}, issuedBy("Margince GmbH"), "de-DE", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(pdfDrawnText(t, withBuyer), []byte("Kunde")) {
		t.Fatalf("a non-nil buyer block must render the buyer section heading:\n%s", withBuyer)
	}

	withoutBuyer, err := RenderOfferPDF(o, testRenderLines(), nil, issuedBy("Margince GmbH"), "de-DE", nil)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(pdfDrawnText(t, withoutBuyer), []byte("Kunde")) {
		t.Fatalf("a nil buyer block must omit the buyer section entirely:\n%s", withoutBuyer)
	}

	// A block carrying ONLY our internal id names no buyer, so it draws no
	// section either. The alternative it replaced was a heading over a bare
	// UUID: the id identifies the record to us and nothing to the customer
	// holding the page.
	idOnly, err := RenderOfferPDF(o, testRenderLines(),
		map[string]any{"company_id": "company-1"}, issuedBy("Margince GmbH"), "de-DE", nil)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(pdfDrawnText(t, idOnly), []byte("Kunde")) {
		t.Fatalf("a buyer block naming nobody must omit the section rather than "+
			"heading a blank one:\n%s", idOnly)
	}
}

// The internal company id never reaches the page.
//
// It used to be the FIRST line of the buyer block, under a hardcoded English
// "Company ID: " label on a document translated into the buyer's own
// language. Nothing failed when it was there, so nothing would fail if it came
// back; this is that test.
func TestRenderOfferPDF_NeverPrintsTheInternalCompanyID(t *testing.T) {
	o := testRenderOffer(100000, 19000, 119000)
	rendered, err := RenderOfferPDF(o, testRenderLines(),
		map[string]any{"company_id": "company-1", "display_name": "Acme GmbH"},
		issuedBy("Margince GmbH"), "de-DE", nil)
	if err != nil {
		t.Fatal(err)
	}
	drawn := pdfDrawnText(t, rendered)
	// The buyer IS named — without this the assertions below would pass over a
	// renderer that drew no buyer section at all.
	if !bytes.Contains(drawn, []byte("Acme GmbH")) {
		t.Fatalf("the buyer's name must be on the page:\n%s", drawn)
	}
	for _, forbidden := range []string{"company-1", "Company ID"} {
		if bytes.Contains(drawn, []byte(forbidden)) {
			t.Errorf("the offer PDF must not print %q — it identifies the record "+
				"to us and nothing to the customer:\n%s", forbidden, drawn)
		}
	}
}

// TestRenderOfferPDF_TwoTemplatesWithDistinctLayoutsProduceDifferentBytes
// is the layout-actually-renders proof: two templates whose layout bags
// differ only in their header/footer text must produce genuinely
// different PDF bytes — the regression this guards is a renderer that
// resolves a template (for its locale) but silently ignores the layout it
// carries, so every template would look identical regardless of its
// configured branding.
func TestRenderOfferPDF_TwoTemplatesWithDistinctLayoutsProduceDifferentBytes(t *testing.T) {
	o := testRenderOffer(100000, 19000, 119000)
	lines := testRenderLines()

	layoutA := map[string]any{"header": "Alpha Consulting GmbH", "footer": "Alpha footer"}
	layoutB := map[string]any{"header": "Beta Solutions GmbH", "footer": "Beta footer"}

	pdfA, err := RenderOfferPDF(o, lines, nil, issuedBy("Margince GmbH"), "de-DE", layoutA)
	if err != nil {
		t.Fatalf("RenderOfferPDF(layoutA) error = %v", err)
	}
	pdfB, err := RenderOfferPDF(o, lines, nil, issuedBy("Margince GmbH"), "de-DE", layoutB)
	if err != nil {
		t.Fatalf("RenderOfferPDF(layoutB) error = %v", err)
	}
	// Compare what the two documents DRAW, never the whole files: every
	// render stamps its own /CreationDate, so two files rendered a second
	// apart differ no matter what the layout did, and a renderer that
	// ignored the layout entirely would still satisfy a raw-bytes
	// inequality. Drawn text is the only comparison this proof can be made
	// of.
	drawnA, drawnB := pdfDrawnText(t, pdfA), pdfDrawnText(t, pdfB)
	if bytes.Equal(drawnA, drawnB) {
		t.Fatal("two templates with distinct layouts must draw different text — the layout is being ignored")
	}

	for _, want := range []string{"Alpha Consulting GmbH", "Alpha footer"} {
		if !bytes.Contains(drawnA, []byte(want)) {
			t.Fatalf("layoutA's PDF must contain %q:\n%s", want, drawnA)
		}
	}
	for _, unwanted := range []string{"Beta Solutions GmbH", "Beta footer"} {
		if bytes.Contains(drawnA, []byte(unwanted)) {
			t.Fatalf("layoutA's PDF must not contain layoutB's text %q", unwanted)
		}
	}
	for _, want := range []string{"Beta Solutions GmbH", "Beta footer"} {
		if !bytes.Contains(drawnB, []byte(want)) {
			t.Fatalf("layoutB's PDF must contain %q:\n%s", want, drawnB)
		}
	}
}

// TestRenderOfferPDF_EmptyLayoutOmitsHeaderFooterTermsSections proves the
// honest-gap side of the contract: a template with no header/footer/terms
// text (or no template at all — a nil layout) renders exactly the base
// document, with no empty "Terms" heading or stray blank lines standing
// in for the sections layout would otherwise add.
func TestRenderOfferPDF_EmptyLayoutOmitsHeaderFooterTermsSections(t *testing.T) {
	o := testRenderOffer(100000, 19000, 119000)

	pdf, err := RenderOfferPDF(o, testRenderLines(), nil, issuedBy("Margince GmbH"), "de-DE", nil)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(pdfDrawnText(t, pdf), []byte("Bedingungen")) {
		t.Fatalf("a nil layout must omit the terms heading entirely:\n%s", pdf)
	}
}

// TestRenderOfferPDF_LayoutIgnoresNonStringAndUnknownKeys proves this
// renderer only ever honors the bounded string keys it documents: an
// unknown key (a future/decorative ref like logo_url) and a non-string
// value under a known key are both silently ignored rather than panicking
// or leaking a Go-formatted value into the document.
func TestRenderOfferPDF_LayoutIgnoresNonStringAndUnknownKeys(t *testing.T) {
	o := testRenderOffer(100000, 19000, 119000)
	layout := map[string]any{
		"logo_url": "https://example.test/logo.png",
		"header":   12345, // wrong type — must be ignored, not stringified
	}

	pdf, err := RenderOfferPDF(o, testRenderLines(), nil, issuedBy("Margince GmbH"), "de-DE", layout)
	if err != nil {
		t.Fatal(err)
	}
	// The key and its value are not text, so the whole file is the honest
	// scope for them: an unhonored key must not reach the document by any
	// route, drawn or metadata.
	if bytes.Contains(pdf, []byte("logo_url")) || bytes.Contains(pdf, []byte("example.test")) {
		t.Fatalf("an unhonored layout key must never leak into the document:\n%s", pdf)
	}
	// The digits, however, are asserted against the drawn text alone.
	// "12345" is a substring of the /CreationDate a PDF rendered at
	// 12:34:5x carries, so the whole-file form of this assertion failed on
	// the clock rather than on a stringified value.
	if bytes.Contains(pdfDrawnText(t, pdf), []byte("12345")) {
		t.Fatalf("a non-string value under a known layout key must be ignored, not stringified:\n%s", pdf)
	}
}

// TestPDFDrawnTextExcludesTheWallClockStamp is the guard under the guard:
// it renders a real offer, rewrites the stamp to the one wall-clock second
// that broke this suite, and proves the two scopes disagree — the raw file
// carries "12345", the drawn text does not. Without it, rescoping the
// assertions above reads as a tidy-up that a later edit could quietly undo,
// and the next failure would again arrive as a ten-second-a-day mystery.
func TestPDFDrawnTextExcludesTheWallClockStamp(t *testing.T) {
	o := testRenderOffer(100000, 19000, 119000)

	pdf, err := RenderOfferPDF(o, testRenderLines(), nil, issuedBy("Margince GmbH"), "de-DE", nil)
	if err != nil {
		t.Fatal(err)
	}

	stamp := regexp.MustCompile(`/(Creation|Mod)Date \(D:\d{14}\)`)
	if !stamp.Match(pdf) {
		t.Fatalf("the renderer no longer stamps a wall-clock date; this guard is asserting on a shape that is gone:\n%s", pdf)
	}
	at123457 := stamp.ReplaceAll(pdf, []byte("/CreationDate (D:20260822123457)"))

	if !bytes.Contains(at123457, []byte("12345")) {
		t.Fatal("the spliced document must carry the colliding digits, or this guard proves nothing")
	}
	if bytes.Contains(pdfDrawnText(t, at123457), []byte("12345")) {
		t.Fatalf("drawn text must not carry the timestamp's digits — an assertion scoped to it would still be reading the clock:\n%s", pdfDrawnText(t, at123457))
	}
}

// TestPDFDrawnTextSurvivesDrawnTextThatSaysEndstream is the case a
// keyword scan cannot handle: an offer's terms text is arbitrary, so it
// may contain "endstream" itself. Reading each stream by its declared
// /Length keeps the whole document in view; scanning for the keyword would
// cut it at the drawn word and lose everything the renderer wrote after.
func TestPDFDrawnTextSurvivesDrawnTextThatSaysEndstream(t *testing.T) {
	o := testRenderOffer(100000, 19000, 119000)
	terms := "endstream is a word a customer may write"
	o.TermsText = &terms
	layout := map[string]any{"footer": "Footer after the terms"}

	pdf, err := RenderOfferPDF(o, testRenderLines(), nil, issuedBy("Margince GmbH"), "de-DE", layout)
	if err != nil {
		t.Fatal(err)
	}

	drawn := pdfDrawnText(t, pdf)
	if !bytes.Contains(drawn, []byte("endstream is a word a customer may write")) {
		t.Fatalf("the drawn terms text must survive intact:\n%s", drawn)
	}
	if !bytes.Contains(drawn, []byte("Footer after the terms")) {
		t.Fatalf("text drawn AFTER the word \"endstream\" must survive too — the stream was cut at the drawn word:\n%s", drawn)
	}
}

// TestRenderOfferPDF_PrintsTheOffersOwnIntroAndTerms holds what Edit header
// promises: the intro and terms typed on the offer are on the page the buyer
// reads, the intro ahead of the lines it introduces and the terms after the
// totals they qualify, between the template's header and footer.
func TestRenderOfferPDF_PrintsTheOffersOwnIntroAndTerms(t *testing.T) {
	o := testRenderOffer(100000, 19000, 119000)
	intro, terms := "Thank you for your enquiry", "Payable within 30 days"
	o.IntroText, o.TermsText = &intro, &terms
	layout := map[string]any{"header": "Letterhead line", "footer": "Registered in Berlin"}

	pdf, err := RenderOfferPDF(o, testRenderLines(), nil, issuedBy("Margince GmbH"), "en-US", layout)
	if err != nil {
		t.Fatal(err)
	}

	drawn := pdfDrawnText(t, pdf)
	order := []string{"Letterhead line", intro, "Consulting Day", "Totals", "Terms", terms, "Registered in Berlin"}
	at := -1
	for _, want := range order {
		next := bytes.Index(drawn, []byte(want))
		if next < 0 {
			t.Fatalf("the PDF must draw %q:\n%s", want, drawn)
		}
		if next < at {
			t.Fatalf("%q is drawn out of order; want %q:\n%s", want, order, drawn)
		}
		at = next
	}
}

// TestRenderOfferPDF_BlankIntroAndTermsDrawNoSection keeps an offer whose
// intro and terms were cleared — stored as empty text, not null — from
// printing a Terms heading over nothing.
func TestRenderOfferPDF_BlankIntroAndTermsDrawNoSection(t *testing.T) {
	o := testRenderOffer(100000, 19000, 119000)
	blank := ""
	o.IntroText, o.TermsText = &blank, &blank

	pdf, err := RenderOfferPDF(o, testRenderLines(), nil, issuedBy("Margince GmbH"), "de-DE", nil)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(pdfDrawnText(t, pdf), []byte("Bedingungen")) {
		t.Fatalf("blank terms must omit the terms heading:\n%s", pdfDrawnText(t, pdf))
	}
}

// issuedBy is an issuer block that carries a name and nothing else.
func issuedBy(name string) PDFIssuer {
	return PDFIssuer{Name: name}
}

// Every offer prints its issuer's legal block, and embeds the issuer's logo.
// A logo that does not decode leaves the document without one.
func TestRenderOfferPDF_PrintsTheIssuerBlockAndEmbedsItsLogo(t *testing.T) {
	o := testRenderOffer(100000, 19000, 119000)
	var logo bytes.Buffer
	if err := png.Encode(&logo, image.NewRGBA(image.Rect(0, 0, 4, 2))); err != nil {
		t.Fatal(err)
	}
	issuer := PDFIssuer{
		Name: "Acme Wien GmbH", Address: []string{"Kärntner Ring 12", "1010 Wien", "AT"},
		VatID: "ATU12345678", RegisterCourt: "HG Wien", RegisterNumber: "FN 123456 a",
		BankAccountHolder: "Acme Wien GmbH", IBAN: "AT611904300234573201", BIC: "BKAUATWW",
		Logo: logo.Bytes(),
	}
	pdf, err := RenderOfferPDF(o, testRenderLines(), nil, issuer, "de-DE", nil)
	if err != nil {
		t.Fatalf("RenderOfferPDF() error = %v", err)
	}
	drawn := pdfDrawnText(t, pdf)
	for _, want := range []string{
		"1010 Wien", "USt-IdNr.: ATU12345678", "Handelsregister: HG Wien FN 123456 a",
		"Bankverbindung: Acme Wien GmbH · AT611904300234573201 · BKAUATWW", "Kärntner Ring 12",
	} {
		if !bytes.Contains(drawn, []byte(want)) {
			t.Errorf("the PDF does not print %q:\n%s", want, drawn)
		}
	}
	if !bytes.Contains(pdf, []byte("/Subtype /Image")) {
		t.Error("the PDF embeds no logo")
	}

	issuer.Logo = []byte("not a png")
	broken, err := RenderOfferPDF(o, testRenderLines(), nil, issuer, "de-DE", nil)
	if err != nil {
		t.Fatalf("a logo that does not decode failed the render: %v", err)
	}
	if bytes.Contains(broken, []byte("/Subtype /Image")) {
		t.Error("a logo that does not decode was embedded")
	}
}

// Text typed as a letter and its accents apart reaches fpdf as the composed
// letter. Drawn mark by mark, fpdf sets a second accent beside the first.
func TestThePDFTextCheckComposesAccentsTypedApart(t *testing.T) {
	check, err := newPDFTextCheck()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := check.text("Nguye\u0302\u0303n Thi\u0323"), "Nguy\u1EC5n Th\u1ECB"; got != want {
		t.Errorf("text() = %+q, want %+q", got, want)
	}
	if err := check.err(); err != nil {
		t.Errorf("the composed letters print: %v", err)
	}
}
