// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// A card's ADR property, as one line and as the record's address parts.

import (
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// The order a card's addresses are preferred in: the one it marks PREF, then
// its work address, then the first it lists.
const (
	addressRankPreferred = iota + 1
	addressRankWork
	addressRankOther
)

// readVCardAddress keeps the ADR the card prefers most. Of two equally
// preferred, the first stays.
func readVCardAddress(entry *VCardEntry, params []string, raw string) {
	rank := vcardAddressRank(params)
	if entry.addressRank != 0 && entry.addressRank <= rank {
		return
	}
	line := addressFromStructured(params, raw)
	if line == "" {
		return
	}
	entry.Address, entry.PostalAddress, entry.addressRank = line, postalAddressFromStructured(params, raw), rank
}

func vcardAddressRank(params []string) int {
	for _, p := range params {
		key, value, _ := strings.Cut(strings.ToUpper(p), "=")
		if key == "PREF" || (key == "TYPE" && strings.Contains(","+value+",", ",PREF,")) {
			return addressRankPreferred
		}
	}
	if typeParam(params) == vcardTypeWork {
		return addressRankWork
	}
	return addressRankOther
}

// postalAddressFromStructured reads ADR: pobox;ext;street;locality;region;code;country
// into the address columns. The post-office box and the extended address go to
// line 2, and so does a country no column can store, so no text is lost.
func postalAddressFromStructured(params []string, raw string) crmcontracts.Address {
	parts := splitVCardComponents(params, raw)
	part := func(i int) *string {
		if i < len(parts) && parts[i] != "" {
			return &parts[i]
		}
		return nil
	}
	out := crmcontracts.Address{Line1: part(2), City: part(3), Region: part(4), PostalCode: part(5)}
	var line2 []string
	for _, i := range []int{0, 1} {
		if p := part(i); p != nil {
			line2 = append(line2, *p)
		}
	}
	if country := part(6); country != nil {
		if code, named := addressCountry(*country); named {
			out.Country = &code
		} else {
			line2 = append(line2, *country)
		}
	}
	if len(line2) > 0 {
		out.Line2 = new(strings.Join(line2, ", "))
	}
	return out
}

// addressFromStructured reads ADR into one line, every component the card
// filled in, in the card's order.
func addressFromStructured(params []string, raw string) string {
	var kept []string
	for _, p := range splitVCardComponents(params, raw) {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			kept = append(kept, trimmed)
		}
	}
	return strings.Join(kept, ", ")
}
