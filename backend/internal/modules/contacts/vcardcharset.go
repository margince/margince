// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
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
	if vcardUTF8Labels[charset] {
		if decoded, err := charmap.Windows1252.NewDecoder().String(value); err == nil && utf8.ValidString(decoded) {
			return decoded, true
		}
	}
	return "", false
}

func vcardCharset(params []string) string {
	for _, p := range params {
		if key, value, found := strings.Cut(p, "="); found && strings.EqualFold(key, "CHARSET") {
			return strings.ToUpper(strings.Trim(value, `"`))
		}
	}
	return ""
}
