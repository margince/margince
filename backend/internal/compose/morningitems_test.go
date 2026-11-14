// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func TestTheMorningMailKeepsStoppedSourcesFocusLeavesOut(t *testing.T) {
	row := func(id string, destination crmcontracts.WorklistItemDestination) crmcontracts.WorklistItem {
		return crmcontracts.WorklistItem{Id: id, Destination: &destination}
	}
	task := row("task", crmcontracts.WorklistItemDestinationWorklistDestinationToday)
	mailbox := row("mailbox", crmcontracts.WorklistItemDestinationWorklistDestinationSystemHealth)
	day := crmcontracts.Worklist{
		Queue: []crmcontracts.WorklistItem{task, mailbox},
		Focus: &crmcontracts.WorklistFocus{Items: []crmcontracts.WorklistItem{task}},
	}
	got := morningItems(day)
	if len(got) != 2 || got[0].Id != "task" || got[1].Id != "mailbox" {
		t.Fatalf("the morning mail draws %v, want the Focus task, then the stopped mailbox", got)
	}

	day.Focus.Items = nil
	if got := morningItems(day); len(got) != 1 || got[0].Id != "mailbox" {
		t.Fatalf("a day holding only a stopped mailbox draws %v, want the mailbox", got)
	}
}
