// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package providerwire

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/provider"
)

// A run that held no credits (a skipped one) still writes `reservations`: the
// contract requires the array.
func TestARunWithNoReservationsWritesAnEmptyArray(t *testing.T) {
	t.Parallel()
	wire, err := json.Marshal(Run(provider.Run{Snapshot: provider.Snapshot{Mode: "on_demand"}}))
	if err != nil || !strings.Contains(string(wire), `"reservations":[]`) {
		t.Errorf("run = %s (%v), want reservations written as an empty array", wire, err)
	}
}

func TestARunKeepsEachReservationItHeld(t *testing.T) {
	t.Parallel()
	spent := 3
	run := Run(provider.Run{Reservations: []provider.Reservation{{Pool: "mobile", Reserved: 5, Actual: &spent}}})
	if len(run.Reservations) != 1 || run.Reservations[0].Pool != "mobile" ||
		run.Reservations[0].ReservedCredits != 5 || run.Reservations[0].ActualCredits == nil || *run.Reservations[0].ActualCredits != 3 {
		t.Errorf("reservations = %+v, want the one held pool with 5 reserved and 3 spent", run.Reservations)
	}
}

// A skipped run says why it sent nothing, and which contact it was for, on
// whichever door reads it.
func TestASkippedRunCarriesItsReasonItsContactAndItsOwnTimes(t *testing.T) {
	t.Parallel()
	created := time.Date(2026, 10, 9, 8, 0, 0, 0, time.UTC)
	updated := created.Add(time.Minute)
	contact, id := ids.NewV7().String(), ids.NewV7().String()
	for _, reason := range []provider.SkipReason{provider.SkipAlreadyFresh, provider.SkipBudgetExhausted} {
		run := Run(provider.Run{
			ID: id, ContactID: contact, State: provider.RunSkipped, SkipReason: reason,
			SafeStatusCode: "declined", CreatedAt: created, UpdatedAt: updated,
		})
		if run.SkipReason == nil || string(*run.SkipReason) != string(reason) {
			t.Errorf("skip_reason = %v, want %s", run.SkipReason, reason)
		}
		if run.ContactId == nil || run.ContactId.String() != contact {
			t.Errorf("contact_id = %v, want %s", run.ContactId, contact)
		}
		if run.SafeStatusCode == nil || *run.SafeStatusCode != "declined" {
			t.Errorf("safe_status_code = %v, want declined", run.SafeStatusCode)
		}
		if !run.UpdatedAt.Equal(updated) {
			t.Errorf("updated_at = %v, want the run's own %v", run.UpdatedAt, updated)
		}
		if run.State != crmcontracts.ProviderRunStateSkipped {
			t.Errorf("state = %s, want skipped", run.State)
		}
	}
}

func TestARunWithNoSkipReasonOrContactLeavesThoseKeysNull(t *testing.T) {
	t.Parallel()
	run := Run(provider.Run{ID: ids.NewV7().String(), State: provider.RunCompleted})
	if run.SkipReason != nil || run.ContactId != nil || run.SafeStatusCode != nil {
		t.Errorf("run = %+v, want skip_reason, contact_id and safe_status_code absent", run)
	}
}
