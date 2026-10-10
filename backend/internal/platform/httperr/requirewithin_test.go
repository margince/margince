// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httperr

import (
	"strings"
	"testing"
)

func TestRequireWithinCountsCharactersNotBytes(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		text    string
		refused bool
	}{
		"at the cap":            {strings.Repeat("a", 5), false},
		"one past the cap":      {strings.Repeat("a", 6), true},
		"umlauts at the cap":    {strings.Repeat("ü", 5), false},
		"umlauts past the cap":  {strings.Repeat("ü", 6), true},
		"empty is not too long": {"", false},
	} {
		err := RequireWithin("note", tc.text, 5)
		if (err != nil) != tc.refused {
			t.Errorf("%s: RequireWithin = %v, want refused=%v", name, err, tc.refused)
		}
	}
}
