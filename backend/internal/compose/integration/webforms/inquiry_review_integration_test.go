// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package webforms

import (
	"net/http"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestExistingInquiryReviewKeepsLegacyTasksAcrossTheirLifecycle(t *testing.T) {
	for _, state := range []string{"open", "done", "promoted", "outbox purged"} {
		t.Run(state, func(t *testing.T) {
			e := setupFormsApp(t)
			form := createForm(t, e, integration.AnyMap{"name": "Inquiries", "sender": "browser", "allowed_origins": []string{siteOrigin}})
			var legacy crmcontracts.Automation
			if status := e.Call(t, "POST", "/v1/automations", integration.AnyMap{"key": "route_lead", "name": "Earlier follow-up", "params": integration.AnyMap{}}, nil, &legacy); status != http.StatusCreated {
				t.Fatalf("create old handling: %d", status)
			}
			if status := e.Call(t, "PATCH", "/v1/automations/"+legacy.Id.String(), integration.AnyMap{"status": "enabled"}, inquiryVersion(t, legacy), nil); status != http.StatusOK {
				t.Fatalf("enable old handling: %d", status)
			}
			if answer := browserPost(t, e, form.FormKey, url.Values{"email": {"earlier@visitor.example"}, "message": {"Please call"}}, siteOrigin); answer.Status != http.StatusAccepted {
				t.Fatalf("submission: %d", answer.Status)
			}
			event := inquiryEvent(t, e, "earlier@visitor.example", "lead.created")
			if err := compose.NewWorkflowEngine(e.DB()).HandleEvent(t.Context(), event); err != nil {
				t.Fatal(err)
			}
			if state == "done" {
				var task ids.UUID
				var version int64
				if err := e.Owner.QueryRow(t.Context(), `SELECT id,version FROM activity WHERE kind='task'`).Scan(&task, &version); err != nil {
					t.Fatal(err)
				}
				if status := e.Call(t, "PATCH", "/v1/activities/"+task.String(), integration.AnyMap{"is_done": true}, map[string]string{"If-Match": strconv.FormatInt(version, 10)}, nil); status != http.StatusOK {
					t.Fatalf("complete task: %d", status)
				}
			}
			if state == "promoted" {
				var version int64
				if err := e.Owner.QueryRow(t.Context(), `SELECT version FROM lead WHERE id=$1`, event.Entity.ID).Scan(&version); err != nil {
					t.Fatal(err)
				}
				if status := e.Call(t, "POST", "/v1/leads/"+event.Entity.ID.String()+"/promote", integration.AnyMap{"trigger": "human_qualify"}, map[string]string{"If-Match": strconv.FormatInt(version, 10)}, nil); status != http.StatusOK {
					t.Fatalf("promote lead: %d", status)
				}
			}
			if state == "outbox purged" {
				if _, err := e.Owner.Exec(t.Context(), `DELETE FROM event_outbox WHERE envelope->>'type'='lead.created'`); err != nil {
					t.Fatal(err)
				}
			}
			policy := setupInquiryPolicy(t, e, form, "one_colleague")
			noteEvent := inquiryEvent(t, e, "earlier@visitor.example", "activity.captured")
			var note crmcontracts.Activity
			if status := e.Call(t, "GET", "/v1/activities/"+noteEvent.Entity.ID.String(), nil, nil, &note); status != http.StatusOK {
				t.Fatalf("read note: %d", status)
			}
			selected := []integration.AnyMap{{"id": note.Id, "version": note.Version}}
			var preview crmcontracts.WebsiteInquiryHandlingPreview
			if status := e.Call(t, "POST", "/v1/web-forms/"+form.ID+"/handling/preview", integration.AnyMap{"automation_id": policy.Id, "automation_version": policy.Version, "policy": policy.InquiryPolicy, "selected": selected}, nil, &preview); status != http.StatusOK {
				t.Fatalf("preview: %d", status)
			}
			if len(preview.Items) != 1 || preview.Items[0].Status != crmcontracts.WebsiteInquiryPreviewItemStatusUnavailable {
				t.Fatalf("old handling was eligible again: %+v", preview.Items)
			}
			var result crmcontracts.WebsiteInquiryHandlingResults
			if status := e.Call(t, "POST", "/v1/web-forms/"+form.ID+"/handling/existing", integration.AnyMap{"automation_id": policy.Id, "automation_version": policy.Version, "selected": selected}, nil, &result); status != http.StatusOK {
				t.Fatalf("handle old inquiry: %d", status)
			}
			var tasks int
			if err := e.Owner.QueryRow(t.Context(), `SELECT count(*) FROM activity WHERE kind='task'`).Scan(&tasks); err != nil {
				t.Fatal(err)
			}
			if tasks != 1 {
				t.Fatalf("old inquiry gained %d tasks", tasks)
			}
		})
	}
}

func TestAcceptedInquirySurvivesAnEditOrPauseBeforeItsEventIsRead(t *testing.T) {
	for _, change := range []string{"edit", "pause and resume"} {
		t.Run(change, func(t *testing.T) {
			e := setupFormsApp(t)
			form := createForm(t, e, integration.AnyMap{"name": "Inquiries", "sender": "browser", "allowed_origins": []string{siteOrigin}})
			policy := setupInquiryPolicy(t, e, form, "one_colleague")
			if answer := browserPost(t, e, form.FormKey, url.Values{"email": {"queued@visitor.example"}, "message": {"Please call"}}, siteOrigin); answer.Status != http.StatusAccepted {
				t.Fatalf("submission: %d", answer.Status)
			}
			event := inquiryEvent(t, e, "queued@visitor.example", "activity.captured")
			if change == "edit" {
				params := policy.Params
				params["task_title"] = "Review the accepted inquiry"
				if status := e.Call(t, "PATCH", "/v1/automations/"+policy.Id.String(), integration.AnyMap{"params": params}, inquiryVersion(t, policy), &policy); status != http.StatusOK {
					t.Fatalf("edit policy: %d", status)
				}
			} else {
				for _, statusValue := range []string{"paused", "enabled"} {
					if status := e.Call(t, "PATCH", "/v1/automations/"+policy.Id.String(), integration.AnyMap{"status": statusValue}, inquiryVersion(t, policy), &policy); status != http.StatusOK {
						t.Fatalf("%s policy: %d", statusValue, status)
					}
				}
			}
			var changed time.Time
			if err := e.Owner.QueryRow(t.Context(), `SELECT enabled_from FROM automation WHERE id=@id`, pgx.NamedArgs{"id": policy.Id}).Scan(&changed); err != nil {
				t.Fatal(err)
			}
			event.OccurredAt = changed.Add(-time.Second)
			if err := compose.NewWorkflowEngine(e.DB()).HandleEvent(t.Context(), event); err != nil {
				t.Fatal(err)
			}
			var tasks int
			if err := e.Owner.QueryRow(t.Context(), `SELECT count(*) FROM activity WHERE kind='task' AND source_system='website_inquiry'`).Scan(&tasks); err != nil {
				t.Fatal(err)
			}
			if tasks != 1 {
				t.Fatalf("accepted inquiry produced %d tasks after %s", tasks, change)
			}
		})
	}
}
