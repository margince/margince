// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package values

import (
	"strings"
	"unicode"
)

// fillerRunes are letters and symbols that draw nothing: the Hangul fillers and
// the blank Braille pattern. Unicode files them as graphic, so IsGraphic admits them.
var fillerRunes = map[rune]bool{
	0x115F: true, 0x1160: true, 0x2800: true, 0x3164: true, 0xFFA0: true,
}

// zeroWidthRunes are the invisible format characters with no job in a name. A
// zero-width joiner or non-joiner is left out, because the scripts that need
// one spell a real word with it.
var zeroWidthRunes = map[rune]bool{0x180E: true, 0x200B: true, 0x2060: true, 0xFEFF: true}

// IsInvisibleRune reports a rune that draws nothing by itself: white space, a
// control or format character, a private-use or unassigned code point, or a filler.
// A combining mark is not in this class, because an accent on a letter is text.
func IsInvisibleRune(r rune) bool {
	return unicode.IsSpace(r) || !unicode.IsGraphic(r) || fillerRunes[r]
}

// HasVisibleText reports whether s holds a character a reader can see.
// Trimming alone lets a zero-width space through, and a combining mark draws
// nothing without a base, so a mark does not count here.
func HasVisibleText(s string) bool {
	for _, r := range s {
		if !IsInvisibleRune(r) && !unicode.IsMark(r) {
			return true
		}
	}
	return false
}

// WithoutZeroWidth drops the zero-width and filler runes from s. A name carrying
// one would otherwise sit beside the same name without it and read as free.
func WithoutZeroWidth(s string) string {
	return strings.Map(func(r rune) rune {
		if zeroWidthRunes[r] || fillerRunes[r] {
			return -1
		}
		return r
	}, s)
}
