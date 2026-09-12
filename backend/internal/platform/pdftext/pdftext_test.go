// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package pdftext_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/go-pdf/fpdf"

	"github.com/margince/margince/backend/internal/platform/pdftext"
)

// generous is a bound no test case here is trying to hit.
const generous = 100_000

// invoicePDF builds the shape this feature actually reads: an order
// confirmation stating the four deal fields, one line each.
func invoicePDF(t *testing.T, lines ...string) []byte {
	t.Helper()
	doc := fpdf.New("P", "mm", "A4", "")
	doc.AddPage()
	doc.SetFont("Helvetica", "", 12)
	for _, line := range lines {
		doc.Cell(0, 8, line)
		doc.Ln(8)
	}
	var out bytes.Buffer
	if err := doc.Output(&out); err != nil {
		t.Fatalf("building the fixture PDF: %v", err)
	}
	return out.Bytes()
}

// A PDF an ordering system generated states its text as text, and that is the
// whole case this package exists for.
func TestExtractReadsWhatAGeneratedDocumentStates(t *testing.T) {
	raw := invoicePDF(t,
		"ACME Industrial GmbH - Order Confirmation",
		"Contract value: EUR 148,500.00",
		"Delivery date: 31 January 2027",
	)

	text, err := pdftext.Extract(t.Context(), raw, generous)
	if err != nil {
		t.Fatalf("reading a generated PDF: %v", err)
	}
	for _, want := range []string{"ACME Industrial GmbH", "EUR 148,500.00", "31 January 2027"} {
		if !strings.Contains(text, want) {
			t.Errorf("extracted text does not state %q; got %q", want, text)
		}
	}
}

// The grounding checks downstream compare a model's quote against this text
// TOKEN by token — the amount as a whole figure, the currency as a whole word.
// An extractor that dropped the thousands separator or glued the currency to
// the figure would pass the test above and still make every amount ungroundable,
// so the property is asserted in the spelling those checks use.
func TestExtractPreservesAFigureInTheSpellingAQuoteIsCheckedAgainst(t *testing.T) {
	raw := invoicePDF(t, "Contract value: EUR 148,500.00", "Deposit: EUR 500.00")

	text, err := pdftext.Extract(t.Context(), raw, generous)
	if err != nil {
		t.Fatalf("reading a generated PDF: %v", err)
	}
	if !strings.Contains(text, "EUR 148,500.00") {
		t.Fatalf("the figure must survive with its grouping and its code beside it, got %q", text)
	}
	// And the two amounts stay distinguishable: a reading that cited the deposit
	// line for the contract value is the defect the downstream token check
	// exists to catch, and it can only catch it if both lines are present.
	if !strings.Contains(text, "EUR 500.00") {
		t.Errorf("the second amount was lost, so a mis-cited quote could not be caught; got %q", text)
	}
}

// A scan carries no text, and saying so is a different answer from failing to
// read the file. A caller shows a rep something different for each.
func TestExtractSaysAScanCarriesNoText(t *testing.T) {
	// A page with no text drawn on it is what an image-only PDF's text layer
	// amounts to: present, and stating nothing.
	blank := fpdf.New("P", "mm", "A4", "")
	blank.AddPage()
	var out bytes.Buffer
	if err := blank.Output(&out); err != nil {
		t.Fatalf("building the fixture PDF: %v", err)
	}

	_, err := pdftext.Extract(t.Context(), out.Bytes(), generous)
	if !errors.Is(err, pdftext.ErrNoTextLayer) {
		t.Fatalf("a PDF with no text must report ErrNoTextLayer, got %v", err)
	}
}

// Whitespace is not text. A page of positioned spaces would otherwise read as a
// document that states something, and the model would be asked to find deal
// facts in it.
func TestExtractTreatsAPageOfWhitespaceAsNoTextAtAll(t *testing.T) {
	raw := invoicePDF(t, "   ", "\t", " ")

	_, err := pdftext.Extract(t.Context(), raw, generous)
	if !errors.Is(err, pdftext.ErrNoTextLayer) {
		t.Fatalf("whitespace-only text must report ErrNoTextLayer, got %v", err)
	}
}

