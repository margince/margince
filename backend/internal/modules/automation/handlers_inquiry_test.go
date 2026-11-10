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
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

type inquiryRecordReader struct {
	datasource.SystemOfRecordProvider
	owner   ids.UUID
	records map[datasource.EntityRef]datasource.Record
}

func (r inquiryRecordReader) Read(ctx context.Context, ref datasource.EntityRef) (datasource.Record, error) {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalHuman || actor.UserID != r.owner {
		return datasource.Record{}, apperrors.ErrPermissionDenied
	}
	row, ok := r.records[ref]
	if !ok {
		return datasource.Record{}, apperrors.ErrNotFound
	}
	return row, nil
}

func inquiryPlannerFixture(t *testing.T) (workflow.Event, map[string]any, datasource.EntityRef) {
	t.Helper()
	params, err := json.Marshal(inquiryPolicyFixture())
	if err != nil {
		t.Fatal(err)
	}
	policy, err := ParseInquiryPolicy(params)
	if err != nil {
		t.Fatal(err)
	}
	key := policy.FormId.String() + ":accepted-key"
	payload, err := json.Marshal(map[string]any{"kind": "note", "source_system": provenance.WebFormSource, "website_submission_key": key})
	if err != nil {
		t.Fatal(err)
	}
	event := workflow.Event{ID: ids.NewV7(), WorkspaceID: ids.NewV7(), AutomationID: ids.NewV7(), OwnerID: ids.NewV7(), Params: params, Payload: payload, Entity: datasource.EntityRef{Type: datasource.EntityActivity, ID: ids.NewV7()}}
	target := datasource.EntityRef{Type: datasource.EntityLead, ID: ids.NewV7()}
	note := map[string]any{"id": event.Entity.ID, "kind": "note", "source_system": provenance.WebFormSource, "source_id": key, "created_at": time.Date(2026, 3, 28, 15, 0, 0, 0, time.UTC), "raw": map[string]any{"handling_automation_id": event.AutomationID}, "links": []map[string]any{{"entity_type": "lead", "entity_id": target.ID}}}
	return event, note, target
}

func TestInquiryPlanningRequiresTheActualAcceptedNoteAndItsSavedOwner(t *testing.T) {
	cases := []struct {
		name      string
		change    func(map[string]any)
		wantError bool
	}{
		{"accepted", func(map[string]any) {}, false},
		{"ordinary note", func(note map[string]any) { note["source_system"] = "manual" }, true},
		{"email", func(note map[string]any) { note["kind"] = "email" }, true},
		{"different submission", func(note map[string]any) { note["source_id"] = "other-form:other-request" }, true},
		{"legacy receipt", func(note map[string]any) { note["raw"] = map[string]any{} }, true},
		{"different policy", func(note map[string]any) { note["raw"] = map[string]any{"handling_automation_id": ids.NewV7()} }, true},
		{"no content", func(note map[string]any) { delete(note, "raw") }, true},
		{"archived", func(note map[string]any) { note["archived_at"] = time.Date(2026, 3, 29, 8, 0, 0, 0, time.UTC) }, true},
		{"no target", func(note map[string]any) { note["links"] = []map[string]any{} }, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			event, note, target := inquiryPlannerFixture(t)
			tc.change(note)
			raw, err := json.Marshal(note)
			if err != nil {
				t.Fatal(err)
			}
			provider := inquiryRecordReader{owner: event.OwnerID, records: map[datasource.EntityRef]datasource.Record{event.Entity: {Ref: event.Entity, Fields: raw}, target: {Ref: target, Fields: json.RawMessage(`{}`)}}}
			handler := websiteInquiry{ex: Executors{Provider: provider, Authority: &fakeAuthzResolver{seat: principal.SeatFull}}}
			plan, err := handler.Plan(t.Context(), event)
			if tc.wantError {
				if err == nil {
					t.Fatal("planned work without an accepted owned receipt")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(plan.Actions) != 1 || plan.Actions[0].Kind != workflow.ActionCreateTask || plan.Actions[0].Target != target {
				t.Fatalf("inquiry plan=%+v", plan)
			}
			var task struct {
				DueAt  time.Time `json:"due_at"`
				Source ids.UUID  `json:"source_activity_id"`
				System string    `json:"source_system"`
			}
			if err := json.Unmarshal(plan.Actions[0].Args, &task); err != nil {
				t.Fatal(err)
			}
			wantDue := time.Date(2026, 3, 29, 21, 59, 59, 999999000, time.UTC)
			if task.Source != event.Entity.ID || task.System != provenance.WebsiteInquirySource || !task.DueAt.Equal(wantDue) {
				t.Fatalf("planned source or calendar deadline=%+v", task)
			}
		})
	}
}

func TestAnInquiryOccurrenceDoesNotDependOnTheBusEventOrPolicyVersion(t *testing.T) {
	event, _, _ := inquiryPlannerFixture(t)
	handler := websiteInquiry{}
	first := handler.IdempotencyKey(event)
	event.ID = ids.NewV7()
	event.Params = json.RawMessage(`{"version":2}`)
	if second := handler.IdempotencyKey(event); first != second {
		t.Fatalf("redelivery changed occurrence: %s vs %s", first, second)
	}
	var payload map[string]any
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	payload["website_submission_key"] = "a-different-accepted-request"
	var err error
	event.Payload, err = json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if handler.IdempotencyKey(event) == first {
		t.Fatal("two accepted transport identities share an occurrence")
	}
}

func TestAnInquiryWithNoLiveAuthorizerCannotReadThroughTheSystemActor(t *testing.T) {
	event, _, _ := inquiryPlannerFixture(t)
	handler := websiteInquiry{ex: Executors{Provider: inquiryRecordReader{}, Authority: &fakeAuthzResolver{err: apperrors.ErrNotFound}}}
	ctx := principal.SystemActing(t.Context(), "system:automation")
	if _, err := handler.Plan(ctx, event); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("gone authorizer planned under system authority: %v", err)
	}
}
