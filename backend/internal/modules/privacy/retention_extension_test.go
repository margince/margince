// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/jurisdiction"
	extdecl "github.com/margince/margince/backend/pkg/extension/privacy"
)

func TestAStricterEntityFloorKeepsTheCorrespondenceOfEveryExtensionTable(t *testing.T) {
	ref := time.Date(2030, 6, 1, 12, 0, 0, 0, time.UTC)
	short := jurisdiction.RetentionClass{Name: jurisdiction.CommercialCorrespondence, Keep: jurisdiction.Period{Years: 6}}
	long := jurisdiction.RetentionClass{Name: jurisdiction.CommercialCorrespondence, Keep: jurisdiction.Period{Years: 10}}
	table := extdecl.Table{Name: "ext_probe_note", Retention: jurisdiction.CommercialCorrespondence}

	floor := extensionFloor(table, FloorPolicy{Current: short, ByEntity: map[ids.UUID]jurisdiction.RetentionClass{ids.NewV7(): long}}, ref)

	if !floor.ProtectedSince(ref).Equal(long.ProtectedSince(ref)) {
		t.Errorf("floor keeps rows since %v, want the stricter entity's %v", floor.ProtectedSince(ref), long.ProtectedSince(ref))
	}
}

func TestACarriedFloorBindsOnlyExtensionRowsFromBeforeTheChange(t *testing.T) {
	ref := time.Date(2030, 6, 1, 12, 0, 0, 0, time.UTC)
	short := jurisdiction.RetentionClass{Name: jurisdiction.CommercialCorrespondence, Keep: jurisdiction.Period{Years: 6}}
	long := jurisdiction.RetentionClass{Name: jurisdiction.CommercialCorrespondence, Keep: jurisdiction.Period{Years: 10}}
	changed := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	table := extdecl.Table{Name: "ext_probe_note", Retention: jurisdiction.CommercialCorrespondence}

	w := extensionWindowUnder(table, retentionPolicy{RetainDays: 30}, FloorPolicy{Current: short, Carried: &CarriedFloor{Class: long, Before: changed}}, ref)

	if !w.OlderThan.Equal(short.ProtectedSince(ref)) {
		t.Errorf("every row is kept since %v, want the current floor's %v: the carried floor binds older rows only", w.OlderThan, short.ProtectedSince(ref))
	}
	if w.CarriedBefore == nil || !w.CarriedBefore.Equal(changed) || w.CarriedSince == nil || !w.CarriedSince.Equal(long.ProtectedSince(ref)) {
		t.Errorf("carried window = %v..%v, want rows before %v kept since %v", w.CarriedSince, w.CarriedBefore, changed, long.ProtectedSince(ref))
	}
}
