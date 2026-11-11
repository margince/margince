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
