// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package goalgap is how far a projected landing falls short of a goal.
package goalgap

// Of is the goal less the landing. Above zero the landing falls short; zero or
// less meets the goal. A report's target comparison and a team's focus list
// both read it, so one period has one gap wherever it is shown.
func Of(goal, landing int64) int64 {
	return goal - landing
}
