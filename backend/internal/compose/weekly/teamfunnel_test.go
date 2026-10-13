// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package weekly

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func entry(owner, stage ids.UUID, entered, converted int) stageEntries {
	return stageEntries{
		owner: owner, entered: entered, converting: converted,
		deviation: FunnelDeviation{StageID: stage, StageName: stage.String()},
	}
}

// The stages furthest from the team come largest first. The team side sums
// every member, and a thin stage is left out on either side.
func TestTheFunnelRanksTheLargestDeviationsFirst(t *testing.T) {
	rep, other := ids.NewV7(), ids.NewV7()
	s1, s2, s3, s4, thin := ids.NewV7(), ids.NewV7(), ids.NewV7(), ids.NewV7(), ids.NewV7()
	entries := []stageEntries{
		entry(rep, s1, 10, 2), entry(other, s1, 10, 8), // rep 20%, team 50%: -3000
		entry(rep, s2, 10, 5), entry(other, s2, 10, 5), // equal: not a deviation
		entry(rep, s3, 10, 9), entry(other, s3, 10, 5), // rep 90%, team 70%: +2000
		entry(rep, s4, 5, 4), entry(other, s4, 5, 4), // equal
		entry(rep, thin, 4, 0), entry(other, thin, 10, 10), // under the floor
	}
	got := repDeviations(rep, entries)
	if len(got) != 2 {
		t.Fatalf("got %d deviations, want 2: %+v", len(got), got)
	}
	if got[0].StageID != s1 || got[0].DeviationBp != -3000 || got[0].TeamEntered != 20 {
		t.Errorf("first = %+v, want stage 1 at -3000 over 20 team entries", got[0])
	}
	if got[1].StageID != s3 || got[1].DeviationBp != 2000 {
		t.Errorf("second = %+v, want stage 3 at +2000", got[1])
	}
}

// Equal distances break on the stage id, so a re-run keeps the same three.
func TestFunnelTiesBreakOnTheStage(t *testing.T) {
	rep, other := ids.NewV7(), ids.NewV7()
	var entries []stageEntries
	stages := []ids.UUID{ids.NewV7(), ids.NewV7(), ids.NewV7(), ids.NewV7()}
	for _, s := range stages {
		entries = append(entries, entry(rep, s, 10, 0), entry(other, s, 10, 10))
	}
	got := repDeviations(rep, entries)
	if len(got) != funnelDeviationsPerRep {
		t.Fatalf("got %d, want %d", len(got), funnelDeviationsPerRep)
	}
	for i := 1; i < len(got); i++ {
		if got[i-1].StageID.String() > got[i].StageID.String() {
			t.Errorf("ties came back out of stage order: %v then %v", got[i-1].StageID, got[i].StageID)
		}
	}
}
