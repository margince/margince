// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/japanese"
)

// vcardCharsets are the CHARSET values a card's bytes are read in. vCard 2.1
// exporters write Latin-1 and Windows-1252 as often as UTF-8.
var vcardCharsets = map[string]encoding.Encoding{
	"ISO-8859-1": charmap.ISO8859_1, "ISO8859-1": charmap.ISO8859_1, "LATIN1": charmap.ISO8859_1, "LATIN-1": charmap.ISO8859_1,
	"WINDOWS-1252": charmap.Windows1252, "CP1252": charmap.Windows1252,
}

// vcardUTF8Labels are the CHARSET values that promise UTF-8, and the empty
// label of a card that names none.
var vcardUTF8Labels = map[string]bool{"": true, "UTF-8": true, "UTF8": true, "US-ASCII": true, "ASCII": true}

// vcardText reads a property's bytes in the charset the card names. A value
// that promises UTF-8 but is not, which is what an Outlook 2.1 export looks
// like, is read as Windows-1252. A value in a charset this reader does not know
// and that is not UTF-8 is unreadable: ok is false and the text is empty.
func vcardText(params []string, value string) (text string, ok bool) {
	charset := vcardCharset(params)
	if decoder, known := vcardCharsets[charset]; known {
		decoded, err := decoder.NewDecoder().String(value)
		if err == nil {
			value = decoded
		}
	}
	if utf8.ValidString(value) {
		return value, true
	}
	if vcardUTF8Labels[charset] && plausiblyLatin(value) {
		if decoded, err := charmap.Windows1252.NewDecoder().String(value); err == nil && utf8.ValidString(decoded) {
			return decoded, true
		}
	}
	return "", false
}

// plausiblyLatin says whether bytes that are not UTF-8 are more likely Western
// text than Shift-JIS or GB2312. Guessing Windows-1252 for those would store
// mojibake, so they are left unreadable and the card is reported.
func plausiblyLatin(value string) bool {
	for i := 0; i < len(value); i++ {
		switch value[i] {
		case 0x81, 0x8D, 0x8F, 0x90, 0x9D:
			return false
		}
	}
	return !looksShiftJIS(value) && !looksGB2312(value)
}

func looksShiftJIS(value string) bool {
	decoded, err := japanese.ShiftJIS.NewDecoder().String(value)
	if err != nil || strings.ContainsRune(decoded, utf8.RuneError) {
		return false
	}
	han, kana := 0, 0
	for _, r := range decoded {
		switch {
		case unicode.Is(unicode.Han, r):
			han++
		case unicode.Is(unicode.Hiragana, r), unicode.Is(unicode.Katakana, r):
			kana++
		}
	}
	// A single kanji after one accented letter reads as Latin text as often as
	// Japanese, so it counts only with a kana, a second kanji, or a lead byte
	// that Western text hardly uses.
	return kana > 0 || han > 1 || (han > 0 && hasC1Byte(value))
}

// looksGB2312 finds a double-byte run in GB2312's own range, which Western text
// seldom produces: two accented letters in a row.
func looksGB2312(value string) bool {
	for i := 0; i+1 < len(value); i++ {
		if value[i] >= 0xA1 && value[i] <= 0xF7 && value[i+1] >= 0xA1 && value[i+1] <= 0xFE {
			return true
		}
	}
	return false
}

func hasC1Byte(value string) bool {
	for i := 0; i < len(value); i++ {
		if value[i] >= 0x80 && value[i] <= 0x9F {
			return true
		}
	}
	return false
}

func vcardCharset(params []string) string {
	for _, p := range params {
		if key, value, found := strings.Cut(p, "="); found && strings.EqualFold(key, "CHARSET") {
			return strings.ToUpper(strings.Trim(value, `"`))
		}
	}
	return ""
}
