// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package weekly

import (
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// TestTheAgendaOrderIsTheOrderTheRulesAreTried is what stops the agenda's
// ranking becoming a second, quietly different copy of focusFor's rule order.
//
// The two are written apart because a switch cannot be asked for an order and a
// list cannot pick a focus, so the only thing holding them together is this: it
// drives focusFor through focusRules — one case per rule, in the order the
// switch tries them — and asks whether what came back IS focusPriority. A rule
// that moved, or a rank that did, fails here rather than re-ordering a meeting.
func TestTheAgendaOrderIsTheOrderTheRulesAreTried(t *testing.T) {
	t.Parallel()
	fired := make([]string, 0, len(focusRules))
	for _, rule := range focusRules {
		kind, _ := focusFor(rule.counts, rule.help)
		fired = append(fired, kind)
	}
	if !slices.Equal(fired, focusPriority) {
		t.Errorf("focusFor fires %v and the agenda ranks %v: one of the two moved, and the meeting would "+
			"then take the week in an order the review did not decide", fired, focusPriority)
	}
}

// TestEveryDeclaredFocusIsRanked is the census half: a focus kind added to the
// switch without a rank sorts LAST by agendaRank's fallback, which is a real
// order and therefore a silent one — the new rule would be raised after the
// quiet weeks with nothing failing.
func TestEveryDeclaredFocusIsRanked(t *testing.T) {
	t.Parallel()
	declared := []string{
		FocusHelpRequested, FocusLeadsBreached, FocusCommitmentsMissed,
		FocusMeetingsWithoutNextStep, FocusStrongWeek, FocusQuietWeek,
	}
	for _, kind := range declared {
		if slices.Index(focusPriority, kind) < 0 {
			t.Errorf("%s is a declared focus with no rank: it would be raised last, after the quiet weeks, "+
				"and nothing else would say so", kind)
		}
	}
	if len(focusPriority) != len(declared) {
		t.Errorf("focusPriority ranks %d kinds and %d are declared: a rank for a focus nobody produces is "+
			"an order over nothing", len(focusPriority), len(declared))
	}
}

// TestTheAgendaIsThePermutationTheContractPromises holds the claim the schema
// makes to every client: the agenda names every rep exactly once, so a client
// rendering `reps` in this order draws the whole team and nobody twice.
func TestTheAgendaIsThePermutationTheContractPromises(t *testing.T) {
	t.Parallel()
	reps := []TeamRep{
		{UserID: ids.NewV7(), DisplayName: "Ada", FocusKind: FocusQuietWeek},
		{UserID: ids.NewV7(), DisplayName: "Bo", FocusKind: FocusHelpRequested},
		{UserID: ids.NewV7(), DisplayName: "Cy", FocusKind: FocusStrongWeek},
		{UserID: ids.NewV7(), DisplayName: "Di", FocusKind: FocusLeadsBreached},
	}
	agenda := agendaOrder(reps)
	if len(agenda) != len(reps) {
		t.Fatalf("the agenda holds %d of %d reps: the contract says every id in reps appears",
			len(agenda), len(reps))
	}
	for _, rep := range reps {
		if slices.Index(agenda, rep.UserID) < 0 {
			t.Errorf("%s is on the team and not on the agenda", rep.DisplayName)
		}
	}
	// Bo asked for help, Di let a lead breach, Cy had a strong week, Ada's was
	// quiet — which is the order, and it is not the order they were handed in.
	want := []ids.UUID{reps[1].UserID, reps[3].UserID, reps[2].UserID, reps[0].UserID}
	if !slices.Equal(agenda, want) {
		t.Errorf("the agenda took the team in the order it was given rather than the order the review ranks")
	}
}

// TestTiesTakeTheNameOrderTheyCameIn keeps a second read of one frozen snapshot
// from shuffling the meeting: the store reads reps by name, and a sort that was
// not stable would order two quiet weeks differently on each open.
func TestTiesTakeTheNameOrderTheyCameIn(t *testing.T) {
	t.Parallel()
	reps := []TeamRep{
		{UserID: ids.NewV7(), DisplayName: "Ada", FocusKind: FocusQuietWeek},
		{UserID: ids.NewV7(), DisplayName: "Bo", FocusKind: FocusQuietWeek},
		{UserID: ids.NewV7(), DisplayName: "Cy", FocusKind: FocusQuietWeek},
	}
	want := []ids.UUID{reps[0].UserID, reps[1].UserID, reps[2].UserID}
	if got := agendaOrder(reps); !slices.Equal(got, want) {
		t.Error("three reps sharing one focus came back in an order other than the one they arrived in, " +
			"so one snapshot draws two different agendas")
	}
}

// TestATeamNobodyCouldBeReadForHasNoAgenda is the empty week the schema
// promises: empty exactly when reps is, so a client says the week could not be
// read rather than drawing an empty meeting.
func TestATeamNobodyCouldBeReadForHasNoAgenda(t *testing.T) {
	t.Parallel()
	if agenda := agendaOrder(nil); len(agenda) != 0 {
		t.Errorf("a team with no reps produced a %d-item agenda", len(agenda))
	}
}