func TestExtractRefusesWhatIsNotAPDF(t *testing.T) {
	for name, raw := range map[string][]byte{
		"empty":           {},
		"prose":           []byte("this is a text file, not a PDF"),
		"header only":     []byte("%PDF-1.4"),
		"zeroes":          bytes.Repeat([]byte{0}, 4096),
		"jpeg magic":      {0xFF, 0xD8, 0xFF, 0xE0},
		"truncated trail": []byte("%PDF-1.4\n1 0 obj\n<<>>\nendobj\n"),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := pdftext.Extract(t.Context(), raw, generous)
			if !errors.Is(err, pdftext.ErrUnreadable) {
				t.Fatalf("%s must report ErrUnreadable, got %v", name, err)
			}
		})
	}
}

// The defect this package's recover exists for, asserted rather than described.
//
// The PDF library panics on malformed input instead of returning an error, and
// these bytes arrive from an upload — so the crash is reachable by anyone who
// can attach a document, and one panicking reading would take every other
// reading on the worker with it.
//
// Mutations of a VALID file rather than random bytes: random bytes fail the
// header check and never reach the parser, which is exactly the shape of test
// that would report this as safe while the real case still panicked.
func TestExtractSurvivesACorruptedDocumentInsteadOfPanicking(t *testing.T) {
	valid := invoicePDF(t, "Contract value: EUR 148,500.00")
	// Seeded, so the corpus is the same 600 documents on every run: a corruption
	// test that finds a panic only sometimes reports a flake rather than a bug.
	rng := rand.New(rand.NewSource(1))

	corrupted := 0
	for range 600 {
		mutant := bytes.Clone(valid)
		for range 8 {
			mutant[rng.Intn(len(mutant))] = byte(rng.Intn(256))
		}
		// No recover here on purpose: a panic escaping Extract fails this test
		// by crashing it, which is the outcome being asserted against.
		text, err := pdftext.Extract(t.Context(), mutant, generous)
		switch {
		case err == nil && text == "":
			t.Fatal("a reading that succeeded must carry text")
		case err != nil && !errors.Is(err, pdftext.ErrUnreadable) && !errors.Is(err, pdftext.ErrNoTextLayer):
			t.Fatalf("a corrupt PDF must report a sentinel a caller can route on, got %v", err)
		case err != nil:
			corrupted++
		}
	}
	if corrupted == 0 {
		t.Fatal("no mutation was rejected at all, so the corpus is not corrupt")
	}
	t.Logf("%d of 600 mutations were refused without panicking", corrupted)
}

// The corpus above proves breadth; this proves DEPTH, and it is the one that
// holds the recover.
//
// A refusal count cannot tell a document turned away at the header from one the
// parser opened and choked on, so a corpus that stopped reaching the page reader
// would keep passing. This fixture is a specific file measured to panic inside
// the parser — `unexpected keyword "841.8\x97" parsing object` — so if the
// recover is ever removed or narrowed, this test does not fail politely: it
// crashes, which is the failure mode being guarded.
//
// Committed rather than generated: it is one mutant out of hundreds, and a test
// that re-derived it would be asserting that the generator still happens to find
// one.
func TestADocumentThatPanicsTheParserIsRefusedInstead(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "panics-the-parser.pdf"))
	if err != nil {
		t.Fatalf("reading the committed fixture: %v", err)
	}

	if _, err := pdftext.Extract(t.Context(), raw, generous); !errors.Is(err, pdftext.ErrUnreadable) {
		t.Fatalf("a document that panics the parser must come back as ErrUnreadable, got %v", err)
	}
}

// Truncation is also refused rather than survived, and it is a different corpus
// from mutation: a prefix of a valid file has a valid header and an absent
// trailer, which is the other half of what the parser can be handed.
func TestExtractSurvivesATruncatedDocument(t *testing.T) {
	valid := invoicePDF(t, "Contract value: EUR 148,500.00")

	for cut := 1; cut < len(valid); cut += max(1, len(valid)/60) {
		if _, err := pdftext.Extract(t.Context(), valid[:cut], generous); err != nil &&
			!errors.Is(err, pdftext.ErrUnreadable) && !errors.Is(err, pdftext.ErrNoTextLayer) {
			t.Fatalf("truncation at %d reported %v, which is not a sentinel a caller can route on", cut, err)
		}
	}
}

