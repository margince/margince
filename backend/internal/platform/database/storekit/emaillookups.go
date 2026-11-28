// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"github.com/margince/margince/backend/internal/platform/mailidentity"
	"github.com/margince/margince/backend/internal/shared/kernel/correspondence"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// EmailLookups are the stored spellings a lookup of these addresses matches.
// That is each address as typed and case folded, its correspondence.Fold
// spelling, and mailidentity.AddressSpellings of it. A row written before the
// fold dropped hidden characters keeps the older spelling, and so does a hash.
func EmailLookups(emails ...string) []string {
	out := make([]string, 0, 2*len(emails))
	seen := map[string]bool{}
	for _, email := range emails {
		typed := values.LegacyIdentifierKey(email)
		if typed == "" {
			continue
		}
		for _, spelled := range append([]string{typed, correspondence.Fold(email)}, mailidentity.AddressSpellings(email)...) {
			if !seen[spelled] {
				seen[spelled] = true
				out = append(out, spelled)
			}
		}
	}
	return out
}
