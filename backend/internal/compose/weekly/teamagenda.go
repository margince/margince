// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package weekly

import (
	"slices"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The Monday agenda: the team's own week, in the order a lead should take it.
//
// Not a fixed number of items and no minute budgets. A week with two things
// worth discussing is a two-item meeting, and a budget printed beside an item
// would read as guidance somebody derived from something — while nothing in the
// product knows a team's size or how long it meets.

// focusPriority is the order a lead raises the focuses in, which is the order
// focusFor tries its rules in and the order the constants are declared in.
//
// Stated as a list because an ORDER is what the agenda needs and a switch
// cannot be asked for one. TestTheAgendaOrderIsTheOrderTheRulesAreTried holds
// the two together in both directions: every declared focus is ranked here, and
// the rules fire in this sequence.
var focusPriority = []string{
	FocusHelpRequested,
	FocusLeadsBreached,
	FocusCommitmentsMissed,
	FocusMeetingsWithoutNextStep,
	FocusStrongWeek,
	FocusQuietWeek,
}

// agendaOrder is the reps' ids in agenda order: the thing to raise first, the
// quiet week last.
//
// An ORDER over the reps rather than a second list of them. The items are the
// reps' own focuses, already assembled and already on the wire; composing a
// parallel list of the same labels would be one agenda spelled twice.
//
// Ties break on the rep order the caller hands in, which is by name, so two
// reps carrying the same focus appear in an order a reader can predict and a
// second read of one snapshot cannot shuffle.
func agendaOrder(reps []TeamRep) []ids.UUID {
	ordered := make([]int, len(reps))
	for i := range ordered {
		ordered[i] = i
	}
	slices.SortStableFunc(ordered, func(a, b int) int {
		return agendaRank(reps[a].FocusKind) - agendaRank(reps[b].FocusKind)
	})
	agenda := make([]ids.UUID, 0, len(reps))
	for _, i := range ordered {
		agenda = append(agenda, reps[i].UserID)
	}
	return agenda
}

// agendaRank is where a focus sits in the meeting.
//
// A kind nobody ranked sorts LAST rather than first: an unranked focus is one
// this file has not been told about, and opening the meeting with it would put
// the least understood item where the most urgent one belongs. The census that
// makes the case unreachable is in the test beside this.
func agendaRank(kind string) int {
	if rank := slices.Index(focusPriority, kind); rank >= 0 {
		return rank
	}
	return len(focusPriority)
}
