// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"testing"

	"github.com/margince/margince/backend/internal/platform/config"
)

// laneNames is the lanes reported, in the order the read returns them.
func laneNames(t *testing.T, cfg RoutingConfig, keys map[string]string) []string {
	t.Helper()
	var names []string
	for _, lane := range lanesNeedingAKey(cfg, config.Static(keys)) {
		names = append(names, lane.Lane)
	}
	return names
}

func TestALaneWhoseVendorHoldsNoKeyIsReportedWithTheVariableToSet(t *testing.T) {
	cfg := RoutingConfig{
		Profile: ProfileCloudFrontier,
		Tiers:   map[Tier]ProviderConfig{TierLocalSmall: {Provider: providerAnthropic, Model: "claude-x"}},
	}

	lanes := lanesNeedingAKey(cfg, config.Static(nil))

	if len(lanes) != 1 {
		t.Fatalf("lanes = %+v, want the one tier that cannot run", lanes)
	}
	if got := lanes[0]; got.Lane != string(TierLocalSmall) || got.Provider != providerAnthropic {
		t.Errorf("lane = %s/%s, want %s/%s", got.Lane, got.Provider, TierLocalSmall, providerAnthropic)
	}
	// The variable rather than prose, because an operator acts on it.
	if got := lanes[0].KeyEnv; got != cloudKeyEnv[providerAnthropic] {
		t.Errorf("key_env = %q, want %q", got, cloudKeyEnv[providerAnthropic])
	}
	if got := lanes[0].Model; got == nil || *got != "claude-x" {
		t.Errorf("model = %v, want the model that was saved", got)
	}
}

func TestTheKeyedAndTheKeylessVendorAreToldApart(t *testing.T) {
	cfg := RoutingConfig{
		Profile: ProfileCloudFrontier,
		Tiers: map[Tier]ProviderConfig{
			TierLocalSmall: {Provider: providerAnthropic, Model: "claude-x"},
			TierPremium:    {Provider: ProviderFake, Model: "fake"},
		},
	}

	// The key is held: nothing to report, and the fake never needed one.
	if names := laneNames(t, cfg, map[string]string{cloudKeyEnv[providerAnthropic]: "sk-held"}); names != nil {
		t.Errorf("lanes = %v, want none once the key is held", names)
	}
	if names := laneNames(t, cfg, nil); len(names) != 1 || names[0] != string(TierLocalSmall) {
		t.Errorf("lanes = %v, want only the tier whose vendor takes a key", names)
	}
}

// The decisions lane is the one the bug was reported on. Its two vendors
// differ: jev demands a key, jev_compatible sends one only when set.
func TestOnlyTheDecisionVendorThatDemandsAKeyIsReported(t *testing.T) {
	cases := []struct {
		provider string
		want     bool
	}{
		{providerJev, true},
		{providerJevCompatible, false},
	}
	for _, c := range cases {
		t.Run(c.provider, func(t *testing.T) {
			cfg := RoutingConfig{
				Profile:   ProfileCloudFrontier,
				Decisions: &DecisionsConfig{Provider: c.provider, Model: "jev-1.13.0", BaseURL: "https://decide.example/v1/one"},
			}

			names := laneNames(t, cfg, nil)

			if got := len(names) == 1 && names[0] == string(TierDecideLane); got != c.want {
				t.Errorf("reported = %v (%v), want %v", got, names, c.want)
			}
		})
	}
}

func TestAConfigNoReaderMaySeeReportsNoLane(t *testing.T) {
	// The zero config is what a caller without routing read is given, so this
	// read must not describe an installation it may not see.
	if names := laneNames(t, RoutingConfig{}, nil); names != nil {
		t.Errorf("lanes = %v, want none for a config that was never read", names)
	}
}