// The bound exists because a PDF's text streams are COMPRESSED: a file inside
// the caller's byte ceiling can still decompress to far more text than the
// caller can hold, so bounding the input does not bound this.
func TestExtractStopsAtTheCallersBound(t *testing.T) {
	lines := make([]string, 0, 400)
	for range 400 {
		lines = append(lines, "Contract value: EUR 148,500.00 for the packaging line retrofit")
	}
	raw := invoicePDF(t, lines...)
	// Many PAGES, not one long one. That is the SETUP the bound has to hold
	// against — an implementation that materialised every page and trimmed the
	// result would satisfy the length check below too, so what this pins is the
	// bound, not the early exit.
	if pages := bytes.Count(raw, []byte("/Type /Page\n")); pages < 5 {
		t.Fatalf("the fixture spans %d page(s); this test needs enough that stopping early matters", pages)
	}

	const limit = 100
	text, err := pdftext.Extract(t.Context(), raw, limit)
	if err != nil {
		t.Fatalf("reading a long PDF: %v", err)
	}
	// At most limit+1: the one rune past the bound is what lets a caller tell a
	// document that exactly fits from one that was cut off.
	if got := utf8.RuneCountInString(text); got > limit+1 {
		t.Fatalf("extracted %d runes against a bound of %d; the bound is not holding", got, limit)
	}
}

// The bound counts RUNES. Counted in bytes it would be a third of itself on a
// document of accented text, silently cutting a European invoice short.
func TestTheBoundCountsCharactersRatherThanBytes(t *testing.T) {
	lines := make([]string, 0, 200)
	for range 200 {
		lines = append(lines, "Zahlungsbedingungen: 30 Tage netto - Lieferung frei Haus")
	}
	raw := invoicePDF(t, lines...)

	const limit = 300
	text, err := pdftext.Extract(t.Context(), raw, limit)
	if err != nil {
		t.Fatalf("reading a long PDF: %v", err)
	}
	if got := utf8.RuneCountInString(text); got < limit {
		t.Errorf("a %d-rune bound yielded only %d runes, so the bound is being read as bytes", limit, got)
	}
	if !utf8.ValidString(text) {
		t.Error("the bound cut a rune in half")
	}
}

// A document printed by a REAL generator, which every case above is not.
//
// fpdf writes one text block per line and no ligatures; a browser positions
// words with Td/Tm and emits the ligature glyphs its embedded font provides.
// Those are different enough that a suite built only on the friendly shape can
// report this package as working while every invoice a customer actually sends
// reads back wrong — so one real file is committed and read as bytes.
//
// The ligature is the case it caught. Chrome prints "retrofit" as
// `retro` + U+FB01, and the downstream grounding check is a plain substring
// match: left folded, a model that quotes what the PAGE shows has its whole
// reading refused for quoting the document correctly.
func TestARealGeneratorsDocumentReadsBackAsItsWords(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "chrome-order-confirmation.pdf"))
	if err != nil {
		t.Fatalf("reading the committed fixture: %v", err)
	}

	text, err := pdftext.Extract(t.Context(), raw, generous)
	if err != nil {
		t.Fatalf("reading a browser-printed PDF: %v", err)
	}
	// The four things a reading of this document has to be able to quote, in the
	// spelling the grounding checks compare under.
	for _, want := range []string{"EUR 148,500.00", "EUR 500.00", "NL-2027-0041", "31 January 2027"} {
		if !strings.Contains(text, want) {
			t.Errorf("extracted text does not state %q; got:\n%s", want, text)
		}
	}
	// "Packaging line 3 retrofit" is printed with a ﬁ ligature.
	if !strings.Contains(text, "retrofit") {
		t.Errorf("a ligature was left as the single rune the font draws, so a quote of "+
			"the word it spells cannot be found in this text; got:\n%s", text)
	}
	for _, ligature := range []string{"ﬀ", "ﬁ", "ﬂ", "ﬃ", "ﬄ"} {
		if strings.Contains(text, ligature) {
			t.Errorf("ligature %q survived normalisation", ligature)
		}
	}
}

