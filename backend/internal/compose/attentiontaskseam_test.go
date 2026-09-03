// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What survives the crossing from a stored activity to a queue task.
//
// The seam's failure mode is silent by construction: a field it stops copying
// costs no rows and raises no error, so the lane still returns the right NUMBER
// of tasks and each one is quietly missing a fact. The assignee is the case
// that made this worth a test — dropped, every held task reaches the reader
// labelled as one nobody has taken, which is the opposite of true and reads as
// an invitation to pick up a colleague's work.

import (
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func storedTask(assignee *ids.UUID, links *[]crmcontracts.ActivityLink) crmcontracts.Activity {
	subject := "Send the retrofit quote"
	due := time.Date(2026, 6, 20, 9, 0, 0, 0, time.UTC)
	row := crmcontracts.Activity{
		Id:      openapi_types.UUID(ids.NewV7()),
		Subject: &subject,
		DueAt:   &due,
		Links:   links,
	}
	if assignee != nil {
		held := openapi_types.UUID(*assignee)
		row.AssigneeId = &held
	}
	return row
}

// The holder crosses. Two of the lane's three scopes put somebody else's task
// in front of the reader, so who owns it is not decoration.
func TestTheHolderOfATaskCrossesTheSeam(t *testing.T) {
	holder := ids.NewV7()
	task := taskFromActivity(storedTask(&holder, nil))

	if task.AssigneeID == nil {
		t.Fatal("the stored assignee did not reach the queue task — every held task then " +
			"reaches the reader labelled as one nobody has taken")
	}
	if *task.AssigneeID != holder {
		t.Fatalf("the task names %v as its holder, want %v", *task.AssigneeID, holder)
	}
}

// An unheld task crosses as unheld rather than as a zero id, which is a value a
// client would try to resolve to a person who does not exist.
func TestAnUnheldTaskCrossesTheSeamCarryingNoHolder(t *testing.T) {
	if task := taskFromActivity(storedTask(nil, nil)); task.AssigneeID != nil {
		t.Fatalf("an unheld task crossed naming %v as its holder", *task.AssigneeID)
	}
}

// The rest of the crossing, asserted beside the assignee so a future edit that
// drops one of these is caught by the same test rather than by nothing.
func TestATaskCrossesTheSeamWithTheFactsTheRowNeeds(t *testing.T) {
	deal := ids.NewV7()
	row := storedTask(nil, &[]crmcontracts.ActivityLink{
		{EntityType: flipObjectDeal, EntityId: openapi_types.UUID(deal)},
	})

	task := taskFromActivity(row)

	if task.ID != ids.UUID(row.Id) {
		t.Errorf("the task's id is %v, want the activity's %v", task.ID, row.Id)
	}
	if task.Subject != "Send the retrofit quote" {
		t.Errorf("the task's subject is %q, want the activity's", task.Subject)
	}
	if task.DueAt == nil || !task.DueAt.Equal(*row.DueAt) {
		t.Errorf("the task is due %v, want the activity's %v", task.DueAt, row.DueAt)
	}
	// The record the task is FOR. Without it the row is a sentence the reader
	// cannot act on, and the lane knew which deal it meant all along.
	if task.LinkType != string(flipObjectDeal) || task.LinkID != deal {
		t.Errorf("the task is filed under %s/%v, want the deal %v", task.LinkType, task.LinkID, deal)
	}
}
