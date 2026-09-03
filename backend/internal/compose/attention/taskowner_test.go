// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// Who holds a task, and the row that has to say so.
//
// The lane serves three scopes and only one of them is the reader's own queue.
// An unassigned sweep and a named colleague's queue both put work on the page
// that is not the reader's — and the scope built to surface work NOBODY has
// taken produced rows that could not say that was what they were. The fact was
// on the activity the store returned and the seam dropped it.

import (
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func taskRow(assignee *ids.UUID) crmcontracts.AttentionItem {
	taskID := ids.NewV7()
	subject := "Send the retrofit quote"
	due := rankInstant.AddDate(0, 0, 2)
	item := crmcontracts.AttentionItem{
		Id:      taskID.String(),
		Source:  "task",
		Title:   &subject,
		DueAt:   &due,
		Subject: subjectOf("deal", ids.NewV7()),
		Actions: []crmcontracts.AttentionItemActions{"complete", "snooze"},
	}
	if assignee != nil {
		held := openapi_types.UUID(*assignee)
		item.AssigneeId = &held
	}
	return item
}

// The row nobody owns says so. This is the entire reason the unassigned scope
// exists, and its rows arrived indistinguishable from the reader's own.
func TestATaskNobodyHasTakenSaysNobodyOwnsIt(t *testing.T) {
	row := classifyTask(taskRow(nil), rankInstant)

	if !hasReason(row.item, "unassigned") {
		t.Fatalf("an unheld task states %v, want it to say nobody owns it", row.item.Because)
	}
}

// And a held one does NOT. A task with an owner claiming to be unowned would
// send a rep to take work a colleague is already doing, which is worse than the
// silence it replaces.
func TestAHeldTaskNeverClaimsNobodyOwnsIt(t *testing.T) {
	holder := ids.NewV7()
	row := classifyTask(taskRow(&holder), rankInstant)

	if hasReason(row.item, "unassigned") {
		t.Fatalf("a task held by somebody claims nobody owns it: %v", row.item.Because)
	}
}

// The holder reaches the wire, so a reader looking at a colleague's queue can
// be shown whose work it is rather than having to infer it from the scope they
// picked.
func TestTheHolderOfATaskReachesTheRow(t *testing.T) {
	holder := ids.NewV7()
	item := taskItem(Task{
		ID:         ids.NewV7(),
		Subject:    "Send the retrofit quote",
		DueAt:      &rankInstant,
		AssigneeID: &holder,
	}, rankInstant)

	if item.AssigneeId == nil {
		t.Fatal("the task's holder never reached the row")
	}
	if ids.UUID(*item.AssigneeId) != holder {
		t.Fatalf("the row names %v as the holder, want %v", *item.AssigneeId, holder)
	}
}

// An unheld task carries no holder rather than a zero uuid. A zero id is a
// value a client would try to resolve to a person, and it names nobody.
func TestAnUnheldTaskCarriesNoHolderRatherThanAZeroOne(t *testing.T) {
	item := taskItem(Task{
		ID:      ids.NewV7(),
		Subject: "Send the retrofit quote",
		DueAt:   &rankInstant,
	}, rankInstant)

	if item.AssigneeId != nil {
		t.Fatalf("an unheld task names %v as its holder", *item.AssigneeId)
	}
}
