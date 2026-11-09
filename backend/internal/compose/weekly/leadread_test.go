// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package weekly

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/weekly/learnings"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/mailcopy"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestLeadReadsRefuseBeforeReachingTheDatabase(t *testing.T) {
	ctx := seatReading("deal", "activity")
	start := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	if _, _, _, err := countWeekLeads(ctx, nil, ids.Nil, start, start.AddDate(0, 0, 7)); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("counts = %v, want permission denied", err)
	}
	if block, err := scoreLeads(ctx, nil, ids.Nil, start, start.AddDate(0, 0, 7)); err != nil || block != nil {
		t.Fatalf("scorecard = %+v, %v, want absent", block, err)
	}
	if _, err := hasLeads(ctx, nil, ids.Nil, start); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("presence = %v, want permission denied", err)
	}
}

func TestWithheldLeadFiguresLeaveTheRestOfTheWeekReadable(t *testing.T) {
	at := time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC)
	source := &crmcontracts.WeeklyNumericSummary{FigureCoverage: &crmcontracts.WeeklyFigureCoverageSet{Leads: &crmcontracts.WeeklyFigureCoverage{Status: crmcontracts.WeeklyFigureCoverageStatusRecorded, RecordedSince: &at}}}
	review := Review{
		Counts:         Counts{LeadsRouted: 19, LeadsAnsweredInTarget: 13, LeadsBreached: 6, DealsWon: 2, TasksDone: 3},
		NumericSummary: source,
		Scorecard:      &Scorecard{Lead: &LeadBlock{Advanced: 7}, Deal: &DealBlock{Advances: 4}},
		Narrative:      "Nineteen leads arrived.", NarratedAt: &at,
		LearningsState: LearningsSynthesized, Learnings: []learnings.Learning{{Kind: "leads", Text: "Six leads breached."}},
	}
	withholdReviewLeads(&review)
	wire := reviewToWire(review)
	if wire.LeadsWithheld == nil || !*wire.LeadsWithheld {
		t.Fatal("wire lost the withheld state")
	}
	if wire.Counts.LeadsRouted+wire.Counts.LeadsAnsweredInTarget+wire.Counts.LeadsBreached != 0 || wire.Scorecard.Lead != nil {
		t.Fatal("lead figures survived")
	}
	if wire.Counts.DealsWon != 2 || wire.Counts.TasksDone != 3 || review.Scorecard.Deal.Advances != 4 {
		t.Fatal("accessible work was removed")
	}
	if wire.Narrative != nil || wire.NarratedAt != nil || len(review.Learnings) != 0 || review.LearningsState != LearningsNotRun {
		t.Fatal("lead-derived prose survived")
	}
	if review.NumericSummary.FigureCoverage.Leads != nil {
		t.Fatal("lead history date survived")
	}
	if source.FigureCoverage.Leads == nil {
		t.Fatal("redaction mutated the source snapshot")
	}
	if NarrativeCovered(review) {
		t.Fatal("withheld lead facts reached narration")
	}
}

func TestTeamLeadRedactionKeepsOtherPatternsInTheFrozenLanguage(t *testing.T) {
	said := teamCopyFor("de")
	rep := said.repFrom(TeamMember{DisplayName: "Lena Falk"}, Counts{LeadsBreached: 7, DealsWon: 2}, 0, nil)
	keptLabel := ""
	for _, pattern := range rep.Patterns {
		if pattern.Kind != FocusLeadsBreached {
			keptLabel = pattern.Label
			break
		}
	}
	review := TeamReview{Language: "de", Counts: TeamCounts{LeadsRouted: 9, LeadsBreached: 7, DealsWon: 2}, Reps: []TeamRep{rep}}
	withheldFromReader(seatReading("forecast", "activity"), &review)
	wire := teamReviewToWire(review)
	if wire.LeadsWithheld == nil || !*wire.LeadsWithheld || wire.Counts.LeadsBreached != 0 {
		t.Fatal("team lead state was not withheld")
	}
	if review.Counts.DealsWon != 2 {
		t.Fatal("authorized team outcome was removed")
	}
	for _, pattern := range review.Reps[0].Patterns {
		if pattern.Kind == FocusLeadsBreached {
			t.Fatal("lead breach pattern survived")
		}
	}
	if keptLabel == "" || review.Reps[0].FocusLabel != keptLabel {
		t.Fatal("redaction changed the frozen pattern language")
	}
	if review.Reps[0].FocusKind == FocusLeadsBreached {
		t.Fatal("focus still names lead breaches")
	}
	if MayReadLeads(context.Background()) {
		t.Fatal("unbound reader was granted lead access")
	}
}

func TestWithheldLeadsPrintUnavailableInMailAndNeverReachNarration(t *testing.T) {
	covered := coverageOf(crmcontracts.WeeklyFigureCoverageStatusRecorded, nil)
	review := coveredFixture(crmcontracts.WeeklyFigureCoverageSet{Deals: covered, Tasks: covered, Meetings: covered, Leads: covered, Commitments: covered})
	if !NarrativeCovered(review) {
		t.Fatal("fixture does not support narration")
	}
	review.LeadsWithheld = true
	if NarrativeCovered(review) {
		t.Fatal("withheld lead placeholders reached narration")
	}
	words := mailcopy.For("en")
	var leadRow bool
	for _, fact := range mailFacts(review, words) {
		if fact.Label == words.WeeklyLeads {
			leadRow = true
			if fact.Value != words.WeeklyUnavailable {
				t.Fatalf("lead mail = %q", fact.Value)
			}
		}
		if fact.Label == words.WeeklyAnswered || fact.Label == words.WeeklyBreached {
			t.Fatal("redacted numeric lead rows reached mail")
		}
	}
	if !leadRow {
		t.Fatal("mail omitted the lead availability state")
	}
}
