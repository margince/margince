// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package introductions

import (
	"strings"
	"testing"
)

func TestTooLongCountsCharactersNotBytes(t *testing.T) {
	if err := tooLong("reason", strings.Repeat("ế", 2000), 2000); err != nil {
		t.Errorf("2000 three-byte characters are within a 2000-character limit: %v", err)
	}
	err := tooLong("reason", strings.Repeat("a", 2001), 2000)
	if got := refusedField(t, err); got != "reason" {
		t.Errorf("refused on %q, want reason", got)
	}
}
