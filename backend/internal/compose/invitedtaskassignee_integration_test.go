// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// An invited colleague may be NAMED as a new task's assignee over HTTP: they
// are a real colleague who has not signed in yet, and the task waits in their
// Worklist until they do. Every automatic writer (deal check-up, lead SLA,
// email requests) calls the store directly and keeps handing work to active
// seats only, and moving an existing task onto an invited seat is routing, so
// it is refused too. Editing a task that already belongs to an invited
// colleague must still work.

import (
	"errors"
	"net/http"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestANamedInvitedColleagueMayBeGivenANewTask(t *testing.T) {
	e := integration.Setup(t)
	e.WsExec(t, `UPDATE app_user SET status = 'invited' WHERE id = $1`, e.Rep3)
	defer e.WsExec(t, `UPDATE app_user SET status = 'active' WHERE id = $1`, e.Rep3)
	invited := ids.From[ids.UserKind](e.Rep3)
	subject := "Imported follow-up"
	due := time.Now().Add(24 * time.Hour)

	assignee := openapi_types.UUID(e.Rep3)
	status, task := postActivity(e.Admin(), t, e, crmcontracts.CreateActivityRequest{
		Kind: "task", Subject: &subject, DueAt: &due, AssigneeId: &assignee,
	})
	if status != http.StatusCreated {
		t.Fatalf("POST /activities for an invited colleague answered %d, want 201", status)
	}
	if task.AssigneeId == nil || ids.UUID(*task.AssigneeId) != e.Rep3 {
		t.Fatalf("assignee = %v, want the invited colleague", task.AssigneeId)
	}

	// An edit that re-sends the unchanged invited assignee is not a reassignment.
	renamed := "Imported follow-up, renamed"
	if _, err := e.Activities.UpdateActivity(e.Admin(), ids.From[ids.ActivityKind](ids.UUID(task.Id)),
		activities.UpdateActivityInput{Subject: &renamed, AssigneeID: &invited}); err != nil {
		t.Errorf("editing a task that belongs to an invited colleague: %v", err)
	}

	// An automatic writer calls the store directly and is still refused.
	if _, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "task", Subject: &subject, DueAt: &due, AssigneeID: &invited, Source: "manual",
	}); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("an automatic task for an invited colleague got %v, want not found", err)
	}

	// Moving an existing task onto an invited colleague is routing.
	mine := ids.From[ids.UserKind](e.Rep1)
	other, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "task", Subject: &subject, DueAt: &due, AssigneeID: &mine, Source: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Activities.UpdateActivity(e.Admin(), ids.From[ids.ActivityKind](ids.UUID(other.Id)),
		activities.UpdateActivityInput{AssigneeID: &invited}); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("moving a task onto an invited colleague got %v, want not found", err)
	}

	// A suspended seat stays refused on the HTTP door too.
	e.WsExec(t, `UPDATE app_user SET status = 'suspended' WHERE id = $1`, e.Rep3)
	if status, _ := postActivity(e.Admin(), t, e, crmcontracts.CreateActivityRequest{
		Kind: "task", Subject: &subject, DueAt: &due, AssigneeId: &assignee,
	}); status == http.StatusCreated {
		t.Errorf("POST /activities for a suspended seat answered 201")
	}
}
