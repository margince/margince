// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

import "testing"

// Only a query of one word of letters and digits may read own matches first;
// every other shape lets a contained-only row outscore one.
func TestOnlyOnePlainWordReadsOwnMatchesFirst(t *testing.T) {
	for query, want := range map[string]bool{
		"co": true, "Müller": true, "co ": true, "abc123": true, "Nguyễn": true,
		"cora alpha": false, "o'rei": false, "o’rei": false, "quinn@oz": false,
		"ozebra.example": false, "-co": false, "acme-co": false, "": false,
	} {
		if got := plainWord(query); got != want {
			t.Errorf("plainWord(%q) = %v, want %v", query, got, want)
		}
	}
}
