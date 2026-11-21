// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestARoutingRuleOnACompanyKeyMatchesBothSpellings(t *testing.T) {
	owner := ids.New[ids.UserKind]()
	cfg := RoutingConfig{
		Owners: []ids.UserID{ids.New[ids.UserKind]()},
		Rules:  []RoutingRule{{Field: candidateCompanyKey, Equals: "bücher.example", OwnerID: owner}},
	}
	active := map[ids.UserID]bool{owner: true, cfg.Owners[0]: true}
	for _, key := range []string{"bücher.example", "xn--bcher-kva.example", "WWW.Bücher.example"} {
		got, reason, ok := chooseOwner(cfg, leadRoutingFacts{CandidateCompanyKey: key}, nil, active)
		if !ok || got != owner || reason != "rule:0:candidate_company_key" {
			t.Errorf("key %q routed to %v (%s), want the rule owner", key, got, reason)
		}
	}
	if got, reason, _ := chooseOwner(cfg, leadRoutingFacts{CandidateCompanyKey: "other.example"}, nil, active); got == owner {
		t.Errorf("another key routed by the rule (%s)", reason)
	}
}

func TestACompanyKeySlugIsLeftAsTyped(t *testing.T) {
	slug := "Acme Inc"
	if got := foldedCompanyKey(&slug); got == nil || *got != "Acme Inc" {
		t.Errorf("a slug was changed to %v", got)
	}
	domain := "https://www.Bücher.example/x"
	if got := foldedCompanyKey(&domain); got == nil || *got != "xn--bcher-kva.example" {
		t.Errorf("a domain-shaped key was stored as %v", got)
	}
}
