// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A frozen task keeps the clock it was chosen on, so the packet names the day
// it was set for in any reader's zone. A pack frozen before the clock was
// stored carries none and reads as an instant.
func TestAFrozenTaskCarriesItsChosenDate(t *testing.T) {
	due := time.Date(2026, 11, 17, 7, 59, 59, 0, time.UTC)
	intent := &crmcontracts.IntentTime{WallClock: "2026-11-16T23:59:59", Timezone: "America/Los_Angeles"}
	dated, old := ids.NewV7(), ids.NewV7()
	subject := "Send the redlines"
	packet := reviewPacket{Contracts: []PacketContract{}, Tasks: []PacketTask{
		{ActivityID: dated, DueAt: &due, DueIntent: intent},
		{ActivityID: old, DueAt: &due},
	}}
	sight := recordSight{activity: map[ids.UUID]*string{dated: &subject, old: &subject}}
	var out crmcontracts.LegalReviewPacket
	recordsWire(packet, sight, &out)
	tasks := *out.Tasks
	if tasks[0].DueHasTime == nil || *tasks[0].DueHasTime || tasks[0].DueIntent == nil || tasks[0].DueIntent.WallClock != intent.WallClock {
		t.Errorf("dated task = %+v, want has_time false and its chosen clock", tasks[0])
	}
	if tasks[1].DueHasTime != nil || tasks[1].DueIntent != nil {
		t.Errorf("a pack frozen before the clock was stored read as %+v, want an instant", tasks[1])
	}
}
