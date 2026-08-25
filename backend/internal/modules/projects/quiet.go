// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package projects

// "Gone quiet" as a predicate on the project row, spelled once. The
// projects-gone-quiet report, the project_gone_quiet signal producer and the
// morning digest's projects section all ask the same question, and three
// spellings of "older than N days" would be three answers the first time one
// of them changed.

import (
	"fmt"

	"github.com/gradionhq/margince/backend/internal/shared/kernel/idlebase"
)

// DefaultProjectQuietDays is how long a project in flight may go without a
// filed activity before it counts as quiet, when the caller names no window.
const DefaultProjectQuietDays = 30

// ProjectInFlightSQL is the phase predicate under the quiet rule: only a
// project being pursued or delivered is expected to show activity. An
// initiative has not started and a closed project has ended, so silence on
// either is not a finding. alias names the project row.
func ProjectInFlightSQL(alias string) string {
	return fmt.Sprintf("%s.phase IN ('%s', '%s')", alias, PhasePursuing, PhaseDelivering)
}

// ProjectQuietSQL is the silence predicate: nothing was filed against the
// project for at least daysPos days, counted back from nowSQL. A project with
// no activity at all is measured from its creation — a body of work opened
// two months ago that nobody has touched is exactly what the rule exists to
// surface, and NULL would otherwise exempt it.
//
// nowSQL is an SQL expression for the instant to measure from — `now()` for a
// request, or a bound parameter for a producer pass that carries its own
// clock — and daysPos is the bind position holding the integer day count.
func ProjectQuietSQL(alias, nowSQL string, daysPos int) string {
	// nowSQL is cast so a bound parameter is typed by the comparison rather
	// than guessed as an interval from the subtraction beside it.
	return fmt.Sprintf("%[1]s < (%[2]s)::timestamptz - make_interval(days => $%[3]d)",
		ProjectQuietAnchorSQL(alias), nowSQL, daysPos)
}

// ProjectQuietAnchorSQL is the instant the quiet rule measured from — the
// expression ProjectQuietSQL compares, which is why that predicate is built
// from THIS function rather than from a second coalesce beside it. A caller
// reporting "quiet since", or keying a finding to one quiet episode, reads the
// value the rule used.
func ProjectQuietAnchorSQL(alias string) string {
	return idlebase.SQL(alias)
}
