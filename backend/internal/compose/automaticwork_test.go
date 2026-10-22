// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// An item whose colleague is gone stays in the total as unattributed, so the
// counts an admin reads still add up to what automation created.
func TestAutomaticWorkCountsAnItemWithNoSeatAsUnattributed(t *testing.T) {
	seat, gone := ids.NewV7(), ids.NewV7()
	day := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	rows := []automaticWorkRow{
		{colleague: &seat, kind: crmcontracts.AutomaticWorkKindNamePromiseTask, day: day, outcome: "open", count: 2},
		{colleague: &gone, kind: crmcontracts.AutomaticWorkKindNamePromiseTask, day: day, outcome: "open", count: 1},
		{colleague: nil, kind: crmcontracts.AutomaticWorkKindNameApprovalRequest, day: day, outcome: "rejected", count: 3},
	}
	byColleague, unattributed, err := groupAutomaticWork(rows, []automaticWorkSeat{{id: seat}})
	if err != nil {
		t.Fatal(err)
	}
	if unattributed != 4 {
		t.Errorf("unattributed = %d, want the removed seat's item and the three naming nobody", unattributed)
	}
	if got := byColleague[seat][crmcontracts.AutomaticWorkKindNamePromiseTask][day].Open; got != 2 {
		t.Errorf("the seat's open items = %d, want 2", got)
	}
}

func TestAutomaticWorkRefusesAnOutcomeItDoesNotKnow(t *testing.T) {
	seat := ids.NewV7()
	rows := []automaticWorkRow{{colleague: &seat, kind: crmcontracts.AutomaticWorkKindNamePromiseTask, outcome: "lost", count: 1}}
	if _, _, err := groupAutomaticWork(rows, []automaticWorkSeat{{id: seat}}); err == nil {
		t.Fatal("an outcome the report has no column for was dropped instead of refused")
	}
}

// The share is flagged only on a sample large enough to judge, and never for
// a kind with no "Not mine".
func TestAutomaticWorkFlagsAShareOnlyOnASampleLargeEnough(t *testing.T) {
	day := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	colleague := colleagueDays{
		crmcontracts.AutomaticWorkKindNamePromiseTask:     {day: {Created: 10, NotMine: 2, Open: 8}},
		crmcontracts.AutomaticWorkKindNameReplyReminder:   {day: {Created: 2, NotMine: 2}},
		crmcontracts.AutomaticWorkKindNameApprovalRequest: {day: {Created: 20, Rejected: 20}},
	}
	got := tallies(colleague)
	if !got[0].AboveThreshold || got[0].NotMineShare == nil || *got[0].NotMineShare != 0.2 {
		t.Errorf("ten items, two not mine: %+v", got[0])
	}
	if got[1].AboveThreshold || !got[1].SmallSample {
		t.Errorf("two items are too few to flag: %+v", got[1])
	}
	if got[2].NotMineShare != nil || got[2].AboveThreshold {
		t.Errorf("rejections are not Not mine: %+v", got[2])
	}
}
