// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package glyphcover

import (
	"testing"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
)

func TestPrintsLeftToRightNeedsAGlyphInEveryFace(t *testing.T) {
	typeface, err := Parse(goregular.TTF, gobold.TTF)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	lookup := typeface.Lookup()
	for _, r := range "Aßđ€ЖΩ" {
		if !lookup.PrintsLeftToRight(r) {
			t.Errorf("the Go fonts draw %q, so it must print", r)
		}
	}
	for _, r := range "李😀" {
		if lookup.PrintsLeftToRight(r) {
			t.Errorf("the Go fonts have no glyph for %q, so it must not print", r)
		}
	}
}

func TestParseRefusesBytesThatAreNoFont(t *testing.T) {
	if _, err := Parse(goregular.TTF, []byte("not a font")); err == nil {
		t.Fatal("Parse() must refuse bytes that are no font face")
	}
}

func TestComposedJoinsALetterWithTheAccentsTypedAfterIt(t *testing.T) {
	if got, want := Composed("e\u0302\u0303 i\u0323"), "\u1EC5 \u1ECB"; got != want {
		t.Errorf("Composed() = %+q, want %+q", got, want)
	}
}
