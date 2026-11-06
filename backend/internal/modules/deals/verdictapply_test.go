// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

// A date the customer named is no longer an estimate, so the change clears
// the provisional flag with the date, and Undo puts both back.
func TestAVerdictDateClearsTheProvisionalFlag(t *testing.T) {
	t.Parallel()
	old := time.Date(2026, 11, 30, 0, 0, 0, 0, time.UTC)
	patch := storekit.NewPatch()
	change := VerdictChange{Correction: VerdictCloseDateCorrection, CloseDate: old.AddDate(0, 2, 0)}
	if !verdictPatch(patch, change, VerdictValues{CloseDate: &old, Provisional: true}) {
		t.Fatal("the date change was refused")
	}
	if got, ok := patch.After()[provisionalField]; !ok || got != false {
		t.Fatalf("provisional after = %v (set %v), want false", got, ok)
	}
}

// A flag confirmed since the read is a different deal from the one decided on.
func TestAConfirmedFlagSinceTheReadMakesTheDateStale(t *testing.T) {
	t.Parallel()
	day := time.Date(2026, 11, 30, 0, 0, 0, 0, time.UTC)
	_, unchanged := verdictColumns(VerdictCloseDateCorrection,
		VerdictValues{CloseDate: &day, Provisional: true}, VerdictValues{CloseDate: &day, Provisional: false})
	if unchanged {
		t.Fatal("a provisional flag cleared since the read did not make the change stale")
	}
}
