// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package values

import "strings"

// ErasedEmail is the address an Art. 17 erasure writes over a record it cannot
// delete, and the one address ParseEmail refuses outright.
//
// Reserved in a second, stronger sense than the RFC 2606 names below. Those cannot
// receive mail but are perfectly storable — the test mailbox exists to send to them,
// and fixtures all over the tree seed contacts under .example and .test. This one may
// not be STORED at all, because its value is that no live record holds it: a deal-room
// seat is resolved by ADDRESS, so once an erasure has written this over one, the
// address names every seat any erasure ever wiped. A subject who also held it would be
// indistinguishable from them, and no predicate could separate the two.
//
// The eraser writes it in SQL and never through the parser, so reserving it costs the
// erasure nothing.
const ErasedEmail = "erased@example.invalid"

// reservedNames are the RFC 2606 names no real mailbox can live under. The
// three named domains admit their subdomains; the four bare labels are TLDs,
// so any host ending in one is reserved at any depth.
var reservedNames = map[string]bool{
	"example.com": true, "example.net": true, "example.org": true,
	"test": true, "example": true, "invalid": true, "localhost": true,
}

// IsReservedHost reports whether host is a reserved name or lies under one.
// Case and a trailing root dot are ignored; a look-alike such as
// notexample.com is not reserved, because only whole labels are compared.
func IsReservedHost(host string) bool {
	host = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	for host != "" {
		if reservedNames[host] {
			return true
		}
		_, rest, found := strings.Cut(host, ".")
		if !found {
			return false
		}
		host = rest
	}
	return false
}

// IsReservedAddress reports whether addr's domain is a reserved host, read
// through ParseEmail. ParseEmail refuses a quoted local part, so a reserved
// name typed in front of the "@" never makes an address reserved, and a real
// domain behind one never hides it. A malformed address is not reserved:
// nothing here can tell where it would go.
func IsReservedAddress(addr string) bool {
	email, err := parseAddress(addr)
	if err != nil {
		return false
	}
	return IsReservedHost(email.Domain())
}
