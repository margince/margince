// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package jobs

// Which moment a "next pass" answer reports.
//
// The screen this feeds exists to say WHEN, so a plausible time is
// indistinguishable from a right one — nothing downstream can tell them apart,
// and a reader plans around whichever they are shown.

import (
	"testing"
	"time"
)

func TestNextPassPrefersTheRunRiverHasAlreadyScheduled(t *testing.T) {
	t.Parallel()

	scheduled := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)
	ranAt := scheduled.Add(-time.Hour)

	got := nextPass(&scheduled, &ranAt, 20*time.Minute)
	if got == nil || !got.Equal(scheduled) {
		t.Errorf("next = %v, want the scheduled row %v — that is the fire time, and the "+
			"projection beside it is only an estimate of it", got, scheduled)
	}
}

// Projected from the completed run's OWN moment, not from when it finished.
// River's ticks are an interval apart from each other, so projecting off the
// finish runs late by however long the pass took — up to the twenty minutes
// these passes are allowed, on the screen that exists to say when.
func TestNextPassProjectsFromTheLastRunsOwnMoment(t *testing.T) {
	t.Parallel()

	ranAt := time.Date(2026, 9, 6, 11, 0, 0, 0, time.UTC)
	const every = 20 * time.Minute

	got := nextPass(nil, &ranAt, every)
	if want := ranAt.Add(every); got == nil || !got.Equal(want) {
		t.Errorf("next = %v, want %v", got, want)
	}
}

// Nothing scheduled and nothing completed inside River's retention answers
// NOTHING. Nil is not "soon": it is what such a deployment honestly knows, and
// the caller says the cadence instead rather than inventing a time.
func TestNextPassSaysNothingRatherThanGuessing(t *testing.T) {
	t.Parallel()

	ranAt := time.Date(2026, 9, 6, 11, 0, 0, 0, time.UTC)
	for name, got := range map[string]*time.Time{
		"nothing scheduled and nothing completed": nextPass(nil, nil, 20*time.Minute),
		"a completed run but no cadence to add":   nextPass(nil, &ranAt, 0),
	} {
		if got != nil {
			t.Errorf("%s: answered %v — a time invented here is one somebody plans around", name, *got)
		}
	}
}
