// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The fold from daily rows onto one entry per unit.
//
// It is asked of appendRefusal directly because the shape it has to get right
// is a property of the ROW ORDER the statement promises — unit, then that
// unit's classes largest first — and a database test would prove the same
// arithmetic while hiding which half was wrong. The statement's ordering is
// asserted where it is written; this is what the ordering buys.

import (
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func TestOneUnitsClassesFoldOntoOneEntry(t *testing.T) {
	t.Parallel()

	early := time.Date(2026, 9, 15, 9, 0, 0, 0, time.UTC)
	late := time.Date(2026, 9, 17, 4, 0, 0, 0, time.UTC)

	var units []crmcontracts.ExtensionUnitIngestHealth
	units = appendRefusal(units, "openchannel", "participants", 40, late)
	units = appendRefusal(units, "openchannel", "key", 2, early)
	units = appendRefusal(units, "relaydemo", "activity", 7, early)

	if len(units) != 2 {
		t.Fatalf("got %d units, want one entry per unit: %+v", len(units), units)
	}
	first := units[0]
	if first.Unit != "openchannel" || first.Refused != 42 {
		t.Errorf("first entry = %q refused %d, want openchannel refused 42", first.Unit, first.Refused)
	}
	if len(first.Refusals) != 2 {
		t.Fatalf("got %d classes for openchannel, want both", len(first.Refusals))
	}
	// Largest first is the statement's order and the reason it is worth having:
	// the class an operator acts on is the one refusing most of the traffic.
	if first.Refusals[0].Refusal != "participants" {
		t.Errorf("the classes are not largest-first: %+v", first.Refusals)
	}
	// The unit's newest refusal is the newest of its classes', not the last
	// row read — the rows arrive ordered by count, so the last one is
	// routinely the oldest.
	if first.LastRefusedAt == nil || !first.LastRefusedAt.Equal(late) {
		t.Errorf("last_refused_at = %v, want the newest of the unit's classes (%v)", first.LastRefusedAt, late)
	}
}

func TestASecondUnitOpensItsOwnEntry(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, 9, 17, 4, 0, 0, 0, time.UTC)
	var units []crmcontracts.ExtensionUnitIngestHealth
	units = appendRefusal(units, "alpha", "key", 1, at)
	units = appendRefusal(units, "zulu", "key", 1, at)

	if len(units) != 2 {
		t.Fatalf("two units folded onto %d entries — a fold that only ever extends the last entry "+
			"reports one unit's refusals under another's name", len(units))
	}
	if units[1].Unit != "zulu" || units[1].Refused != 1 {
		t.Errorf("second entry = %+v, want zulu refused 1", units[1])
	}
}
