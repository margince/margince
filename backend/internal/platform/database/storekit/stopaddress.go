// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import "strings"

// StopOnAddressesSQL is true when the communication_suppression row aliased
// alias stops one of a set of addresses, by its plain address or its hash.
// lowered and hashes are the text[] parameters StopAddressArgs answers. The
// send check, the refusal review, the stop list and the Art. 15 export all ask
// through it, so none misses a stop that holds only a hash.
func StopOnAddressesSQL(alias, lowered, hashes string) string {
	return "((" + alias + ".address IS NOT NULL AND lower(" + alias + ".address) = ANY(" + lowered + "::text[]))" +
		" OR (" + alias + ".address_hash IS NOT NULL AND " + alias + ".address_hash = ANY(" + hashes + "::text[])))"
}

// StopAddressArgs answers the parameters of StopOnAddressesSQL. They are each
// address lowercased, and SuppressionHash of each spelling EmailLookups knows,
// the hash an anonymize writes. An empty address matches nothing.
func StopAddressArgs(addresses ...string) (lowered, hashes []string) {
	lowered = make([]string, 0, len(addresses))
	for _, address := range addresses {
		if trimmed := strings.ToLower(strings.TrimSpace(address)); trimmed != "" {
			lowered = append(lowered, trimmed)
		}
	}
	spellings := EmailLookups(addresses...)
	hashes = make([]string, 0, len(spellings))
	for _, spelled := range spellings {
		hashes = append(hashes, SuppressionHash(spelled))
	}
	return lowered, hashes
}
