// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package values

import (
	"net/url"
	"strings"
	"unicode/utf8"
)

// WebAddressCharacters bounds a stored web address. Browsers and servers
// commonly stop at about 2000 characters, so a longer one is not a link anyone
// can follow.
const WebAddressCharacters = TextNoteCharacters

// IsWebAddress reports whether s may be stored as a web address: one
// FollowableWebAddress admits, of at most WebAddressCharacters. The frontend's
// isWebAddress (frontend/src/format/weburl.ts) draws the same line.
func IsWebAddress(s string) bool {
	return utf8.RuneCountInString(s) <= WebAddressCharacters && FollowableWebAddress(s)
}

// FollowableWebAddress reports whether s is an absolute http or https address a
// reader may follow. The scheme is matched without case, as RFC 3986 says and
// browsers do. A host is required, so `https:///path` is refused. An address
// with a user name or password is refused: `https://bank.example@evil.example`
// reads as one site and opens another. Whitespace and control characters are
// refused, so a value cannot carry a second line into a header or a mail.
func FollowableWebAddress(s string) bool {
	if s == "" || strings.ContainsFunc(s, func(r rune) bool { return r <= ' ' || r == 0x7f }) {
		return false
	}
	parsed, err := url.Parse(s)
	if err != nil {
		return false
	}
	scheme := strings.ToLower(parsed.Scheme)
	return (scheme == "http" || scheme == "https") && parsed.Host != "" && parsed.User == nil
}
