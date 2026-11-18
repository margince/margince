// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// The typeface every offer PDF prints in, and the check that refuses text it
// cannot print. Liberation Sans (OFL-1.1, fonts/LICENSE) holds Latin with
// Vietnamese and Central European letters, Greek and Cyrillic. fpdf embeds only
// the glyphs a document uses.

import (
	_ "embed"
	"fmt"
	"strings"
	"sync"
	"unicode"

	"github.com/go-pdf/fpdf"

	"github.com/margince/margince/backend/internal/platform/glyphcover"
)

const offerFontFamily = "LiberationSans"

var (
	//go:embed fonts/LiberationSans-Regular.ttf
	offerFontRegular []byte
	//go:embed fonts/LiberationSans-Bold.ttf
	offerFontBold []byte
)

// offerTypeface parses the two embedded faces once, for the glyph lookups.
var offerTypeface = sync.OnceValues(func() (*glyphcover.Typeface, error) {
	typeface, err := glyphcover.Parse(offerFontRegular, offerFontBold)
	if err != nil {
		return nil, fmt.Errorf("parse the offer PDF font: %w", err)
	}
	return typeface, nil
})

func registerOfferFont(pdf *fpdf.Fpdf) {
	pdf.AddUTF8FontFromBytes(offerFontFamily, "", offerFontRegular)
	pdf.AddUTF8FontFromBytes(offerFontFamily, "B", offerFontBold)
}

// pdfTextCheck collects the characters the font has no glyph for. It counts a
// right-to-left letter as one too, since fpdf lays it out reversed.
type pdfTextCheck struct {
	glyphs    *glyphcover.Lookup
	printable map[rune]bool
	missing   []rune
}

func newPDFTextCheck() (*pdfTextCheck, error) {
	typeface, err := offerTypeface()
	if err != nil {
		return nil, err
	}
	return &pdfTextCheck{glyphs: typeface.Lookup(), printable: map[rune]bool{}}, nil
}

// text records what s holds that cannot be printed, and hands s back with each
// such character as "?". fpdf's width tables stop at U+FFFF and index past
// them, so an emoji must never reach a draw call; the render is refused anyway.
func (c *pdfTextCheck) text(s string) string {
	return strings.Map(func(r rune) rune {
		ok, seen := c.printable[r]
		if !seen {
			ok = c.hasGlyph(r)
			c.printable[r] = ok
			if !ok {
				c.missing = append(c.missing, r)
			}
		}
		if !ok {
			return '?'
		}
		return r
	}, s)
}

func (c *pdfTextCheck) hasGlyph(r rune) bool {
	return unicode.IsControl(r) || c.glyphs.PrintsLeftToRight(r)
}

func (c *pdfTextCheck) err() error {
	if len(c.missing) == 0 {
		return nil
	}
	return &UnprintableTextError{Chars: c.missing}
}

// unprintableShown bounds how many refused characters the message lists.
const unprintableShown = 20

// UnprintableTextError refuses a render whose text the offer font cannot
// print, rather than storing a document with substitutes in its place.
type UnprintableTextError struct {
	Chars []rune
}

func (e *UnprintableTextError) Error() string {
	_, message := e.MessageFault()
	return message
}

// MessageFault names the characters, so the seller knows what to change.
func (e *UnprintableTextError) MessageFault() (code, message string) {
	shown := e.Chars
	if len(shown) > unprintableShown {
		shown = shown[:unprintableShown]
	}
	names := make([]string, 0, len(shown))
	for _, r := range shown {
		names = append(names, fmt.Sprintf("%c (U+%04X)", r, r))
	}
	return "offer_pdf_unprintable_text",
		"The offer PDF cannot print " + strings.Join(names, ", ") +
			". Change them in the offer, its lines, the buyer's name or the template, then render again."
}
