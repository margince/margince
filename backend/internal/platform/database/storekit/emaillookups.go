// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"strings"

	"github.com/margince/margince/backend/internal/platform/mailidentity"
)

// EmailLookups are the stored spellings a lookup of these addresses matches.
// That is each address as typed and case folded, plus
// mailidentity.AddressSpellings of it. Postgres cannot fold IDNA, and a row
// stores the address as it was typed.
func EmailLookups(emails ...string) []string {
	out := make([]string, 0, 2*len(emails))
	seen := map[string]bool{}
	for _, email := range emails {
		typed := strings.ToLower(strings.TrimSpace(email))
		if typed == "" {
			continue
		}
		for _, spelled := range append([]string{typed}, mailidentity.AddressSpellings(email)...) {
			if !seen[spelled] {
				seen[spelled] = true
				out = append(out, spelled)
			}
		}
	}
	return out
}
