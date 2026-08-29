// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"testing"

	"github.com/margince/margince/backend/internal/modules/ai"
)

// A dev stack seeds a cloud binding at bootstrap, and the engineer running it
// may hold no key for that vendor. The worker used to take the stored binding
// as its only candidate and exit at boot when it could not be built — while
// the api, on the same machine and the same flag, fell back and served. The
// stack then looked healthy and ran no queued job at all, which reads as a
// broken feature rather than an unconfigured one.
func TestAnUnservableBindingFallsBackToTheFakeOnlyWhenItWasAskedFor(t *testing.T) {
	bound := ai.FakeRoutingConfig()
	if bound.Unconfigured() {
		t.Fatal("FakeRoutingConfig is unconfigured, so it cannot stand in for a stored binding here")
	}

	for _, tc := range []struct {
		name  string
		cfg   ai.RoutingConfig
		fake  bool
		want  int
		first string
	}{
		{
			name:  "a binding plus --ai-fake keeps the fake as a fallback behind it",
			cfg:   bound,
			fake:  true,
			want:  2,
			first: "the stored binding",
		},
		{
			name:  "a binding without --ai-fake has no fallback, so a deployment fails closed",
			cfg:   bound,
			fake:  false,
			want:  1,
			first: "the stored binding",
		},
		{
			name:  "no binding but --ai-fake runs the fake outright",
			cfg:   ai.RoutingConfig{},
			fake:  true,
			want:  1,
			first: "the fake",
		},
		{
			name: "neither leaves nothing to run, and nothing is picked silently",
			cfg:  ai.RoutingConfig{},
			fake: false,
			want: 0,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := modelPathCandidates(tc.cfg, tc.fake)
			if len(got) != tc.want {
				t.Fatalf("modelPathCandidates gave %d candidate(s), want %d", len(got), tc.want)
			}
			if tc.want == 0 {
				return
			}
			// The ORDER carries the rule: the stored binding is tried first and
			// the fake is only ever the last resort, so a bound installation
			// never quietly serves canned text while its own binding works.
			if tc.first == "the stored binding" && got[0].Unconfigured() {
				t.Error("the first candidate is not the stored binding, so a bound installation would serve the fake")
			}
			if tc.want == 2 && len(got) == 2 && got[1].Unconfigured() {
				t.Error("the fallback candidate is unconfigured, so the fallback cannot serve anything")
			}
		})
	}
}
