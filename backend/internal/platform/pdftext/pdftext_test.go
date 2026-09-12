// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package pdftext_test

import (
	"bytes"
	"errors"
	"math/rand"
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

	text, err := pdftext.Extract(raw, generous)
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

	text, err := pdftext.Extract(raw, generous)
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

	_, err := pdftext.Extract(out.Bytes(), generous)
	if !errors.Is(err, pdftext.ErrNoTextLayer) {
		t.Fatalf("a PDF with no text must report ErrNoTextLayer, got %v", err)
	}
}

// Whitespace is not text. A page of positioned spaces would otherwise read as a
// document that states something, and the model would be asked to find deal
// facts in it.
func TestExtractTreatsAPageOfWhitespaceAsNoTextAtAll(t *testing.T) {
	raw := invoicePDF(t, "   ", "\t", " ")

	_, err := pdftext.Extract(raw, generous)
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
			_, err := pdftext.Extract(raw, generous)
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
		text, err := pdftext.Extract(mutant, generous)
		switch {
		case err == nil && text == "":
			t.Fatal("a reading that succeeded must carry text")
		case err != nil && !errors.Is(err, pdftext.ErrUnreadable) && !errors.Is(err, pdftext.ErrNoTextLayer):
			t.Fatalf("a corrupt PDF must report a sentinel a caller can route on, got %v", err)
		case err != nil:
			corrupted++
		}
	}
	// The corpus has to actually reach the parser, or this test passes by
	// testing nothing — the census-of-zero failure mode. Some mutations land in
	// the header and are refused early; enough must land deeper.
	if corrupted == 0 {
		t.Fatal("no mutation was rejected, so this test never exercised the parser it exists to guard")
	}
	t.Logf("%d of 600 mutations were refused without panicking", corrupted)
}

// Truncation is also refused rather than survived, and it is a different corpus
// from mutation: a prefix of a valid file has a valid header and an absent
// trailer, which is the other half of what the parser can be handed.
func TestExtractSurvivesATruncatedDocument(t *testing.T) {
	valid := invoicePDF(t, "Contract value: EUR 148,500.00")

	for cut := 1; cut < len(valid); cut += max(1, len(valid)/60) {
		if _, err := pdftext.Extract(valid[:cut], generous); err != nil &&
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

	const limit = 100
	text, err := pdftext.Extract(raw, limit)
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
	text, err := pdftext.Extract(raw, limit)
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

func TestExtractRefusesABoundItCannotHonour(t *testing.T) {
	raw := invoicePDF(t, "Contract value: EUR 148,500.00")

	for _, limit := range []int{0, -1} {
		if _, err := pdftext.Extract(raw, limit); err == nil {
			t.Errorf("a bound of %d must be refused rather than silently treated as unbounded", limit)
		}
	}
}
