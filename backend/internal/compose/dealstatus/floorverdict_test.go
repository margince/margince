// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealstatus

import (
	"strings"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func healthyFacts() facts {
	last := testNow.AddDate(0, 0, -2)
	return facts{
		deal: openDeal(), now: testNow, loc: time.UTC, lang: "en",
		health: &deals.DealHealth{
			Health:   0.8,
			Factors:  deals.DealHealthFactors{ActivityRecency: 0.9, StageVelocity: 0.8, Engagement: 0.7, Commitments: 1},
			Evidence: deals.DealHealthEvidence{LastActivityAt: &last},
		},
	}
}

func floorCall(t *testing.T, f facts) *crmcontracts.DealStatusCardVerdict {
	t.Helper()
	card := composeDeterministic(f, moveNone())
	if card.Verdict == nil {
		return nil
	}
	if len(card.Verdict.Because.Sentences) != 1 || card.Verdict.Because.Sentences[0].Text == "" {
		t.Fatalf("the floor's call rests on %+v, want one sentence", card.Verdict.Because.Sentences)
	}
	return card.Verdict
}

func TestAHealthyOpenDealIsCalledLiveOnTheRecordsAlone(t *testing.T) {
	got := floorCall(t, healthyFacts())
	if got == nil || got.Standing != "live" {
		t.Fatalf("verdict = %+v, want live", got)
	}
	if !strings.Contains(got.Because.Sentences[0].Text, "2 days ago") {
		t.Fatalf("because = %q, want the last activity", got.Because.Sentences[0].Text)
	}
}

func TestAnAtRiskDealIsDriftingAndSaysWhichFactorPullsItDown(t *testing.T) {
	f := healthyFacts()
	f.health.AtRisk = true
	f.health.Factors.Commitments = 0.1
	f.health.Evidence.OverdueTaskIDs = make([]ids.UUID, 2)
	got := floorCall(t, f)
	if got == nil || got.Standing != "drifting" {
		t.Fatalf("verdict = %+v, want drifting", got)
	}
	if got.Because.Sentences[0].Text != "2 tasks are overdue." {
		t.Fatalf("because = %q, want the overdue tasks", got.Because.Sentences[0].Text)
	}
}

func TestAStalledDealIsNeverCalledLive(t *testing.T) {
	f := healthyFacts()
	f.health.Evidence.Stalled = true
	if got := floorCall(t, f); got == nil || got.Standing != "drifting" {
		t.Fatalf("verdict = %+v, want drifting", got)
	}
}

func TestADealWithNoActivityIsNeverCalledLive(t *testing.T) {
	f := healthyFacts()
	f.health.Evidence.LastActivityAt = nil
	if got := floorCall(t, f); got == nil || got.Standing != "drifting" {
		t.Fatalf("verdict = %+v, want drifting", got)
	}
}

func TestAClosedDealGetsNoCallFromTheFloor(t *testing.T) {
	f := healthyFacts()
	f.deal.Status = crmcontracts.DealStatusWon
	if got := floorCall(t, f); got != nil {
		t.Fatalf("a won deal was called %+v; its outcome is the call", got)
	}
}
