// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package salesprogress

import "testing"

func TestSaleCategoryReadsTheCapturedLineKind(t *testing.T) {
	for group, want := range map[string]string{
		"kind:software":       Product,
		"kind:implementation": Service,
		"kind:other":          Unresolved,
		"kind:unallocated":    Unresolved,
		"kind:invalid":        Unresolved,
	} {
		if got := Category(group); got != want {
			t.Errorf("Category(%q) = %q, want %q", group, got, want)
		}
	}
}
