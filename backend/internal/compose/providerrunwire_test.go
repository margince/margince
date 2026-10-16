// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/provider"
)

// A run that held no credits (a skipped one) still writes `reservations`: the
// contract requires the array.
func TestARunWithNoReservationsWritesAnEmptyArray(t *testing.T) {
	t.Parallel()
	wire, err := json.Marshal(toProviderRun(provider.Run{Snapshot: provider.Snapshot{Mode: "on_demand"}}))
	if err != nil || !strings.Contains(string(wire), `"reservations":[]`) {
		t.Errorf("run = %s (%v), want reservations written as an empty array", wire, err)
	}
}

func TestARunKeepsEachReservationItHeld(t *testing.T) {
	t.Parallel()
	spent := 3
	run := toProviderRun(provider.Run{Reservations: []provider.Reservation{{Pool: "mobile", Reserved: 5, Actual: &spent}}})
	if len(run.Reservations) != 1 || run.Reservations[0].Pool != "mobile" ||
		run.Reservations[0].ReservedCredits != 5 || run.Reservations[0].ActualCredits == nil || *run.Reservations[0].ActualCredits != 3 {
		t.Errorf("reservations = %+v, want the one held pool with 5 reserved and 3 spent", run.Reservations)
	}
}
