// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// How much an age is allowed to decide. A wait, a drifting deal and a lapsed
// relationship each measure a span of days and the ordering step compares them
// against each other, so the bound belongs to the comparison rather than to any
// one classifier.

// waitingDaysCeiling bounds what age contributes to the ORDER. Age breaks ties
// the bands could not separate; it does not earn precedence on its own.
// Uncapped it does exactly that — every further day of silence outranks every
// newer wait forever, and the queue becomes an archive sorted by how long it has
// been ignored. Past the ceiling all waits tie and the next tie-break decides,
// which is honest: at six months against seven, age says nothing about what to
// do first.
const waitingDaysCeiling = 30

// orderingAge is the age the ordering step reads, bounded by the ceiling. Every
// source that measures an age answers here, so two deals quiet ninety and three
// days cannot tie on an age neither of them showed.
func orderingAge(days int) int {
	if days > waitingDaysCeiling {
		return waitingDaysCeiling
	}
	return days
}
