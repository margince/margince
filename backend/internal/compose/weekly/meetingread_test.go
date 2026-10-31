// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package weekly

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/weekly/learnings"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// seatReading is a full seat holding read on each object named.
func seatReading(objects ...string) context.Context {
	grants := map[string]principal.ObjectGrant{}
	for _, object := range objects {
		grants[object] = principal.ObjectGrant{Read: true}
	}
	return principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:lead", UserID: ids.NewV7(), SeatType: principal.SeatFull,
		Permissions: principal.Permissions{RowScope: principal.RowScopeAll, Objects: grants},
	})
}

func repWeekWithMeetings() Review {
	narrated := time.Date(2026, 10, 26, 8, 0, 0, 0, time.UTC)
	return Review{
		Counts: Counts{DealsWon: 2, MeetingsHeld: 5, MeetingsWithNextStep: 3},
		Scorecard: &Scorecard{Lead: &LeadBlock{
			Advanced: 4, Booked: 6, Held: 5, NoShow: 1, PartialHistory: 2,
		}},
		Narrative:      "Five meetings held, three with a next step.",
		NarratedAt:     &narrated,
		LearningsState: LearningsSynthesized,
		Learnings:      []learnings.Learning{{Kind: "meetings", Text: "Two meetings left no next step."}},
	}
}

func TestARepWeekWithoutActivityReadCarriesNoMeetingFigure(t *testing.T) {
	review := repWeekWithMeetings()
	withholdReviewMeetings(&review)
	wire := reviewToWire(review)
	if wire.MeetingsWithheld == nil || !*wire.MeetingsWithheld {
		t.Error("the review does not say its meeting figures are withheld")
	}
	if wire.Counts.MeetingsHeld != 0 || wire.Counts.MeetingsWithNextStep != 0 {
		t.Errorf("the counts carry meetings %d and %d, want both zero",
			wire.Counts.MeetingsHeld, wire.Counts.MeetingsWithNextStep)
	}
	lead := wire.Scorecard.Lead
	if lead.MeetingsBooked+lead.MeetingsHeld+lead.MeetingsNoShow+lead.MeetingsPartialHistory != 0 {
		t.Errorf("the scorecard's lead block carries a meeting figure: %+v", lead)
	}
	if wire.Narrative != nil || wire.NarratedAt != nil {
		t.Errorf("the narrative a model wrote from the meetings travels: %v", wire.Narrative)
	}
	if review.LearningsState != LearningsNotRun || len(review.Learnings) != 0 {
		t.Errorf("learnings %q with %d items travel, want not_run and none", review.LearningsState, len(review.Learnings))
	}
	if wire.Counts.DealsWon != 2 || lead.Advanced != 4 {
		t.Errorf("a figure that is no meeting went too: deals won %d, advanced %d", wire.Counts.DealsWon, lead.Advanced)
	}
}

func TestATeamWeekWithoutActivityReadCarriesNoMeetingFigureOrPattern(t *testing.T) {
	onlyMeetings := summaryRep("Ada", 0, FocusMeetingsWithoutNextStep)
	onlyMeetings.MeetingsHeld = 4
	alsoBreaches := summaryRep("Ben", 0, FocusLeadsBreached, FocusMeetingsWithoutNextStep)
	review := TeamReview{
		Language: "en",
		Counts:   TeamCounts{DealsWon: 1, MeetingsHeld: 4, MeetingsWithNextStep: 1},
		Reps:     []TeamRep{onlyMeetings, alsoBreaches},
	}
	withholdTeamMeetings(&review)
	wire := teamReviewToWire(review)
	if wire.MeetingsWithheld == nil || !*wire.MeetingsWithheld {
		t.Error("the team week does not say its meeting figures are withheld")
	}
	if wire.Counts.MeetingsHeld != 0 || wire.Counts.MeetingsWithNextStep != 0 || wire.Counts.DealsWon != 1 {
		t.Errorf("team counts = %+v, want the meetings zero and deals won kept", wire.Counts)
	}
	for _, rep := range wire.Reps {
		if rep.MeetingsHeld != 0 {
			t.Errorf("%s carries %d meetings, want none", rep.DisplayName, rep.MeetingsHeld)
		}
		for _, p := range rep.Patterns {
			if string(p.Kind) == FocusMeetingsWithoutNextStep {
				t.Errorf("%s still carries the meetings pattern", rep.DisplayName)
			}
		}
	}
	if got := review.Reps[0]; got.FocusKind != FocusQuietWeek || got.FocusLabel == "" {
		t.Errorf("a rep whose only pattern was meetings focuses on %q %q, want a quiet week", got.FocusKind, got.FocusLabel)
	}
	if got := review.Reps[1].FocusKind; got != FocusLeadsBreached {
		t.Errorf("a rep with breaches as well focuses on %q, want leads breached", got)
	}
}

func TestOnlyASeatWithoutActivityReadLosesTheMeetings(t *testing.T) {
	week := func() TeamReview {
		return TeamReview{Language: "en", Counts: TeamCounts{MeetingsHeld: 3},
			Reps: []TeamRep{summaryRep("Ada", 0, FocusMeetingsWithoutNextStep)}}
	}
	reader := week()
	withheldFromReader(seatReading("deal", "forecast", "activity"), &reader)
	if reader.MeetingsWithheld || reader.Counts.MeetingsHeld != 3 {
		t.Errorf("a seat with activity read lost the meetings: withheld %v, held %d", reader.MeetingsWithheld, reader.Counts.MeetingsHeld)
	}
	refused := week()
	withheldFromReader(seatReading("deal", "forecast"), &refused)
	if !refused.MeetingsWithheld || refused.Counts.MeetingsHeld != 0 {
		t.Errorf("a seat without activity read kept the meetings: withheld %v, held %d", refused.MeetingsWithheld, refused.Counts.MeetingsHeld)
	}
}
