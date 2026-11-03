// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package owndomain

import "fmt"

// CoversSQL is CoversDomain as a SQL condition, for a query that must decide
// before its scan cap. Both arguments must be values Fold already produced:
// read from a column its writers fold, or passed in as a Set's Domains. Postgres cannot fold IDNA, so a raw
// address or a raw domain here compares two spellings of one domain as unequal.
// The suffix test uses right(), never LIKE, where an underscore is a wildcard.
func CoversSQL(domain, own string) string {
	return fmt.Sprintf(`(%[1]s = %[2]s OR right(%[1]s, length(%[2]s) + 1) = '.' || %[2]s)`, domain, own)
}

// CoversIndexedSQL is CoversSQL behind a range an index on (reverse(domain) COLLATE "C") serves.
// Reversed, a domain and its subdomains sort from reverse(own) to before reverse(own) || '/'.
func CoversIndexedSQL(domain, own string) string {
	host := `(reverse(` + domain + `) COLLATE "C")`
	return fmt.Sprintf(`(%[1]s >= (reverse(%[2]s) COLLATE "C") AND %[1]s < ((reverse(%[2]s) || '/') COLLATE "C") AND %[3]s)`,
		host, own, CoversSQL(domain, own))
}

// StoredOfAddress is what an address row stores for CoversSQL: the folded
// domain of the address, or nil when it has none.
func StoredOfAddress(address string) *string {
	return stored(OfAddress(address))
}

// StoredOwn is what an own-domain candidate stores for CoversSQL: the folded
// domain, or nil when it is blank or nobody can own it. New drops the same.
func StoredOwn(domain string) *string {
	folded := Fold(domain)
	if !Ownable(folded) {
		return nil
	}
	return stored(folded)
}

func stored(folded string) *string {
	if folded == "" {
		return nil
	}
	return &folded
}
