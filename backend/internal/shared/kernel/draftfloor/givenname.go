// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package draftfloor

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// vietnameseFamilyNames are the common Vietnamese family names, folded to
// ASCII. A Vietnamese name puts the family name first and the given name last.
// The screen keeps the same list in frontend/src/format/greetingname.ts, held
// by backend/gates/frontendgivenname_test.go.
var vietnameseFamilyNames = map[string]bool{
	"nguyen": true, "tran": true, "le": true, "pham": true, "hoang": true, "huynh": true,
	"phan": true, "vu": true, "vo": true, "dang": true, "bui": true, "do": true,
	"ho": true, "ngo": true, "duong": true, "ly": true, "dinh": true, "truong": true,
	"dao": true, "doan": true, "trinh": true, "luong": true, "cao": true, "chu": true,
	"quach": true, "tang": true, "thai": true, "trieu": true, "kieu": true, "vuong": true,
	"luu": true, "phung": true, "lam": true, "ta": true,
}

// VietnameseFamilyNames lists the folded family names, for the parity gate.
func VietnameseFamilyNames() []string {
	out := make([]string, 0, len(vietnameseFamilyNames))
	for name := range vietnameseFamilyNames {
		out = append(out, name)
	}
	return out
}

// GivenName is the word of a display name a greeting uses. That is the last
// word after a Vietnamese family name, else the first word.
func GivenName(display string) string {
	fields := strings.Fields(OneLine(display))
	switch {
	case len(fields) == 0:
		return ""
	case len(fields) > 1 && vietnameseFamilyNames[foldName(fields[0])]:
		return fields[len(fields)-1]
	default:
		return fields[0]
	}
}

// foldName lower-cases a word and drops its marks, so "Nguyễn" and "Đặng" read
// as "nguyen" and "dang".
func foldName(word string) string {
	var b strings.Builder
	for _, r := range norm.NFD.String(strings.ToLower(word)) {
		switch {
		case unicode.Is(unicode.Mn, r):
		case r == 'đ':
			b.WriteRune('d')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