// pdfClaimingPages hand-writes a one-page PDF whose `/Count` says whatever it is
// told, with a correct cross-reference table.
//
// Hand-written because the alternative does not work: rewriting `/Count` inside
// a file some writer produced changes the file's length, every later object
// offset in its xref is then wrong, and the parser refuses the document before
// it ever reads a page count. A test built that way passes on the refusal and
// never reaches the bound it claims to check.
func pdfClaimingPages(t *testing.T, claimed int, line string) []byte {
	t.Helper()
	content := fmt.Sprintf("BT /F1 12 Tf 72 720 Td (%s) Tj ET\n", line)
	objects := []string{
		"<</Type/Catalog/Pages 2 0 R>>",
		fmt.Sprintf("<</Type/Pages/Kids[3 0 R]/Count %d>>", claimed),
		"<</Type/Page/Parent 2 0 R/MediaBox[0 0 612 792]/Contents 4 0 R" +
			"/Resources<</Font<</F1 5 0 R>>>>>>",
		fmt.Sprintf("<</Length %d>>stream\n%sendstream", len(content), content),
		"<</Type/Font/Subtype/Type1/BaseFont/Helvetica>>",
	}

	var out bytes.Buffer
	out.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objects))
	for i, body := range objects {
		offsets[i] = out.Len()
		fmt.Fprintf(&out, "%d 0 obj%s endobj\n", i+1, body)
	}
	startxref := out.Len()
	fmt.Fprintf(&out, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for _, at := range offsets {
		fmt.Fprintf(&out, "%010d 00000 n \n", at)
	}
	fmt.Fprintf(&out, "trailer<</Size %d/Root 1 0 R>>\nstartxref\n%d\n%%%%EOF\n",
		len(objects)+1, startxref)
	return out.Bytes()
}

// `/Count` is a number the DOCUMENT chooses, and the parser returns it from
// NumPage without checking it against anything. Honouring it walks the page tree
// that many times — and the library keeps no object cache, so each walk
// re-parses — which turns a sub-kilobyte file into unbounded work.
//
// The walk is bounded by what the FILE could really hold instead, and this
// asserts the arithmetic rather than the survival: the fixture is small enough
// that len/minBytesPerPage is far below the claim, so the claim must be the
// thing discarded. No deadline, because a timeout rescuing it would prove
// nothing about the bound.
func TestALyingPageCountIsBoundedByTheFileItself(t *testing.T) {
	const claimed = 99999999
	lying := pdfClaimingPages(t, claimed, "Contract value: EUR 148,500.00")

	// The premise, checked rather than assumed: the file's own length has to cap
	// the walk well below what the document claims, or this test is measuring
	// nothing. 64 is minBytesPerPage; naming it here rather than importing it
	// keeps the production constant unexported.
	if affordable := len(lying) / 64; affordable >= claimed {
		t.Fatalf("the fixture is %d bytes and could afford %d pages, which is not below its claim of %d",
			len(lying), affordable, claimed)
	}

	text, err := pdftext.Extract(t.Context(), lying, generous)
	// Unconditional: a document that parses must still be READ. Accepting
	// ErrUnreadable here is what let an earlier version of this test pass while
	// the parser refused the file before the page count was ever consulted.
	if err != nil {
		t.Fatalf("a lying page count must not change the answer, got %v", err)
	}
	if !strings.Contains(text, "148,500.00") {
		t.Errorf("the one real page was not read; got %q", text)
	}
}

// The parser follows `/Parent` and `/Pages` with no cycle guard, so a document
// whose page tree points at itself spins forever. Nothing panics, so the recover
// cannot see it; only a deadline can. An already-cancelled context is the
// deterministic stand-in for that document — a real cycle fixture would hang
// this test for as long as the bug lasted.
func TestExtractGivesUpWhenItsContextDoes(t *testing.T) {
	raw := invoicePDF(t, "Contract value: EUR 148,500.00")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := pdftext.Extract(ctx, raw, generous); !errors.Is(err, pdftext.ErrUnreadable) {
		t.Fatalf("a read whose context is done must report ErrUnreadable, got %v", err)
	}
}

func TestExtractRefusesABoundItCannotHonour(t *testing.T) {
	raw := invoicePDF(t, "Contract value: EUR 148,500.00")

	for _, limit := range []int{0, -1} {
		if _, err := pdftext.Extract(t.Context(), raw, limit); err == nil {
			t.Errorf("a bound of %d must be refused rather than silently treated as unbounded", limit)
		}
	}
}
