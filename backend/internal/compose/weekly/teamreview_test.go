// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package weekly

// The coaching focus, over fixture counts.
//
// A unit test because the rule is arithmetic over one member's frozen week and
// nothing else — no database, no clock. What it defends is that EVERY rep gets
// a row, and that a request already made outranks any metric.

import (
	"testing"
	"time"
)

func TestARepWhoAskedForHelpIsRaisedFirst(t *testing.T) {
	// A week with something to raise on every other rung too, so this asserts
	// the ORDER rather than the only rule that could fire.
	counts := Counts{
		LeadsBreached: 3, CommitmentsDue: 4, CommitmentsKept: 1,
		MeetingsHeld: 2, MeetingsWithNextStep: 0,
	}

	kind, label := teamCopyFor("en").focusFor(counts, 1, &DealBlock{ForecastDown: 1})

	if kind != FocusHelpRequested {
		t.Errorf("focus = %q, want %q — walking past a request to raise a metric "+
			"is the fastest way to teach a rep not to ask", kind, FocusHelpRequested)
	}
	if label == "" {
		t.Error("the focus carries no words")
	}
}

// focusRules is one case per rule patternsFor can fire, in the order it tries
// them. TestTheAgendaOrderIsTheOrderTheRulesAreTried reads the agenda's ranking
// off this sequence, so a case added out of order re-ranks the Monday meeting.
//
// Each case supplies only what its own rule needs, so it fires alone.
var focusRules = []struct {
	name   string
	counts Counts
	help   int
	deals  *DealBlock
	want   string
}{
	{"asked for help", Counts{}, 2, nil, FocusHelpRequested},
	{"a lead went past", Counts{LeadsRouted: 3, LeadsBreached: 1}, 0, nil, FocusLeadsBreached},
	{"missed commitments", Counts{CommitmentsDue: 3, CommitmentsKept: 1}, 0, nil, FocusCommitmentsMissed},
	{"forecast down", Counts{}, 0, &DealBlock{ForecastDown: 1}, FocusForecastDowngrades},
	{"moved backwards", Counts{}, 0, &DealBlock{Regressions: 1}, FocusStageRegressions},
	{"provisional dates", Counts{}, 0, &DealBlock{Open: 3, WithNextStep: 3, CloseDateProvisional: new(2)}, FocusCloseDatesProvisional},
	{"no next steps", Counts{}, 0, &DealBlock{Open: 4, WithNextStep: 1}, FocusNoNextStepCoverage},
	{"won something", Counts{DealsWon: 2}, 0, nil, FocusStrongWeek},
	{
		"meetings with no next step",
		Counts{MeetingsHeld: 2, MeetingsWithNextStep: 1},
		0, nil, FocusMeetingsWithoutNextStep,
	},
	{"nothing happened", Counts{}, 0, nil, FocusQuietWeek},
}

func TestEveryWeekProducesAFocus(t *testing.T) {
	for _, tc := range focusRules {
		t.Run(tc.name, func(t *testing.T) {
			patterns := teamCopyFor("en").patternsFor(tc.counts, tc.help, tc.deals)
			if len(patterns) != 1 {
				t.Fatalf("the case fired %d rules, want only its own: %v", len(patterns), patterns)
			}
			kind, label := patterns[0].Kind, patterns[0].Label
			if kind != tc.want {
				t.Errorf("focus = %q, want %q", kind, tc.want)
			}
			if label == "" {
				t.Error("the focus carries no words — a row a lead cannot read is not a focus")
			}
		})
	}
}

// A week with nothing won is still a strong week when every commitment was
// kept. It is the second route to strong_week and so cannot sit in focusRules,
// which carries one case per kind because the agenda's order is read off it.
func TestKeepingEveryCommitmentIsAStrongWeek(t *testing.T) {
	kind, label := teamCopyFor("en").focusFor(Counts{CommitmentsDue: 3, CommitmentsKept: 3}, 0, nil)
	if kind != FocusStrongWeek {
		t.Errorf("focus = %q, want %q — a rep who kept all three has something to copy", kind, FocusStrongWeek)
	}
	if label == "" {
		t.Error("the focus carries no words — a row a lead cannot read is not a focus")
	}
}

// The label states a stored figure and nothing else, so it cannot say something
// the snapshot does not hold.
func TestTheFocusLabelNamesTheFiguresItRestsOn(t *testing.T) {
	_, label := teamCopyFor("en").focusFor(Counts{CommitmentsDue: 5, CommitmentsKept: 2}, 0, nil)

	if label != "Kept 2 of 5 commitments" {
		t.Errorf("label = %q, want the two stored figures", label)
	}
}

// One is one, and two are two. A label reading "1 leads" is a small thing that
// makes a page look machine-written.
func TestOneReadsAsOne(t *testing.T) {
	for _, tc := range []struct {
		count int
		want  string
	}{{1, "1 lead went past the response target"}, {3, "3 leads went past the response target"}} {
		_, got := teamCopyFor("en").focusFor(Counts{LeadsBreached: tc.count}, 0, nil)
		if got != tc.want {
			t.Errorf("%d leads: %q, want %q", tc.count, got, tc.want)
		}
	}
}

// THE TEAM'S LANDING REACHES THE WIRE, rendered by the SAME function the rep's
// outlook uses. A landing mapped to the wrong field, or not mapped at all,
// reads to the frontend as a team that forecasts nothing.
func TestTheTeamOutlookTravelsOnTheWire(t *testing.T) {
	review := TeamReview{
		TeamName: "Team One",
		Outlook: []Outlook{{
			PeriodKind:     "quarter",
			PeriodStart:    time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
			PeriodEnd:      time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC),
			BaseCurrency:   "EUR",
			WonMinor:       180_000_00,
			ForwardMeasure: "commit_evidence",
		}},
	}
	wire := teamReviewToWire(review)
	if wire.Outlook == nil || len(*wire.Outlook) != 1 {
		t.Fatal("a team week with a landing must carry it on the wire")
	}
	if (*wire.Outlook)[0].WonMinor != 180_000_00 {
		t.Fatalf("the figure must survive the mapping, got %d", (*wire.Outlook)[0].WonMinor)
	}
}

// A team week with NO landing sends no outlook at all, rather than an empty
// array: absent says "no forecast was composed", and a reader draws nothing
// instead of zeros.
func TestATeamWeekWithoutAForecastSendsNoOutlook(t *testing.T) {
	wire := teamReviewToWire(TeamReview{TeamName: "Team One"})
	if wire.Outlook != nil {
		t.Fatalf("no forecast means no outlook on the wire, got %d horizons", len(*wire.Outlook))
	}
}

func TestActivityWithoutACoachingSignalDoesNotMeanAQuietWeek(t *testing.T) {
	kind, label := teamCopyFor("en").focusFor(Counts{DealsMoved: 12, LeadsRouted: 8, LeadsAnsweredInTarget: 8}, 0, nil)
	if kind != FocusQuietWeek || label != "No priority indicated by the recorded metrics" {
		t.Fatalf("unsupported productivity verdict: %s / %s", kind, label)
	}
}

func TestRecordedSuccessOutranksMissingMeetingDocumentation(t *testing.T) {
	kind, _ := teamCopyFor("en").focusFor(Counts{DealsWon: 1, MeetingsHeld: 5}, 0, nil)
	if kind != FocusStrongWeek {
		t.Fatalf("won deal overshadowed by recording gaps: %s", kind)
	}
}
