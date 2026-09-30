// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/weekly"
	"github.com/margince/margince/backend/internal/compose/weekly/learnings"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The learnings prompt reads the review's FROZEN figures: every tally it is
// handed is the stored one, and every deal it may cite carries the id and label
// the review shows, so a citation names a row the reader can open.
func TestTheLearningsPromptReadsTheReviewsFrozenWeek(t *testing.T) {
	won, lost := ids.NewV7(), ids.NewV7()
	review := weekly.Review{
		LocalWeekStart: time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC),
		Counts: weekly.Counts{
			TasksDue: 9, TasksDone: 7, TasksCarriedOver: 2, DealsMoved: 5, DealsWon: 1, DealsLost: 1,
			CommitmentsDue: 4, CommitmentsKept: 3, MeetingsHeld: 8, LeadsRouted: 6,
		},
		Deals: []weekly.DealLine{
			{DealID: won, Label: "Voltaq renewal", Outcome: "won", ToStageLabel: "Closed won"},
			{DealID: lost, Label: "Nordwerk pilot", Outcome: "lost"},
		},
	}

	got := promptedWeek(t, learningsInput(review))

	want := promptWeek{
		WeekStart: "2026-09-14",
		Counts: learnings.Counts{
			TasksDue: 9, TasksDone: 7, TasksCarriedOver: 2, DealsMoved: 5, DealsWon: 1, DealsLost: 1,
			CommitmentsDue: 4, CommitmentsKept: 3, MeetingsHeld: 8, LeadsRouted: 6,
		},
		Deals: []promptDeal{
			{Type: learnings.SubjectDeal, ID: won, Label: "Voltaq renewal", Outcome: "won"},
			{Type: learnings.SubjectDeal, ID: lost, Label: "Nordwerk pilot", Outcome: "lost"},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("the prompt reads\n%+v\nwant the review's frozen week\n%+v", got, want)
	}
}

// A week with no deal lines still renders an empty list, never null, so the
// prompt says "no deals" rather than "no field".
func TestAWeekWithoutDealsPromptsAnEmptyList(t *testing.T) {
	got := promptedWeek(t, learningsInput(weekly.Review{LocalWeekStart: time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)}))
	if got.Deals == nil || len(got.Deals) != 0 {
		t.Errorf("deals = %#v, want an empty list", got.Deals)
	}
}

type promptWeek struct {
	WeekStart string           `json:"week_start"`
	Counts    learnings.Counts `json:"counts"`
	Deals     []promptDeal     `json:"deals"`
}

type promptDeal struct {
	Type    string   `json:"type"`
	ID      ids.UUID `json:"id"`
	Label   string   `json:"label"`
	Outcome string   `json:"outcome,omitempty"`
}

// promptedWeek decodes the week out of the fenced user turn the request sends.
func promptedWeek(t *testing.T, in learnings.Input) promptWeek {
	t.Helper()
	req := learnings.Request(in, "en")
	if len(req.Messages) != 1 {
		t.Fatalf("the request carries %d messages, want the one fenced week", len(req.Messages))
	}
	content := req.Messages[0].Content
	start := strings.Index(content, `{"week_start"`)
	if start < 0 {
		t.Fatalf("the user turn carries no week: %q", content)
	}
	var week promptWeek
	if err := json.NewDecoder(strings.NewReader(content[start:])).Decode(&week); err != nil {
		t.Fatalf("decode the prompted week: %v", err)
	}
	return week
}
