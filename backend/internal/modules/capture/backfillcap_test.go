// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"slices"
	"testing"
)

// Unset means uncapped, which is what every deployment has today.
func TestNoCapOffersTheWholeSupportedSet(t *testing.T) {
	t.Parallel()
	r := &Registry{}
	if got, want := r.OfferedBackfillWindows(), BackfillWindowMonths(); !slices.Equal(got, want) {
		t.Fatalf("offered %v with no cap, want the product's own set %v", got, want)
	}
	for _, m := range BackfillWindowMonths() {
		if !r.admitsWindow(m) {
			t.Errorf("an uncapped installation refused %d months", m)
		}
	}
}

// A cap narrows the offer to what it admits, and the widest offered window is
// the cap itself when the cap is one of the supported values.
func TestACapNarrowsTheOfferedSet(t *testing.T) {
	t.Parallel()
	r := (&Registry{}).WithMaxBackfillMonths(24)
	got := r.OfferedBackfillWindows()
	if len(got) == 0 {
		t.Fatal("a cap of 24 months offered nothing")
	}
	for _, m := range got {
		if m > 24 {
			t.Errorf("offered %d months under a 24-month cap", m)
		}
	}
	if !slices.Contains(got, 24) {
		t.Error("the cap itself is not offered — a ceiling nobody may reach is not a ceiling")
	}
	if slices.Contains(got, 120) {
		t.Error("the widest window survived a cap that excludes it")
	}
}

func TestACapRefusesAWindowAboveIt(t *testing.T) {
	t.Parallel()
	r := (&Registry{}).WithMaxBackfillMonths(24)
	if r.admitsWindow(36) {
		t.Error("36 months admitted under a 24-month cap")
	}
	if !r.admitsWindow(12) {
		t.Error("12 months refused under a 24-month cap")
	}
}

// A window outside the product's own set stays refused however the cap is set:
// the cap narrows, it never widens.
func TestACapNeverAdmitsAnUnsupportedWindow(t *testing.T) {
	t.Parallel()
	r := (&Registry{}).WithMaxBackfillMonths(240)
	if r.admitsWindow(240) {
		t.Error("a cap admitted a window the contract and the database do not have")
	}
	if got := r.OfferedBackfillWindows(); !slices.Equal(got, BackfillWindowMonths()) {
		t.Errorf("a cap above the ceiling changed the offer to %v", got)
	}
}

// Zero and negatives are "no cap" rather than "cap at nothing", which would
// leave an installation unable to import at all.
func TestANonPositiveCapIsNoCap(t *testing.T) {
	t.Parallel()
	for _, months := range []int{0, -1} {
		r := (&Registry{}).WithMaxBackfillMonths(months)
		if got, want := r.OfferedBackfillWindows(), BackfillWindowMonths(); !slices.Equal(got, want) {
			t.Errorf("a cap of %d offered %v, want the whole set", months, got)
		}
	}
}

// The builder configures in place and returns the registry, which is the shape
// every other With… on this type takes — it carries a mutex and must not be
// copied. A non-positive value leaves whatever was already set alone, so a
// caller passing an unset config cannot clear a ceiling somebody configured.
func TestWithMaxBackfillMonthsConfiguresInPlaceAndIgnoresNonPositives(t *testing.T) {
	t.Parallel()
	r := &Registry{}
	if got := r.WithMaxBackfillMonths(6); got != r {
		t.Error("the builder returned a different registry — this type carries a mutex and is configured in place")
	}
	if r.capOrUnbounded() != 6 {
		t.Fatalf("cap = %d, want 6", r.capOrUnbounded())
	}
	r.WithMaxBackfillMonths(0)
	if r.capOrUnbounded() != 6 {
		t.Errorf("a zero cleared a configured ceiling, leaving %d", r.capOrUnbounded())
	}
}
