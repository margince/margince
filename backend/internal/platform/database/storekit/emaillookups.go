// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"strings"

	"github.com/margince/margince/backend/internal/platform/mailidentity"
	"github.com/margince/margince/backend/internal/shared/kernel/correspondence"
)

// EmailLookups are the stored spellings a lookup of these addresses matches.
// That is each address folded, the lowercased form older rows and erasure
// hashes hold, and mailidentity.AddressSpellings of it. Postgres cannot fold
// IDNA, and a hash cannot be folded again after the fact.
func EmailLookups(emails ...string) []string {
	out := make([]string, 0, 2*len(emails))
	seen := map[string]bool{}
	for _, email := range emails {
		typed := correspondence.Fold(email)
		if typed == "" {
			continue
		}
		legacy := strings.ToLower(strings.TrimSpace(email))
		for _, spelled := range append([]string{typed, legacy}, mailidentity.AddressSpellings(email)...) {
			if !seen[spelled] {
				seen[spelled] = true
				out = append(out, spelled)
			}
		}
	}
	return out
}
