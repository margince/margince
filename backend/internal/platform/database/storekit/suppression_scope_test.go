// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import "testing"

func TestChannelSuppressionAccountKeepsBareIDsAndSeparatesScopes(t *testing.T) {
	if got := ChannelSuppressionAccount("", "8100"); got != "8100" {
		t.Errorf("an unscoped id is %q, want the bare id", got)
	}
	seen := map[string]string{}
	for _, pair := range [][2]string{{"1", "23"}, {"12", "3"}, {"", "123"}, {"W", "B"}, {"W", ""}, {"", "W"}} {
		key := ChannelSuppressionAccount(pair[0], pair[1])
		if other, dup := seen[key]; dup {
			t.Errorf("(%q, %q) and %s share the key %q", pair[0], pair[1], other, key)
		}
		seen[key] = pair[0] + "|" + pair[1]
	}
}
