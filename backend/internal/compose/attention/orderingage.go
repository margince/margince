// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// How much an age is allowed to decide.
//
// Apart from the classifiers because all three read it. A wait, a drifting deal
// and a lapsed relationship each measure a span of days, and the ordering step
// compares them against each other — so the bound belongs to none of the three
// and to the comparison they share.

// waitingDaysCeiling bounds what age contributes to the ORDER.
//
// Age breaks ties between rows the bands could not separate; it does not earn
// precedence on its own. Uncapped it does exactly that — every additional day
// of silence outranks every newer wait forever, so the oldest thread in the
// workspace leads the page permanently and the queue becomes an archive sorted
// by how long it has been ignored. That is the live page's own defect: eight
// half-year-old threads holding the top of a working rep's day.
//
// Past the ceiling all waits tie on age and the next tie-break decides, which
// is the honest answer — at six months versus seven, age has stopped saying
// anything about what to do first.
const waitingDaysCeiling = 30

// orderingAge is the age the ordering step reads, bounded by the ceiling.
//
// Every source that measures an age in days answers here, and that is the
// point rather than a convenience: the three classifiers each held their own
// age, and two of them left waitingRank at its zero value. A deal quiet ninety
// days and one quiet three then tied on the ordering step while both rows
// PRINTED their true age, so the page ordered by something no reader could see
// and explained itself with a figure that had not decided anything.
func orderingAge(days int) int {
	if days > waitingDaysCeiling {
		return waitingDaysCeiling
	}
	return days
}
