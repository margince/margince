// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatlinks

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
)

func TestAChannelIsNamedBySlugHashOrLink(t *testing.T) {
	for raw, want := range map[string]string{
		"acme-deal":  "acme-deal",
		"#Acme-Deal": "acme-deal",
		"https://chat.example.com/channels/acme/": "acme",
		"https://chat.example.com/c/acme?tab=1":   "acme",
		"  ":                                      "",
	} {
		if got := channelSlugOf(raw); got != want {
			t.Errorf("channelSlugOf(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestLinkingAnExistingChannelMustSayWhetherToSummarise(t *testing.T) {
	req := LinkRequest{Mode: ModeExisting, Channel: "acme"}
	if err := validateLink(&req); !errors.Is(err, apperrors.ErrInvalidArgument) {
		t.Fatalf("existing without summarize_history = %v, want a refusal", err)
	}
	yes := true
	req = LinkRequest{Mode: ModeExisting, Channel: "acme", SummarizeHistory: &yes}
	if err := validateLink(&req); err != nil || req.Verbosity != VerbosityImportant {
		t.Fatalf("existing with summarize_history = %v at %q, want accepted at important", err, req.Verbosity)
	}
	req = LinkRequest{Mode: ModeCreate, Channel: "ignored", SummarizeHistory: &yes}
	if err := validateLink(&req); err != nil || req.SummarizeHistory != nil || req.Channel != "" {
		t.Fatalf("create kept %+v, want no channel and no history question", req)
	}
	req = LinkRequest{Mode: ModeCreate, Verbosity: "loud"}
	if err := validateLink(&req); !errors.Is(err, apperrors.ErrInvalidArgument) {
		t.Fatalf("an unknown verbosity = %v, want a refusal", err)
	}
}

func TestEachLevelAdmitsItsTiers(t *testing.T) {
	want := map[string][]string{
		VerbosityEverything: {TierMilestone, TierImportant, TierEverything},
		VerbosityImportant:  {TierMilestone, TierImportant},
		VerbosityMilestones: {TierMilestone},
		VerbositySilent:     nil,
	}
	for level, tiers := range want {
		admitted := map[string]bool{}
		for _, tier := range tiers {
			admitted[tier] = true
		}
		for _, tier := range []string{TierMilestone, TierImportant, TierEverything} {
			if admits(level, tier) != admitted[tier] {
				t.Errorf("%s admits %s = %v, want %v", level, tier, admits(level, tier), admitted[tier])
			}
		}
	}
}
