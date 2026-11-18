// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package glyphcover answers which characters a typeface can print from left
// to right. A renderer asks first, so it can refuse text before it draws a box.
package glyphcover

import (
	"fmt"

	"golang.org/x/image/font/sfnt"
	"golang.org/x/text/unicode/bidi"
)

// Typeface is a set of parsed faces that must each draw a character, such as
// a regular and a bold face. It is read-only and safe to share.
type Typeface struct {
	faces []*sfnt.Font
}

// Parse reads each TrueType or OpenType face of one typeface.
func Parse(faces ...[]byte) (*Typeface, error) {
	parsed := make([]*sfnt.Font, 0, len(faces))
	for _, data := range faces {
		face, err := sfnt.Parse(data)
		if err != nil {
			return nil, fmt.Errorf("parse a font face: %w", err)
		}
		parsed = append(parsed, face)
	}
	return &Typeface{faces: parsed}, nil
}

// Lookup answers for one caller at a time, since it holds sfnt's scratch buffer.
type Lookup struct {
	faces []*sfnt.Font
	buf   sfnt.Buffer
}

// Lookup starts a fresh lookup over the typeface's faces, one per render.
func (t *Typeface) Lookup() *Lookup {
	return &Lookup{faces: t.faces}
}

// PrintsLeftToRight is false for a right-to-left letter, which a left-to-right
// renderer would lay out reversed, and for a character a face has no glyph for.
func (l *Lookup) PrintsLeftToRight(r rune) bool {
	if props, _ := bidi.LookupRune(r); props.Class() == bidi.R || props.Class() == bidi.AL {
		return false
	}
	for _, face := range l.faces {
		if glyph, err := face.GlyphIndex(&l.buf, r); err != nil || glyph == 0 {
			return false
		}
	}
	return true
}
