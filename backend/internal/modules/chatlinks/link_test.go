// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatlinks

import (
	"errors"
	"fmt"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
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

func TestANewChannelInvitesTheOwnerAndTheLinkerFirstAndCountsWhoIsOverTheCap(t *testing.T) {
	owner, linker := ids.NewV7(), ids.NewV7()
	var viewers []Viewer
	for i := range inviteCap + 3 {
		viewers = append(viewers, Viewer{ID: ids.NewV7(), Email: fmt.Sprintf("m%d@chat.test", i)})
	}
	viewers = append(viewers, Viewer{ID: linker, Email: "Linker@chat.test"}, Viewer{ID: owner, Email: "owner@chat.test"})

	emails, skipped := invitees(viewers, &owner, linker)
	if len(emails) != inviteCap || skipped != 5 {
		t.Fatalf("invited %d and skipped %d, want %d and 5", len(emails), skipped, inviteCap)
	}
	if emails[0] != "owner@chat.test" || emails[1] != "linker@chat.test" || emails[2] != "m0@chat.test" {
		t.Errorf("first invited = %v, want the owner, the linker, then the rest in order", emails[:3])
	}

	emails, skipped = invitees([]Viewer{{ID: linker, Email: "linker@chat.test"}}, nil, linker)
	if len(emails) != 1 || skipped != 0 {
		t.Errorf("a record with no owner invited %v and skipped %d, want the linker alone", emails, skipped)
	}
}
