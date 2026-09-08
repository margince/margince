// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package briefs

import "testing"

// The night says WHY it picked a deal, and the answer is read off the same
// vector the composite folded.
//
// Every entry used to reach the queue as "at risk" whatever the ranking found.
// The formula selects on winnability, value, timing, momentum and warmth, so an
// attractive deal clears the bar on its merits — and a rep told twice that a
// healthy deal is drifting stops believing the label on the one that is.
func TestTheSignalNamesWhyTheNightPickedTheDeal(t *testing.T) {
	cases := []struct {
		name string
		in   BriefFeatureVector
		want BriefSignal
	}{
		// A date that has nearly arrived is work today whatever else is true —
		// including on a deal that is also silent, where filing it as stalled
		// would put urgent work on a watchlist.
		{"a date about to arrive", BriefFeatureVector{Timing: 0.9}, SignalClosingSoon},
		{"about to close and silent too", BriefFeatureVector{
			Timing: 0.9, Momentum: briefMomentumUnchanged,
		}, SignalClosingSoon},
		// Silence is worth naming only when the relationship is cold as well: a
		// warm deal between conversations is not stalling.
		{"quiet and cold", BriefFeatureVector{
			Momentum: briefMomentumUnchanged, Warmth: 0.1,
		}, SignalStalled},
		{"quiet but warm is not stalling", BriefFeatureVector{
			Momentum: briefMomentumUnchanged, Warmth: 0.9, Winnability: 0.8,
		}, SignalOpportunity},
		{"something happened on it", BriefFeatureVector{
			Momentum: briefMomentumChanged, Warmth: 0.2,
		}, SignalMoved},
		// THE CORRECTION: winnable and worth money, nothing wrong. The case the
		// old blanket label was flatly untrue about.
		{"winnable and worth money", BriefFeatureVector{
			Winnability: 0.9, Revenue: 0.9, Warmth: 0.8,
		}, SignalOpportunity},
	}
	for _, c := range cases {
		if got := SignalOf(c.in); got != c.want {
			t.Errorf("%s: signal = %q, want %q", c.name, got, c.want)
		}
	}
}
