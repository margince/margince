// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The two sweep-pair families beside jobmetrics.go's per-job gauges. They
// answer "did the pass run" per kind and, where a kind's unit is finer than the
// installation, per unit. The job-runtime gauges next door answer what a queue
// holds.

import (
	"cmp"
	"fmt"
	"io"
	"slices"

	"github.com/margince/margince/backend/internal/platform/jobs"
)

// writeSweepGauges renders the pair that answers "did the pass run". Both
// halves carry the same sweep label so an alert can compare them. For a fan-out
// kind the label is the child kind, which is what the rows hold. The reading is
// 1 when the kind has a tagged row, since an installation holds one company and
// so one pass to count. The series keep their names so a dashboard reading them
// keeps working.
func writeSweepGauges(w io.Writer, sweeps []jobs.SweepPass) error {
	ordered := slices.SortedFunc(slices.Values(sweeps), func(a, b jobs.SweepPass) int {
		return cmp.Compare(a.Kind, b.Kind)
	})

	if err := writeFamilyHeader(w, "margince_sweep_workspaces",
		"1 when this fleet pass has a surviving tagged row, so the pass has run within River's job retention. A pass whose only row aged out of that retention is absent rather than reported as zero."); err != nil {
		return err
	}
	for _, s := range ordered {
		if _, err := fmt.Fprintf(w, "margince_sweep_workspaces{sweep=%s} %d\n",
			label(s.Kind), s.Passes); err != nil {
			return err
		}
	}

	if err := writeFamilyHeader(w, "margince_sweep_workspaces_failed",
		"1 when the most recent row of this fleet pass THAT ENDED was discarded or cancelled, so the pass did not happen. A pass that failed and then succeeded is not counted; one that failed and is running again still is, until the new run succeeds."); err != nil {
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
// The pair above can mask a failed unit behind a healthy sibling. These two
// cannot, because each unit is counted on its own.
//
// Present only for kinds whose declared unit is finer than the installation.
// For every other kind this pair would repeat the one above value for value.
// The unit rides as a label so an alert can see which grain it reads.
func writeSweepUnitGauges(w io.Writer, units []jobs.SweepUnit) error {
	ordered := slices.SortedFunc(slices.Values(units), func(a, b jobs.SweepUnit) int {
		return cmp.Compare(a.Kind, b.Kind)
	})

	if err := writeFamilyHeader(w, "margince_sweep_units",
		"Fan-out units with a surviving child of this fleet pass, for the dispatchers that fan out per connection or per build. The unit label names the grain. Kinds that fan out once for the installation are absent here and reported by margince_sweep_workspaces, which is the same measurement for them."); err != nil {
		return err
	}
	for _, u := range ordered {
		if _, err := fmt.Fprintf(w, "margince_sweep_units{sweep=%s,unit=%s} %d\n",
			label(u.Kind), label(fanOutUnitName(u.Unit)), u.Units); err != nil {
			return err
		}
	}

	if err := writeFamilyHeader(w, "margince_sweep_units_failed",
		"Fan-out units whose most recent child of this fleet pass THAT ENDED was discarded or cancelled. Unlike the per-kind pair, a failed connection is counted even when a sibling connection succeeded afterwards -- which is the masking this pair exists to remove."); err != nil {
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
