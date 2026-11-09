// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package promptref

import (
	"cmp"
	"slices"
	"strings"
	"unicode"
)

// Rewrite replaces every id the tables hold with its ref, so a payload built
// with record ids reaches the model in refs. The longest id goes first, so an
// id that contains another, such as "activity:<uuid>" beside "<uuid>", keeps
// its own ref.
func Rewrite(text string, tables ...*Refs) string {
	type pair struct{ id, ref string }
	var pairs []pair
	for _, table := range tables {
		for _, ref := range table.refs {
			pairs = append(pairs, pair{id: table.idOf[ref], ref: ref})
		}
	}
	slices.SortStableFunc(pairs, func(a, b pair) int { return cmp.Compare(len(b.id), len(a.id)) })
	for _, p := range pairs {
		if p.id != "" {
			text = strings.ReplaceAll(text, p.id, p.ref)
		}
	}
	return text
}

// NamedIn returns the first ref of the table that text spells as a word of its
// own, and false when it spells none. "m1" is found in "see m1." and not in
// "m10" or "xm1": a ref in prose a reader sees is a leak, as an id would be.
func (r *Refs) NamedIn(text string) (string, bool) {
	for _, ref := range r.refs {
		for at := strings.Index(text, ref); at >= 0; {
			end := at + len(ref)
			if !wordRune(text, at-1) && !wordRune(text, end) {
				return ref, true
			}
			next := strings.Index(text[at+1:], ref)
			if next < 0 {
				break
			}
			at += 1 + next
		}
	}
	return "", false
}

// wordRune reports whether the byte at i continues a word: a letter or digit.
// An index outside text is no word.
func wordRune(text string, i int) bool {
	if i < 0 || i >= len(text) {
		return false
	}
	c := rune(text[i])
	return c < 0x80 && (unicode.IsLetter(c) || unicode.IsDigit(c))
}
