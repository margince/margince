// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package owndomain

import "slices"

// Parties is what a meeting's sides are judged against. Capture asks it whether
// a meeting is internal. Deal evidence asks it whether an attendee is the buyer.
type Parties struct {
	// Trusted are the domains a human vouched for: claimed or confirmed.
	Trusted Set
	// Possible adds the domains a connected mailbox suggested.
	Possible Set
	// Consumer reports whether a domain is a consumer mail provider. Nil
	// treats no domain as one.
	Consumer func(domain string) bool
}

// SeatDomainIsOurs reports whether the domain of the seat a meeting was synced
// for proves the parties on it are colleagues.
//
// A consumer domain never proves it: two strangers can both use one. Without
// trusted domains, any other seat domain does. With them, the seat's domain
// must be one the workspace might own.
func (p Parties) SeatDomainIsOurs(domain string) bool {
	domain = Fold(domain)
	if domain == "" || (p.Consumer != nil && p.Consumer(domain)) {
		return false
	}
	if p.Trusted.Empty() {
		return true
	}
	return p.Possible.CoversDomain(domain)
}

// IsOutsideAttendee reports whether an attendee's address shows somebody from
// the other side. seatDomains are the folded domains of our seats on the
// meeting, as OfAddress returns them.
//
// An address with no readable domain is not outside: an unknown guest proves
// nothing about who attended.
func (p Parties) IsOutsideAttendee(address string, seatDomains []string) bool {
	domain := OfAddress(address)
	if domain == "" || p.Trusted.CoversDomain(domain) {
		return false
	}
	return !slices.Contains(seatDomains, domain) || !p.SeatDomainIsOurs(domain)
}
