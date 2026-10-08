// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Four routes whose answer departed from what crm.yaml declares: an activity
// patch that could not clear a nullable field, and three refusals that used the
// wrong status or named the wrong field.

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

type activityPatchRead struct {
	ID         string  `json:"id"`
	Version    int64   `json:"version"`
	DueAt      *string `json:"due_at"`
	RemindAt   *string `json:"remind_at"`
	AssigneeID *string `json:"assignee_id"`
}

func readActivityForPatch(t *testing.T, e *apptest.AppEnv, id string) activityPatchRead {
	t.Helper()
	var got activityPatchRead
	if status := e.Call(t, "GET", "/v1/activities/"+id, nil, nil, &got); status != http.StatusOK {
		t.Fatalf("reading the activity → %d, want 200", status)
	}
	return got
}

func TestAnActivityPatchClearsANullableFieldItIsSentAsNull(t *testing.T) {
	e := apptest.SetupApp(t)
	apptest.BootstrapWorkspaceSession(t, e, "Clear fields", "clear@fable.test", "Admin")
	var me struct {
		User struct {
			ID string `json:"id"`
		} `json:"user"`
	}
	if status := e.Call(t, "GET", "/v1/me", nil, nil, &me); status != http.StatusOK {
		t.Fatalf("reading the caller → %d", status)
	}
	var task activityPatchRead
	if status := e.Call(t, "POST", "/v1/activities", AnyMap{
		"kind": "task", "subject": "Send offer", "due_at": "2026-07-08T09:00:00Z",
		"remind_at": "2026-07-07T09:00:00Z", "assignee_id": me.User.ID,
	}, nil, &task); status != http.StatusCreated {
		t.Fatalf("create task → %d", status)
	}
	if task.DueAt == nil || task.RemindAt == nil || task.AssigneeID == nil {
		t.Fatalf("the task did not start with all three set: %+v", task)
	}

	var cleared activityPatchRead
	if status := e.Call(t, "PATCH", "/v1/activities/"+task.ID,
		AnyMap{"due_at": nil, "remind_at": nil, "assignee_id": nil},
		map[string]string{"If-Match": strconv.FormatInt(task.Version, 10)}, &cleared); status != http.StatusOK {
		t.Fatalf("clearing the three fields → %d, want 200", status)
	}
	if cleared.DueAt != nil || cleared.RemindAt != nil || cleared.AssigneeID != nil {
		t.Errorf("after null the answer still holds due_at=%v remind_at=%v assignee_id=%v", cleared.DueAt, cleared.RemindAt, cleared.AssigneeID)
	}
	if again := readActivityForPatch(t, e, task.ID); again.DueAt != nil || again.RemindAt != nil || again.AssigneeID != nil {
		t.Errorf("a read after the clear still holds %+v", again)
	}
}

func TestAnActivityPatchThatChangesNothingKeepsTheVersion(t *testing.T) {
	e := apptest.SetupApp(t)
	apptest.BootstrapWorkspaceSession(t, e, "Empty patch", "empty@fable.test", "Admin")
	_, taskID := seedTaskAndTarget(t, e)
	before := readActivityForPatch(t, e, taskID)

	for _, body := range []AnyMap{{}, {"due_at": nil}} {
		var after activityPatchRead
		if status := e.Call(t, "PATCH", "/v1/activities/"+taskID, body, nil, &after); status != http.StatusOK {
			t.Fatalf("patch %v → %d, want 200", body, status)
		}
		if after.Version != before.Version {
			t.Errorf("patch %v moved the version %d → %d though nothing changed", body, before.Version, after.Version)
		}
	}
}

func TestANudgeDismissalOutsideTheRangeIsA422NamingDays(t *testing.T) {
	e := apptest.SetupApp(t)
	apptest.BootstrapWorkspaceSession(t, e, "Nudge range", "nudge@fable.test", "Admin")
	contactID, _ := seedTaskAndTarget(t, e)

	for _, body := range []AnyMap{{"days": 0}, {"days": 91}, {}} {
		var problem struct {
			Code    string `json:"code"`
			Details struct {
				Errors []struct {
					Field string `json:"field"`
				} `json:"errors"`
			} `json:"details"`
		}
		status := e.Call(t, "PUT", "/v1/contacts/"+contactID+"/nudge-dismissal", body, nil, &problem)
		if status != http.StatusUnprocessableEntity || len(problem.Details.Errors) == 0 || problem.Details.Errors[0].Field != "days" {
			t.Errorf("dismissal %v → %d %+v, want 422 naming days", body, status, problem)
		}
	}
	if status := e.Call(t, "PUT", "/v1/contacts/"+contactID+"/nudge-dismissal", AnyMap{"days": 90}, nil, nil); status != http.StatusNoContent {
		t.Fatalf("the longest span → %d, want 204", status)
	}
}

func TestAnEmptyDealRoomCredentialIs422OnBothPublicDoors(t *testing.T) {
	e := apptest.SetupApp(t)
	apptest.BootstrapWorkspaceSession(t, e, "Empty credential", "cred@fable.test", "Admin")

	for _, path := range []string{"/v1/public/rooms/peek", "/v1/public/rooms/exchange"} {
		for _, body := range []AnyMap{{"credential": ""}, {"credential": "   "}, {}} {
			if status := publicCall(t, e, "POST", path, body, nil, nil); status != http.StatusUnprocessableEntity {
				t.Errorf("%s %v → %d, want 422", path, body, status)
			}
		}
	}
}

func TestAPassportLifetimeFaultNamesTheLifetimeField(t *testing.T) {
	e := apptest.SetupApp(t)
	apptest.BootstrapWorkspaceSession(t, e, "Passport ttl", "ttl@fable.test", "Admin")

	for _, hours := range []int{0, -1, 2161} {
		var problem struct {
			Details struct {
				Errors []struct {
					Field string `json:"field"`
				} `json:"errors"`
			} `json:"details"`
		}
		status := e.Call(t, "POST", "/v1/passports", AnyMap{"scopes": []string{"read"}, "ttl_hours": hours}, nil, &problem)
		if status != http.StatusUnprocessableEntity || len(problem.Details.Errors) == 0 || problem.Details.Errors[0].Field != "ttl_hours" {
			t.Errorf("ttl_hours %d → %d %+v, want 422 naming ttl_hours", hours, status, problem)
		}
	}
}
