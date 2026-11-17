// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

func TestATaskForADisappearedTargetIsSkipped(t *testing.T) {
	ex := Executors{Provider: &fakeReadProvider{err: apperrors.ErrNotFound}}
	effect, err := ownedTaskEffect(context.Background(), ex, workflow.Event{}, taskWords{subject: "Follow up"}, time.Time{}, nil)
	var declined *workflow.DeclinedError
	if !errors.As(err, &declined) || len(effect.Actions) != 0 {
		t.Fatalf("stale target planned work: %+v, %v", effect, err)
	}
}

func TestAnUnreadableTaskOwnerIsNotTreatedAsUnassigned(t *testing.T) {
	for name, provider := range map[string]*fakeReadProvider{
		"permission denied": {err: apperrors.ErrPermissionDenied},
		"malformed record":  {record: datasource.Record{Fields: []byte("not json")}},
	} {
		t.Run(name, func(t *testing.T) {
			effect, err := ownedTaskEffect(context.Background(), Executors{Provider: provider}, workflow.Event{}, taskWords{subject: "Follow up"}, time.Time{}, nil)
			if err == nil || len(effect.Actions) != 0 {
				t.Fatalf("unreadable owner planned an unassigned task: %+v, %v", effect, err)
			}
		})
	}
}

func TestAStageMoveTaskSaysWhichDealMovedBetweenWhichStages(t *testing.T) {
	ex := Executors{Provider: &fakeReadProvider{record: datasource.Record{Fields: []byte(`{"name":"Depot rollout"}`)}}}
	ev := workflow.Event{Payload: []byte(`{"from_stage_name":"Qualified","to_stage_name":"Proposal","to_status":"open","from_status":"open"}`)}
	effect, err := stageChangeCreateTask{ex: ex}.Plan(context.Background(), ev)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := plannedBody(t, effect), "Depot rollout moved from Qualified to Proposal."; got != want {
		t.Fatalf("body %q, want %q", got, want)
	}
	if message := effect.Actions[0].SystemText; message == nil || message.Body == nil || message.Body.Code != "automation.why.stage_moved" {
		t.Fatalf("the reason is not translatable: %+v", message)
	}
}

func TestAQuietRecordReminderNamesTheRecordAndItsLastTouch(t *testing.T) {
	ex := Executors{Provider: &fakeReadProvider{record: datasource.Record{Fields: []byte(`{"full_name":"Anna Weber"}`)}}}
	ev := workflow.Event{Payload: []byte(`{"last_activity_at":"2026-10-02T09:00:00Z"}`)}
	effect, err := noActivityReminder{ex: ex}.Plan(context.Background(), ev)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := plannedBody(t, effect), "Nothing has been logged on Anna Weber since 2026-10-02."; got != want {
		t.Fatalf("body %q, want %q", got, want)
	}
}

func TestATaskWhoseRecordHasNoNameCarriesNoStockReason(t *testing.T) {
	ex := Executors{Provider: &fakeReadProvider{record: datasource.Record{Fields: []byte(`{}`)}}}
	ev := workflow.Event{Payload: []byte(`{"from_stage_name":"Qualified","to_stage_name":"Proposal"}`)}
	effect, err := stageChangeCreateTask{ex: ex}.Plan(context.Background(), ev)
	if err != nil {
		t.Fatal(err)
	}
	if got := plannedBody(t, effect); got != "" {
		t.Fatalf("an unnamed record got the reason %q", got)
	}
}

func plannedBody(t *testing.T, effect workflow.Effect) string {
	t.Helper()
	if len(effect.Actions) != 1 {
		t.Fatalf("planned %d actions, want 1", len(effect.Actions))
	}
	var fields struct {
		Body string `json:"body"`
	}
	if err := json.Unmarshal(effect.Actions[0].Args, &fields); err != nil {
		t.Fatal(err)
	}
	return fields.Body
}
