// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httperr

import (
	"fmt"
	"strings"
)

// dispositionFilename spells the filename parameters of a Content-Disposition
// (RFC 6266 section 4.3). `filename=` is an ASCII fallback. A name with other
// characters also gets `filename*=` with the real name.
// Without it a browser saves "Angebot Müller.txt" as "download".
func dispositionFilename(name string) string {
	var fallback strings.Builder
	ascii := true
	for _, r := range name {
		switch {
		case r < 0x20 || r == 0x7f:
			ascii = false
		case r > 0x7e:
			ascii = false
			fallback.WriteByte('_')
		case r == '"' || r == '\\':
			fallback.WriteByte('\\')
			fallback.WriteRune(r)
		default:
			fallback.WriteRune(r)
		}
	}
	out := `; filename="` + fallback.String() + `"`
	if ascii {
		return out
	}
	return out + "; filename*=UTF-8''" + percentEncodeAttrChars(name)
}

// percentEncodeAttrChars encodes every byte outside RFC 5987's attr-char set.
func percentEncodeAttrChars(s string) string {
	const attrSpecials = "!#$&+-.^_`|~"
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 0x80 || !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.IndexByte(attrSpecials, c) >= 0) {
			fmt.Fprintf(&b, "%%%02X", c)
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}
