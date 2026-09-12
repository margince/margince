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
	"time"
	"unicode/utf8"

	"github.com/go-pdf/fpdf"

	"github.com/margince/margince/backend/internal/platform/pdftext"
)

// generous is a bound no test case here is trying to hit.
const generous = 100_000

// reader is shared across this file's cases on purpose: each Reader owns a
// compiled 5 MB WebAssembly module, and building one per test would spend a
// second per test compiling the same bytes.
var reader = pdftext.New()

func TestMain(m *testing.M) {
	code := m.Run()
	if err := reader.Close(); err != nil {
		panic("closing the PDF engine: " + err.Error())
	}
	os.Exit(code)
}

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

	text, err := reader.Read(t.Context(), raw, generous)
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

	text, err := reader.Read(t.Context(), raw, generous)
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

	_, err := reader.Read(t.Context(), out.Bytes(), generous)
	if !errors.Is(err, pdftext.ErrNoTextLayer) {
		t.Fatalf("a PDF with no text must report ErrNoTextLayer, got %v", err)
	}
}

// Whitespace is not text. A page of positioned spaces would otherwise read as a
// document that states something, and the model would be asked to find deal
// facts in it.
func TestExtractTreatsAPageOfWhitespaceAsNoTextAtAll(t *testing.T) {
	raw := invoicePDF(t, "   ", "\t", " ")

	_, err := reader.Read(t.Context(), raw, generous)
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
			_, err := reader.Read(t.Context(), raw, generous)
			if !errors.Is(err, pdftext.ErrUnreadable) {
				t.Fatalf("%s must report ErrUnreadable, got %v", name, err)
			}
		})
	}
}

// Breadth: six hundred corrupted documents, none of which may crash the process
// or come back as anything but an answer.
//
// Mutations of a VALID file rather than random bytes, because random bytes fail
// the header check and never reach the parser — the shape of corpus that reports
// a parser as safe while never having tested it.
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
		text, err := reader.Read(t.Context(), mutant, generous)
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

// The three files below are the reason this package parses inside a sandbox
// rather than in the host process. Each one defeated the pure-Go parser this
// package was written against first, and each is committed so the defeat cannot
// come back unnoticed.
//
// They are held to a DEADLINE as well as an answer. Two of the three cost
// nothing in memory and everything in time, and a test that only checked the
// verdict would pass just as happily while a worker span forever.
const attackDeadline = 5 * time.Second

// readWithin fails the test if a reading does not come back in time, rather
// than hanging the suite until the whole package times out.
func readWithin(t *testing.T, raw []byte) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), attackDeadline)
	defer cancel()

	type outcome struct{ err error }
	done := make(chan outcome, 1)
	go func() {
		_, err := reader.Read(ctx, raw, generous)
		done <- outcome{err}
	}()
	select {
	case got := <-done:
		return got.err
	case <-time.After(attackDeadline):
		t.Fatalf("the reading did not come back within %v — a document is holding the engine", attackDeadline)
		return nil
	}
}

// A cross-reference table may name any object number it likes, and the previous
// parser sized its table from that number: a 133-byte file claiming twenty
// million objects allocated 610 MB and reported SUCCESS, and the same file
// scaled up reached tens of gigabytes. A Go out-of-memory is a fatal error
// rather than a panic, so no amount of recovering caught it.
func TestAnXrefClaimingMoreObjectsThanTheFileCouldHoldIsRefused(t *testing.T) {
	raw := fixture(t, "xref-claims-millions-of-objects.pdf")

	if err := readWithin(t, raw); !errors.Is(err, pdftext.ErrUnreadable) {
		t.Fatalf("a file whose xref outruns its own length must be refused, got %v", err)
	}
}

// A page tree may list itself as its own child. The previous parser followed
// `/Kids` with no visited set, so a 224-byte file span with no exit — nothing
// panicked, nothing allocated, and the goroutine could not be stopped.
func TestAPageTreeThatListsItselfIsRefusedRatherThanFollowed(t *testing.T) {
	raw := fixture(t, "page-tree-lists-itself.pdf")

	if err := readWithin(t, raw); err == nil {
		t.Fatal("a page tree that is its own child must not read as a document")
	}
}

