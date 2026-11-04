// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import "testing"

func TestAStampIsAUsersOnlyForTheirSessionOrTheirPassport(t *testing.T) {
	for stamp, want := range map[string]bool{
		"human:019a0b1c-0000-7000-8000-000000000001": true,
		"agent:019a0b1c-0000-7000-8000-000000000001": true,
		"agent:enrich":     false,
		"agent:deal-scout": false,
		"connector:mail":   false,
		"system":           false,
		"":                 false,
	} {
		if got := StampIsForAUser(stamp); got != want {
			t.Errorf("StampIsForAUser(%q) = %t, want %t", stamp, got, want)
		}
	}
}
