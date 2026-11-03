// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// claimOverHTTP sends one claim through the handler a browser tab calls.
func claimOverHTTP(ctx context.Context, t *testing.T, store *Store, task ids.ActivityID, body string) (int, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/activities/"+task.String()+"/reminder-delivery",
		strings.NewReader(body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	Handlers{store: store}.ClaimTaskReminderDelivery(rec, req, crmcontracts.Id(task.UUID))
	return rec.Code, rec.Body.String()
}

// asMember is one member reading activities, as a browser tab of theirs does.
func asMember(e *sendEnv, member ids.UUID) context.Context {
	ctx := principal.WithCorrelationID(principal.WithWorkspaceID(context.Background(), e.ws), ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + member.String(), UserID: member,
		Permissions: principal.Permissions{
			RoleKeys: []string{"rep"},
			Objects:  map[string]principal.ObjectGrant{"activity": {Read: true, Create: true, Update: true}},
			RowScope: principal.RowScopeAll,
		},
	})
}

// A member signed in on two browsers sees a due reminder once. The first tab
// to claim it shows it; a moved or handed-on reminder can be claimed again.
func TestADueReminderIsClaimedOncePerMemberAndAgainOnceMoved(t *testing.T) {
	e := setupSend(t)
	now := clocktest.Now(t)
	store := pinStore(e).WithClock(func() time.Time { return now })
	rep := asMember(e, e.rep)
	due := now.Add(-time.Minute).Truncate(time.Microsecond)
	assignee := ids.From[ids.UserKind](e.rep)
	created, _, err := store.LogActivity(rep, LogActivityInput{
		Kind: "task", Subject: new("Call back"), Source: "manual", RemindAt: &due, AssigneeID: &assignee,
	})
	if err != nil {
		t.Fatal(err)
	}
	task := ids.From[ids.ActivityKind](ids.UUID(created.Id))
	claim := func(ctx context.Context, at time.Time) bool {
		t.Helper()
		code, out := claimOverHTTP(ctx, t, store, task, fmt.Sprintf(`{"remind_at": %q}`, at.Format(time.RFC3339Nano)))
		if code != http.StatusOK {
			t.Fatalf("claim answered %d: %s", code, out)
		}
		var answer crmcontracts.TaskReminderClaim
		if err := json.Unmarshal([]byte(out), &answer); err != nil {
			t.Fatal(err)
		}
		return answer.Claimed
	}

	for body, want := range map[string]int{`{}`: http.StatusUnprocessableEntity, `{"remind_at":`: http.StatusUnprocessableEntity} {
		if code, out := claimOverHTTP(rep, t, store, task, body); code != want {
			t.Errorf("claim %s answered %d (%s), want %d", body, code, out, want)
		}
	}

	if !claim(rep, due) {
		t.Fatal("the first tab's claim was refused")
	}
	if claim(rep, due) {
		t.Error("a second tab claimed a reminder the first already showed")
	}
	if claim(asMember(e, e.other), due) {
		t.Error("a colleague claimed a reminder on a task that is not theirs")
	}

	moved := due.Add(-time.Second)
	if _, err := store.UpdateActivity(rep, task, UpdateActivityInput{RemindAt: &moved}); err != nil {
		t.Fatal(err)
	}
	if claim(rep, due) {
		t.Error("the old reminder instant was claimed after the reminder moved")
	}
	if !claim(rep, moved) {
		t.Error("the moved reminder could not be claimed again")
	}

	colleague := ids.From[ids.UserKind](e.other)
	if _, err := store.UpdateActivity(rep, task, UpdateActivityInput{AssigneeID: &colleague}); err != nil {
		t.Fatal(err)
	}
	if !claim(asMember(e, e.other), moved) {
		t.Error("the new assignee could not claim a reminder its old assignee had shown")
	}
	if claim(rep, moved) {
		t.Error("the old assignee claimed a reminder on a task handed away")
	}
}

// The write re-reads the task under a lock. A list read that a handover beat
// to commit cannot turn into a claim for the member who no longer holds it.
func TestAReminderClaimBehindAHandoverRecordsNothing(t *testing.T) {
	e := setupSend(t)
	now := clocktest.Now(t)
	store := pinStore(e).WithClock(func() time.Time { return now })
	rep := asMember(e, e.rep)
	due := now.Add(-time.Minute).Truncate(time.Microsecond)
	assignee := ids.From[ids.UserKind](e.rep)
	created, _, err := store.LogActivity(rep, LogActivityInput{
		Kind: "task", Subject: new("Call back"), Source: "manual", RemindAt: &due, AssigneeID: &assignee,
	})
	if err != nil {
		t.Fatal(err)
	}
	task := ids.From[ids.ActivityKind](ids.UUID(created.Id))
	colleague := ids.From[ids.UserKind](e.other)
	if _, err := store.UpdateActivity(rep, task, UpdateActivityInput{AssigneeID: &colleague}); err != nil {
		t.Fatal(err)
	}

	claimed, err := store.recordReminderClaim(rep, task, due)
	if err != nil {
		t.Fatal(err)
	}
	if claimed {
		t.Error("a claim was recorded for a member the task was handed away from")
	}
}