// One mutant of a valid file, measured to PANIC the previous parser
// (`unexpected keyword "841.8\x97" parsing object`). The engine here opens it
// and finds nothing to read, which is a better answer than a crash and still
// one the caller can route on.
func TestADocumentThatCrashedThePreviousParserIsMerelyAnswered(t *testing.T) {
	raw := fixture(t, "crashed-the-previous-parser.pdf")

	err := readWithin(t, raw)
	if !errors.Is(err, pdftext.ErrUnreadable) && !errors.Is(err, pdftext.ErrNoTextLayer) {
		t.Fatalf("a document that used to crash the parser must come back as a sentinel, got %v", err)
	}
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("reading the committed fixture: %v", err)
	}
	return raw
}

// Truncation is also refused rather than survived, and it is a different corpus
// from mutation: a prefix of a valid file has a valid header and an absent
// trailer, which is the other half of what the parser can be handed.
func TestExtractSurvivesATruncatedDocument(t *testing.T) {
	valid := invoicePDF(t, "Contract value: EUR 148,500.00")

	recovered := 0
	for cut := 1; cut < len(valid); cut += max(1, len(valid)/60) {
		text, err := reader.Read(t.Context(), valid[:cut], generous)
		switch {
		case err != nil && !errors.Is(err, pdftext.ErrUnreadable) && !errors.Is(err, pdftext.ErrNoTextLayer):
			t.Fatalf("truncation at %d reported %v, which is not a sentinel a caller can route on", cut, err)
		case err == nil:
			// A prefix CAN read: the engine recovers a document whose trailer
			// is missing by rebuilding the cross-reference table, and the last
			// ~20 bytes of this fixture are exactly that trailer. Measured: 21
			// of 1081 byte-prefixes are readable, all of them at the very end.
			//
			// So a successful reading is not a failure — but it must be a
			// reading OF THIS DOCUMENT. Recovering a file into confident
			// nonsense is the outcome that would matter, and this is what
			// separates the two.
			recovered++
			if !strings.Contains(text, "148,500.00") {
				t.Fatalf("truncation at %d was accepted and read as %q, "+
					"which is not what the document says", cut, text)
			}
		}
	}
	t.Logf("%d sampled prefixes were recovered and read correctly", recovered)
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
	text, err := reader.Read(t.Context(), raw, limit)
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
	text, err := reader.Read(t.Context(), raw, limit)
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

	text, err := reader.Read(t.Context(), raw, generous)
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

// `/Count` is a number the DOCUMENT chooses, and this package no longer has any
// arithmetic of its own to defend against it: the engine resolves the page tree
// and reports what it found, so the count reaching the page loop is a fact about
// the file rather than the file's claim about itself.
//
// Asserted because that is a PROPERTY OF THE ENGINE and not of this code — the
// kind of thing a version bump can take away silently. A document claiming a
// hundred million pages must still be read as the one page it has, promptly.
func TestALyingPageCountIsReadAsTheDocumentActuallyIs(t *testing.T) {
	lying := pdfClaimingPages(t, 99999999, "Contract value: EUR 148,500.00")

	ctx, cancel := context.WithTimeout(t.Context(), attackDeadline)
	defer cancel()

	text, err := reader.Read(ctx, lying, generous)
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

	if _, err := reader.Read(ctx, raw, generous); !errors.Is(err, pdftext.ErrUnreadable) {
		t.Fatalf("a read whose context is done must report ErrUnreadable, got %v", err)
	}
}

func TestExtractRefusesABoundItCannotHonour(t *testing.T) {
	raw := invoicePDF(t, "Contract value: EUR 148,500.00")

	for _, limit := range []int{0, -1} {
		if _, err := reader.Read(t.Context(), raw, limit); err == nil {
			t.Errorf("a bound of %d must be refused rather than silently treated as unbounded", limit)
		}
	}
}
