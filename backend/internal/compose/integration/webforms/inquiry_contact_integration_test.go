// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package webforms

import (
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

func TestAContactInquiryReusesTheImmediateTaskAndKeepsCompletedWorkClosed(t *testing.T) {
	for _, completed := range []bool{false, true} {
		name := "open"
		if completed {
			name = "already completed"
		}
		t.Run(name, func(t *testing.T) {
			e := setupFormsApp(t)
			var colleague string
			if err := e.Owner.QueryRow(t.Context(), `SELECT id::text FROM app_user WHERE email='ada@example.com'`).Scan(&colleague); err != nil {
				t.Fatal(err)
			}
			if status := e.Call(t, "POST", "/v1/contacts", integration.AnyMap{"full_name": "Contact", "emails": []integration.AnyMap{{"email": "contact@visitor.example", "email_type": "work", "is_primary": true}}, "owner_id": colleague, "source": "manual"}, nil, nil); status != http.StatusCreated {
				t.Fatalf("create contact: %d", status)
			}
			form := createForm(t, e, integration.AnyMap{"name": "Inquiries", "sender": "browser", "allowed_origins": []string{siteOrigin}})
			setupInquiryPolicy(t, e, form, "one_colleague")
			answer := browserPost(t, e, form.FormKey, url.Values{"email": {"contact@visitor.example"}, "message": {"Another request"}}, siteOrigin)
			if answer.Status != http.StatusAccepted {
				t.Fatalf("submission: %d %s", answer.Status, answer.Body)
			}
			var task ids.UUID
			if err := e.Owner.QueryRow(t.Context(), `SELECT id FROM activity WHERE kind='task' AND source_system=$1`, provenance.LeadForContactSource).Scan(&task); err != nil {
				t.Fatal(err)
			}
			if completed {
				if status := e.Call(t, "PATCH", "/v1/activities/"+task.String(), integration.AnyMap{"is_done": true}, nil, nil); status != http.StatusOK {
					t.Fatalf("complete immediate task: %d", status)
				}
			}
			event := inquiryEvent(t, e, "contact@visitor.example", "activity.captured")
			if err := compose.NewWorkflowEngine(e.DB()).HandleEvent(t.Context(), event); err != nil {
				t.Fatal(err)
			}
			var count int
			if err := e.Owner.QueryRow(t.Context(), `SELECT count(*) FROM activity WHERE kind='task'`).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 1 {
				t.Fatalf("accepted inquiry has %d tasks", count)
			}
			var source *ids.UUID
			var due *time.Time
			var done bool
			if err := e.Owner.QueryRow(t.Context(), `SELECT source_activity_id,due_at,is_done FROM activity WHERE id=$1`, task).Scan(&source, &due, &done); err != nil {
				t.Fatal(err)
			}
			if done != completed {
				t.Fatalf("task completion changed: %v", done)
			}
			if !completed && (source == nil || *source != event.Entity.ID || due == nil) {
				t.Fatalf("reused task has source=%v due=%v", source, due)
			}
		})
	}
}
