// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package commsauthz

import "strings"

// CountryOverride is the country a sender states for one recipient of one
// message. The marketing rules follow it in place of the country the record
// states, and nothing writes it to the record.
type CountryOverride struct {
	Address string
	// Country is a lower-case ISO 3166-1 alpha-2 code. Empty says the sender
	// cleared it, which judges the recipient under the baseline even when the
	// record states a country.
	Country string
}

// Cleared reports whether the sender removed the recipient's country.
func (o CountryOverride) Cleared() bool { return o.Country == "" }

// CountryOverrideFor answers what the sender stated about one address. An
// entry for an address the message does not carry is never asked for, so it
// changes nothing.
func CountryOverrideFor(overrides []CountryOverride, address string) (CountryOverride, bool) {
	want := strings.ToLower(strings.TrimSpace(address))
	if want == "" {
		return CountryOverride{}, false
	}
	for _, o := range overrides {
		if strings.ToLower(strings.TrimSpace(o.Address)) == want {
			return o, true
		}
	}
	return CountryOverride{}, false
}

// CountryOverridesFor keeps the one entry about address, or none.
func CountryOverridesFor(overrides []CountryOverride, address string) []CountryOverride {
	if o, ok := CountryOverrideFor(overrides, address); ok {
		return []CountryOverride{o}
	}
	return nil
}
