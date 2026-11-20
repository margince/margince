// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import "testing"

// FakeRoutingConfig must bind every ladder, or --ai-fake would silently
// refuse whichever task landed on the gap. TaskSiteExtract's ladder is
// premium-only: a config without premium looks fine for every other task
// and fails only deep-read extraction. UnboundLadderWarnings is the completeness check the boot
// path runs over the stored routing setting. Reusing it here holds the fake
// config to the same contract, not to a hand-kept list of tasks.
func TestFakeRoutingConfigBindsEveryLadder(t *testing.T) {
	cfg := FakeRoutingConfig()
	if err := cfg.validate(); err != nil {
		t.Fatalf("FakeRoutingConfig() failed its own validation: %v", err)
	}
	if got := cfg.UnboundLadderWarnings(); len(got) != 0 {
		t.Fatalf("FakeRoutingConfig() leaves ladders unbound: %v", got)
	}
	if cfg.Profile != ProfileEUHosted {
		t.Fatalf("Profile = %q, want %q", cfg.Profile, ProfileEUHosted)
	}
	for tier, binding := range cfg.Tiers {
		if got := binding.Provider; got != ProviderFake {
			t.Fatalf("tier %s provider = %q, want %q", tier, got, ProviderFake)
		}
	}
	if got := cfg.Embeddings.Provider; got != ProviderFake {
		t.Fatalf("embeddings provider = %q, want %q", got, ProviderFake)
	}
}
