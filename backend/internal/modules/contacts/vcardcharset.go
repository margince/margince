// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"log/slog"
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

// vcardText reads a property's bytes in the charset the card names. A value
// that is still not UTF-8 is dropped, because the database refuses it and no
// guess would spell what the card meant.
func vcardText(params []string, value string) string {
	charset := vcardCharset(params)
	if decoder, known := vcardCharsets[charset]; known {
		decoded, err := decoder.NewDecoder().String(value)
		if err == nil {
			value = decoded
		}
	}
	if !utf8.ValidString(value) {
		slog.Warn("vcard: dropping a property value that is not readable text", "charset", charset)
		return ""
	}
	return value
}

func vcardCharset(params []string) string {
	for _, p := range params {
		if key, value, found := strings.Cut(p, "="); found && strings.EqualFold(key, "CHARSET") {
			return strings.ToUpper(strings.Trim(value, `"`))
		}
	}
	return ""
}
