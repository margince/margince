// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package weekly

import (
	"strings"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/mailcopy"
)

func TestAnEmptyRecordedWeekDoesNotEarnAnEmail(t *testing.T) {
	if MailUseful(Review{}) || TeamMailUseful(TeamReview{Counts: TeamCounts{RepsCounted: 5}}) {
		t.Fatal("empty outcomes were treated as useful")
	}
	for _, counts := range []Counts{{TasksDone: 1}, {TasksCarriedOver: 1}, {LeadsAnsweredInTarget: 1}, {CommitmentsKept: 1}, {MeetingsHeld: 1}, {DealsWon: 1}} {
		if !MailUseful(Review{Counts: counts}) {
			t.Errorf("real outcome was suppressed: %+v", counts)
		}
	}
	if !TeamMailUseful(TeamReview{Reps: []TeamRep{{HelpRequested: 1}}}) {
		t.Fatal("request for help was suppressed")
	}
}

func TestHighlightsAreFactualAndHandleSingularAndPlural(t *testing.T) {
	for _, tc := range []struct {
		done, won int
		want      string
	}{{1, 1, "Du hast einen Deal gewonnen. Du hast eine Aufgabe erledigt."}, {4, 2, "Du hast 2 Deals gewonnen. Du hast 4 Aufgaben erledigt."}} {
		got := mailHighlights(Review{Counts: Counts{TasksCompleted: &tc.done, DealsWon: tc.won}}, mailcopy.For("de"))
		if got != tc.want {
			t.Errorf("highlights = %q, want %q", got, tc.want)
		}
	}
	done := 4
	review := Review{Counts: Counts{TasksCompleted: &done}, NumericSummary: &crmcontracts.WeeklyNumericSummary{FigureCoverage: &crmcontracts.WeeklyFigureCoverageSet{Tasks: &crmcontracts.WeeklyFigureCoverage{Status: "not_recorded"}}}}
	if got := mailHighlights(review, mailcopy.For("de")); strings.Contains(got, "4 Aufgaben erledigt") {
		t.Fatal("unrecorded source became an achievement")
	}
}

func TestAWeekOfLeadWorkAndMeetingsShowsThoseOutcomes(t *testing.T) {
	words := mailcopy.For("de")
	message := MailMessage(Review{Counts: Counts{LeadsAnsweredInTarget: 3, MeetingsHeld: 2}}, "", words)
	if !strings.Contains(message.Text, words.WeeklyAnswered+": 3") || !strings.Contains(message.Text, words.WeeklyMeetings+": 2") {
		t.Fatal("real lead and meeting work disappeared behind deal/task metrics")
	}
	if strings.Contains(message.Text, words.WeeklyDealsWon+": 0") || strings.Contains(message.Text, words.WeeklyCompleted+": 0") {
		t.Fatal("unrelated zero metrics dominate the report")
	}
}

func TestAFrozenEnglishFocusDoesNotLeakIntoGermanMail(t *testing.T) {
	got := TeamMailFocus(TeamRep{FocusKind: FocusLeadsBreached, FocusLabel: "First response overdue"}, "de")
	if got != "Antwortziel für Leads überschritten" {
		t.Fatalf("focus = %q", got)
	}
}

func TestReportingWeekBoundaryIncludesTheDSTChange(t *testing.T) {
	start, end, err := WeekWindow(time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC), "Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	if end.Sub(start) != 169*time.Hour || end.UTC() != time.Date(2026, 10, 25, 23, 0, 0, 0, time.UTC) {
		t.Fatalf("wrong reporting boundary: %v to %v", start, end)
	}
}
