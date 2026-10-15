// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package weekly

import (
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// retiredFocusKinds are ranked and never written: snapshots frozen while they
// were still carry them.
var retiredFocusKinds = []string{FocusDealsAtRisk}

// TestTheAgendaOrderIsTheOrderTheRulesAreTried stops the agenda's ranking
// drifting from patternsFor's rule order. It fires each rule alone, in the
// order focusRules lists them. The result must be focusPriority without the
// kinds no longer written.
func TestTheAgendaOrderIsTheOrderTheRulesAreTried(t *testing.T) {
	t.Parallel()
	fired := make([]string, 0, len(focusRules))
	for _, rule := range focusRules {
		kind, _ := focusFor(rule.counts, rule.help, rule.deals)
		fired = append(fired, kind)
	}
	written := slices.DeleteFunc(slices.Clone(focusPriority), func(kind string) bool {
		return slices.Contains(retiredFocusKinds, kind)
	})
	if !slices.Equal(fired, written) {
		t.Errorf("patternsFor fires %v and the agenda ranks %v: one of the two moved, and the meeting would "+
			"then take the week in an order the review did not decide", fired, written)
	}
}

// TestEveryDeclaredFocusIsRanked is the census of the declared vocabulary.
// It is kept apart from the one derived from patternsFor, so a kind no longer
// produced is still ranked: older snapshots carry it.
func TestEveryDeclaredFocusIsRanked(t *testing.T) {
	t.Parallel()
	declared := []string{
		FocusHelpRequested, FocusLeadsBreached, FocusCommitmentsMissed, FocusDealsAtRisk,
		FocusForecastDowngrades, FocusStageRegressions, FocusCloseDatesProvisional,
		FocusNoNextStepCoverage, FocusMeetingsWithoutNextStep, FocusStrongWeek, FocusQuietWeek,
	}
	for _, kind := range declared {
		if slices.Index(focusPriority, kind) < 0 {
			t.Errorf("%s is a declared focus with no rank: it would be raised last, after the quiet weeks, "+
				"and nothing else would say so", kind)
		}
	}
	if len(focusPriority) != len(declared) {
		t.Errorf("focusPriority ranks %d kinds and %d are declared: a rank for a focus nobody declares is "+
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
