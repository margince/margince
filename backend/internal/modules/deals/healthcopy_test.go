// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy/langcopytest"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

func TestHealthReasonCatalog(t *testing.T) {
	langcopytest.Census(t, healthWords)
}

func TestHealthReasonsKeepFactorsAcrossLanguages(t *testing.T) {
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	last := now.AddDate(0, 0, -20)
	health := DealHealth{
		Factors:  DealHealthFactors{ActivityRecency: 0.1, StageVelocity: 0.2, Engagement: 0.3, Commitments: 0.4},
		Evidence: DealHealthEvidence{LastActivityAt: &last, Stalled: true, DaysInStage: 20, ExpectedDaysInStage: 10},
	}
	english := HealthReasons(health, now, time.UTC, "en")
	for _, language := range textlang.Shipped {
		t.Run(string(language), func(t *testing.T) {
			got := HealthReasons(health, now, time.UTC, string(language))
			if len(got) != len(english) {
				t.Fatal("language changed the factor count")
			}
			for i, reason := range got {
				if reason.Key != english[i].Key || reason.Value != english[i].Value {
					t.Fatal("language changed a health factor")
				}
				if reason.Reason == "" || language != textlang.English && reason.Reason == english[i].Reason {
					t.Fatalf("missing %s explanation for %s", language, reason.Key)
				}
			}
		})
	}
}

func TestGermanEngagementKeepsCountAndWindowDistinct(t *testing.T) {
	reason := engagementReason(3, langcopy.For("de"))
	if !strings.Contains(reason, "3 Beteiligten") || !strings.Contains(reason, "90 Tagen") {
		t.Fatalf("engagement swapped the count and observation window: %s", reason)
	}
}
