// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package messagingrules

import "testing"

// Advertising is opt-out in a send only where every pack that binds it says so.
func TestAFoldIsOptOutOnlyWhereEveryPackIs(t *testing.T) {
	out := Rules{MarketingMode: MarketingOptOut}
	in := Rules{MarketingMode: MarketingOptIn}
	silent := Rules{}
	for name, tc := range map[string]struct {
		a, b Rules
		want bool
	}{
		"both opt out":           {out, out, true},
		"one opts in":            {out, in, false},
		"the other opts in":      {in, out, false},
		"one says nothing":       {out, silent, false},
		"neither says anything":  {silent, silent, false},
		"both opt in explicitly": {in, in, false},
	} {
		if got := stricter(tc.a, tc.b).MarketingMode.OptOut(); got != tc.want {
			t.Errorf("%s: opt-out = %v, want %v", name, got, tc.want)
		}
	}
}

// A layer may tighten opt-out to opt-in and may not loosen opt-in to opt-out.
// Stating opt-in over a silent layer changes nothing.
func TestALayerCannotLoosenOptInToOptOut(t *testing.T) {
	out := Rules{MarketingMode: MarketingOptOut}
	in := Rules{MarketingMode: MarketingOptIn}
	if got := Loosened(in, out); len(got) != 1 || got[0] != "MarketingMode" {
		t.Errorf("opt-out over opt-in loosened %v, want MarketingMode", got)
	}
	if got := Loosened(Rules{}, out); len(got) != 1 {
		t.Errorf("opt-out over a silent layer loosened %v, want MarketingMode", got)
	}
	if got := Loosened(out, in); len(got) != 0 {
		t.Errorf("opt-in over opt-out loosened %v, want nothing", got)
	}
	if got := Loosened(Rules{}, in); len(got) != 0 {
		t.Errorf("stating opt-in over a silent layer loosened %v, want nothing", got)
	}
}
