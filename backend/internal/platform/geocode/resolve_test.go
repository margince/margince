// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package geocode

import "testing"

func TestResolveReadsTheSettingOnce(t *testing.T) {
	for _, tc := range []struct {
		setting string
		baseURL string
		on      bool
	}{
		{"", PublicBaseURL, true},
		{"  ", PublicBaseURL, true},
		{"public", PublicBaseURL, true},
		{" public ", PublicBaseURL, true},
		{"off", "", false},
		{" off ", "", false},
		{"https://nominatim.internal", "https://nominatim.internal", true},
	} {
		baseURL, on := Resolve(tc.setting)
		if baseURL != tc.baseURL || on != tc.on {
			t.Errorf("Resolve(%q) = (%q, %v), want (%q, %v)", tc.setting, baseURL, on, tc.baseURL, tc.on)
		}
	}
}
