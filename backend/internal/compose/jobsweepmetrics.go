// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The two sweep-pair families beside jobmetrics.go's per-job gauges: "are
// tenants being missed" at the workspace grain and, where a kind's declared
// unit is finer, at the unit grain too. Split out of jobmetrics.go because
// the sweep passes answer a different question (did every tenant's share of
// a pass run) from the job-runtime gauges next door (what is a queue
// currently holding).

import (
	"cmp"
	"fmt"
	"io"
	"slices"

	"github.com/margince/margince/backend/internal/platform/jobs"
)

// writeSweepGauges renders the pair that answers "are tenants being
// missed". Both halves carry the same sweep label so an alert can compare
// them; for a fan-out kind the label is the CHILD kind, which is what the
// rows hold — mapping back to the dispatcher would be a hand-kept table.
// For a Fleet kind that fans out to nothing (ADR-0103's collapsed pass) the
// label is the kind's own name, and the reading is 1 (or 0, if the pass has
// never run) rather than a true workspace count — there is no workspace
// grain to read for it at all.
func writeSweepGauges(w io.Writer, sweeps []jobs.SweepPass) error {
	ordered := slices.SortedFunc(slices.Values(sweeps), func(a, b jobs.SweepPass) int {
		return cmp.Compare(a.Kind, b.Kind)
	})

	if err := writeFamilyHeader(w, "margince_sweep_workspaces",
		"Workspaces with a surviving child of this fleet pass. Counted per workspace rather than per pass: a child still active from an earlier fan-out is deduplicated out of the current one and writes no new row, so no batch can be identified. A workspace whose only child aged out of River's job retention is absent rather than reported as zero. For a kind that answers for the whole installation in one row rather than fanning out, this reads 1 when its latest tagged run exists — the closest reading to 'did the pass happen' when there is no workspace to count."); err != nil {
		return err
	}
	for _, s := range ordered {
		if _, err := fmt.Fprintf(w, "margince_sweep_workspaces{sweep=%s} %d\n",
			label(s.Kind), s.Workspaces); err != nil {
			return err
		}
	}

	if err := writeFamilyHeader(w, "margince_sweep_workspaces_failed",
		"Workspaces whose MOST RECENT child of this fleet pass ended discarded or cancelled — tenants whose share of the pass did not happen. A workspace that failed and then succeeded is not counted. For a kind that answers for the whole installation in one row, this reads 1 when its latest tagged run ended that way."); err != nil {
		return err
	}
	for _, s := range ordered {
		if _, err := fmt.Fprintf(w, "margince_sweep_workspaces_failed{sweep=%s} %d\n",
			label(s.Kind), s.Failed); err != nil {
			return err
		}
	}
	return nil
}

// writeSweepUnitGauges renders the pair that answers the same question one
// grain down, for the dispatchers that fan out per connection or per build.
// The workspace pair above is the coarser reading of the same passes and can
// mask a failed unit behind a healthy sibling in the same workspace; these two
// cannot, because each unit is counted in its own right.
//
// Present ONLY for kinds whose declared unit is finer than a workspace. For
// every other fan-out kind the unit IS the workspace, so this pair would repeat
// the one above value for value, and two published series for one number is a
// worse surface than one series and a documented pairing. The unit rides as a
// label so an alert can see which grain it is reading without a lookup.
func writeSweepUnitGauges(w io.Writer, units []jobs.SweepUnit) error {
	ordered := slices.SortedFunc(slices.Values(units), func(a, b jobs.SweepUnit) int {
		return cmp.Compare(a.Kind, b.Kind)
	})

	if err := writeFamilyHeader(w, "margince_sweep_units",
		"Fan-out units with a surviving child of this fleet pass, for the dispatchers that fan out per connection or per build rather than per workspace. The unit label names the grain. Kinds that fan out per workspace are absent here and reported by margince_sweep_workspaces, which is the same measurement for them."); err != nil {
		return err
	}
	for _, u := range ordered {
		if _, err := fmt.Fprintf(w, "margince_sweep_units{sweep=%s,unit=%s} %d\n",
			label(u.Kind), label(fanOutUnitName(u.Unit)), u.Units); err != nil {
			return err
		}
	}

	if err := writeFamilyHeader(w, "margince_sweep_units_failed",
		"Fan-out units whose MOST RECENT child of this fleet pass ended discarded or cancelled. Unlike the per-workspace pair, a failed connection is counted even when a sibling connection in the same workspace succeeded afterwards -- which is the masking this pair exists to remove."); err != nil {
		return err
	}
	for _, u := range ordered {
		if _, err := fmt.Fprintf(w, "margince_sweep_units_failed{sweep=%s,unit=%s} %d\n",
			label(u.Kind), label(fanOutUnitName(u.Unit)), u.Failed); err != nil {
			return err
		}
	}
	return nil
}
