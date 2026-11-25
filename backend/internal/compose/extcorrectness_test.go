// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/ports/recordkinds"
)

// The set-aside read asks the bound clock, so a test or replay holds a fixed day.
func TestTheSetAsideReadAsksTheBoundClockAndFallsBackToTheWallClock(t *testing.T) {
	fixed := time.Date(2031, 3, 4, 5, 6, 7, 0, time.UTC)
	bound := &callRuntime{deps: extensionRuntimeBinding{now: func() time.Time { return fixed }}}
	if got := bound.clockNow(); !got.Equal(fixed) {
		t.Errorf("clockNow with a bound clock = %v, want %v", got, fixed)
	}
	if got := (&callRuntime{}).clockNow(); time.Since(got) > time.Minute || time.Until(got) > time.Minute {
		t.Errorf("clockNow with no bound clock = %v, want the wall clock", got)
	}
}

// A breakdown over a unit's kind groups by a declared column with no admin
// opt-in. The kind's read right is the gate, as the decision on
// extensionKindSpec records.
func TestAnExtensionBreakdownNeedsNoReportableOptIn(t *testing.T) {
	spec := extensionKindSpec(recordkinds.Declared{Object: "ext_alpha_visit", Table: "ext_alpha_visit", DisplayName: "Visit"}, "channel")
	if _, ok := spec.dimensions["channel"]; !ok {
		t.Errorf("dimensions = %v, want channel offered with no catalog lookup", spec.dimensions)
	}
	if len(spec.customFields) != 0 {
		t.Errorf("customFields = %v, want none: the opt-in is for admin-defined fields", spec.customFields)
	}
}
