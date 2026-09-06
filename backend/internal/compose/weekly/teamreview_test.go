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

	kind, label := focusFor(counts, 1)

	if kind != FocusHelpRequested {
		t.Errorf("focus = %q, want %q — walking past a request to raise a metric "+
			"is the fastest way to teach a rep not to ask", kind, FocusHelpRequested)
	}
	if label == "" {
		t.Error("the focus carries no words")
	}
}

// Every rep gets a focus, including the one whose week went well. Without this
// a page promising one row per rep quietly shortens to the troubled ones.
// focusRules is one case per rule focusFor can fire, IN THE ORDER the switch
// tries them. Ordered and one-per-kind on purpose: the agenda's ranking is read
// off this sequence by TestTheAgendaOrderIsTheOrderTheRulesAreTried, so a case
// added out of order, or a second case for a kind, would quietly re-rank the
// Monday meeting.
//
// Each case supplies only what its own rule needs. One that supplies more would
// still reach its rule and still pass, while proving nothing about the rules
// above it.
var focusRules = []struct {
	name   string
	counts Counts
	help   int
	want   string
}{
	{"asked for help", Counts{}, 2, FocusHelpRequested},
	{"a lead went past", Counts{LeadsRouted: 3, LeadsBreached: 1}, 0, FocusLeadsBreached},
	{"missed commitments", Counts{CommitmentsDue: 3, CommitmentsKept: 1}, 0, FocusCommitmentsMissed},
	{
		"meetings with no next step",
		Counts{MeetingsHeld: 2, MeetingsWithNextStep: 1},
		0, FocusMeetingsWithoutNextStep,
	},
	{"won something", Counts{DealsWon: 2}, 0, FocusStrongWeek},
	{"nothing happened", Counts{}, 0, FocusQuietWeek},
}

func TestEveryWeekProducesAFocus(t *testing.T) {
	for _, tc := range focusRules {
		t.Run(tc.name, func(t *testing.T) {
			kind, label := focusFor(tc.counts, tc.help)
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
	kind, label := focusFor(Counts{CommitmentsDue: 3, CommitmentsKept: 3}, 0)
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
	_, label := focusFor(Counts{CommitmentsDue: 5, CommitmentsKept: 2}, 0)

	if label != "Kept 2 of 5 commitments" {
		t.Errorf("label = %q, want the two stored figures", label)
	}
}

// One is one, and two are two. A label reading "1 leads" is a small thing that
// makes a page look machine-written.
func TestOneReadsAsOne(t *testing.T) {
	if got := plural(1, "lead"); got != "1 lead" {
		t.Errorf("plural(1) = %q, want %q", got, "1 lead")
	}
	if got := plural(3, "lead"); got != "3 leads" {
		t.Errorf("plural(3) = %q, want %q", got, "3 leads")
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
