// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

// SeatLoginIsSQL is the predicate that the login of the app_user aliased seat
// is address, in either spelling of its domain. Every exact seat match reads
// it, which TestEveryExactSeatMatchFoldsTheDomain holds.
//
// address is a lowercased SQL expression and domainFolded its domain through
// owndomain.Fold. Postgres cannot fold IDNA, so the login compares as its
// lowercased local part and the email_domain_folded its writers store. The raw
// arm answers for a seat the backfill has not reached yet.
func SeatLoginIsSQL(seat, address, domainFolded string) string {
	return "(lower(" + seat + ".email) = " + address +
		" OR (" + seat + ".email_domain_folded = " + domainFolded +
		" AND lower(regexp_replace(" + seat + ".email, '@[^@]*$', '')) = regexp_replace(" + address + ", '@[^@]*$', '')))"
}

// SeatLoginPrefersTypedSQL orders the seats SeatLoginIsSQL matched so the one
// spelled as the caller typed it comes first, and takes that one.
//
// A unicode domain and its punycode form are two rows, because the login's
// unique index reads the raw spelling. The folded arm matches both, and a
// caller reading a single row would otherwise get either of them.
func SeatLoginPrefersTypedSQL(seat, address string) string {
	return "ORDER BY (lower(" + seat + ".email) = " + address + ") DESC, " + seat + ".id LIMIT 1"
}
