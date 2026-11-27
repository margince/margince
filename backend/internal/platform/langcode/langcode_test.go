// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package langcode

import "testing"

func TestKnownTakesAssignedTwoLetterCodesOnly(t *testing.T) {
	for code, want := range map[string]bool{
		"de": true, "en": true, "vi": true, "fr": true, "ja": true,
		"zz": false, "xx": false, "qa": false,
		"DE": false, "deu": false, "d1": false, "": false, "und": false,
	} {
		if got := Known(code); got != want {
			t.Errorf("Known(%q) = %v, want %v", code, got, want)
		}
	}
}
