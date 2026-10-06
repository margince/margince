// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package values

import "strings"

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
	email, err := ParseEmail(addr)
	if err != nil {
		return false
	}
	return IsReservedHost(email.Domain())
}
