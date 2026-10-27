// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// A row offers a verb that writes only to a reader the write would admit. The
// Worklist's checkbox for Mark done is drawn from the same `complete`, so a
// verb offered here and refused on press is a checkbox that lies too.

import (
	"slices"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestARowTheReaderMayNotChangeOffersNoVerbThatWrites(t *testing.T) {
	asOf := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	for name, tc := range map[string]struct {
		item     crmcontracts.AttentionItem
		complete bool
	}{
		"a task they may change":    {taskItem(Task{ID: ids.NewV7()}, asOf, asOf, time.UTC), true},
		"a task they may only read": {taskItem(Task{ID: ids.NewV7(), ReadOnly: true}, asOf, asOf, time.UTC), false},
		"a promise they may settle": {commitmentItem(Commitment{ID: ids.NewV7(), DueAt: asOf}, asOf), true},
		"a promise they may not":    {commitmentItem(Commitment{ID: ids.NewV7(), DueAt: asOf, ReadOnly: true}, asOf), false},
	} {
		if got := slices.Contains(tc.item.Actions, crmcontracts.AttentionItemActionsComplete); got != tc.complete {
			t.Errorf("%s: offers complete = %v, want %v (actions %v)", name, got, tc.complete, tc.item.Actions)
		}
	}
}

func TestOnlyATaskNobodyHoldsThatTheReaderMayChangeOffersTake(t *testing.T) {
	asOf := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	holder := ids.NewV7()
	for name, tc := range map[string]struct {
		task Task
		take bool
	}{
		"nobody holds it and they may change it": {Task{ID: ids.NewV7()}, true},
		"nobody holds it and they may only read": {Task{ID: ids.NewV7(), ReadOnly: true}, false},
		"a colleague holds it":                   {Task{ID: ids.NewV7(), AssigneeID: &holder}, false},
	} {
		item := taskItem(tc.task, asOf, asOf, time.UTC)
		if got := slices.Contains(item.Actions, crmcontracts.AttentionItemActionsTake); got != tc.take {
			t.Errorf("%s: offers take = %v, want %v (actions %v)", name, got, tc.take, item.Actions)
		}
	}
}

// The Worklist gates Reassign on `complete`, because a task carries it only
// when the reader may change it. The client has no other writability signal
// on a row, so this tie must not come apart.
func TestATaskCarriesCompleteExactlyWhenTheReaderMayChangeIt(t *testing.T) {
	holder := ids.NewV7()
	for name, tc := range map[string]struct {
		task     Task
		complete bool
	}{
		"unheld and changeable": {Task{ID: ids.NewV7()}, true},
		"held and changeable":   {Task{ID: ids.NewV7(), AssigneeID: &holder}, true},
		"unheld and read-only":  {Task{ID: ids.NewV7(), ReadOnly: true}, false},
		"held and read-only":    {Task{ID: ids.NewV7(), AssigneeID: &holder, ReadOnly: true}, false},
	} {
		if got := slices.Contains(taskActions(tc.task), crmcontracts.AttentionItemActionsComplete); got != tc.complete {
			t.Errorf("%s: carries complete = %v, want %v", name, got, tc.complete)
		}
	}
}
