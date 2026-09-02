// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package collections

import (
	"errors"
	"strings"
	"testing"
)

// The spelling rule is one function because three write paths share it —
// create, rename, and the name an agent resolves against. These cases are the
// collisions it exists to prevent, not a tour of the string library: each one
// is a pair a reader would call the same tag and a database would not.
func TestNormalizeTagName(t *testing.T) {
	for _, c := range []struct {
		name string
		in   string
		want string
	}{
		{"the ends are trimmed", "  Key Account  ", "Key Account"},
		{"an inner run collapses", "Key   Account", "Key Account"},
		{"a tab is whitespace like any other", "Key\tAccount", "Key Account"},
		{"a newline does not survive into a badge", "Key\nAccount", "Key Account"},
		{"an already-clean name is untouched", "Key Account", "Key Account"},
		{"nothing but whitespace normalizes to nothing", "   \t ", ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := NormalizeTagName(c.in); got != c.want {
				t.Errorf("NormalizeTagName(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// The two spellings a database would hold as separate rows and a person would
// read as one word. This is the whole reason the rule exists, so it is asserted
// as a property rather than left implied by the cases above.
func TestNormalizeTagNameFoldsWhatAReaderCannotTellApart(t *testing.T) {
	if NormalizeTagName("Key  Account") != NormalizeTagName("Key Account") {
		t.Error("two spacings of one name normalize differently, so the vocabulary can hold both")
	}
}

func TestValidateTagNameRefusesWhatCannotBeATag(t *testing.T) {
	t.Run("an empty name is refused, and says which field", func(t *testing.T) {
		_, err := ValidateTagName("   ")
		var bad *BadInputError
		if !errors.As(err, &bad) || bad.Field != "name" {
			t.Fatalf("err = %v, want a BadInputError naming `name`", err)
		}
	})

	t.Run("the cap counts runes, not bytes", func(t *testing.T) {
		// 64 two-byte runes: 128 bytes, and exactly at the limit. A byte cap
		// would refuse this while admitting 64 ASCII characters, which would
		// make the same name legal in English and illegal in German.
		if _, err := ValidateTagName(strings.Repeat("ü", 64)); err != nil {
			t.Fatalf("64 multi-byte runes were refused: %v", err)
		}
		if _, err := ValidateTagName(strings.Repeat("ü", 65)); err == nil {
			t.Error("65 runes were admitted; the cap is 64")
		}
	})

	t.Run("the returned name is the normalized one", func(t *testing.T) {
		got, err := ValidateTagName("  Key   Account ")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "Key Account" {
			t.Errorf("got %q, want the normalized %q — a caller that stores the raw input reintroduces the collision", got, "Key Account")
		}
	})

	t.Run("length is measured after normalizing", func(t *testing.T) {
		// 64 characters plus inner double-spaces: over the cap as typed, at it
		// once collapsed. Measuring first would refuse a name that fits.
		typed := strings.Repeat("ab ", 21) + "c" // 21*3+1 = 64 runes normalized
		if _, err := ValidateTagName(strings.ReplaceAll(typed, " ", "  ")); err != nil {
			t.Errorf("a name that fits once collapsed was refused: %v", err)
		}
	})
}
