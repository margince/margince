// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The auto-enrich daily cap paces how many companies a backfill day can mint.
// Sizing it wrong in either direction is visible only days later — people
// without companies, or a crawler burst — so the resolution rules are pinned
// here: absent and zero take the compiled default, a positive integer replaces
// it, and anything else refuses rather than silently pacing at the default.

import (
	"testing"

	"github.com/margince/margince/backend/internal/platform/config"
)

func TestAutoEnrichDailyCapResolution(t *testing.T) {
	cases := []struct {
		name string
		env  string
		want int
	}{
		{name: "unset takes the compiled default", env: "", want: defaultAutoEnrichDailyCap},
		{name: "blank is unset", env: "   ", want: defaultAutoEnrichDailyCap},
		{name: "zero takes the compiled default", env: "0", want: defaultAutoEnrichDailyCap},
		{name: "a positive integer replaces it", env: "2000", want: 2000},
		{name: "a cap below the default also binds", env: "10", want: 10},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := AutoEnrichDailyCapFromEnv(config.Static(map[string]string{AutoEnrichDailyCapEnv: tc.env}))
			if err != nil {
				t.Fatalf("%s=%q refused: %v", AutoEnrichDailyCapEnv, tc.env, err)
			}
			if got != tc.want {
				t.Errorf("%s=%q resolved to %d, want %d", AutoEnrichDailyCapEnv, tc.env, got, tc.want)
			}
		})
	}
}

func TestAnUnreadableAutoEnrichDailyCapRefusesTheBoot(t *testing.T) {
	for _, v := range []string{"fast", "-1", "1.5", "500x"} {
		if _, err := AutoEnrichDailyCapFromEnv(config.Static(map[string]string{AutoEnrichDailyCapEnv: v})); err == nil {
			t.Errorf("%s=%q resolved without error — a typo would silently pace at the compiled default", AutoEnrichDailyCapEnv, v)
		}
	}
}
