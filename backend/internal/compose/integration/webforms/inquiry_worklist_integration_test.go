// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package webforms

import (
	"net/http"
	"net/url"
	"strconv"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

func TestWebsiteInquiryWorklistShowsAcceptedWorkBeforeItsDeadline(t *testing.T) {
	for _, days := range []int{1, 30} {
		t.Run(strconv.Itoa(days), func(t *testing.T) {
			e := setupFormsApp(t)
			form := createForm(t, e, integration.AnyMap{"name": "Inquiries", "sender": "browser", "allowed_origins": []string{siteOrigin}})
			policy := setupInquiryPolicy(t, e, form, "one_colleague")
			params := policy.Params
			params["due_in_days"] = days
			if status := e.Call(t, "PATCH", "/v1/automations/"+policy.Id.String(), integration.AnyMap{"params": params}, inquiryVersion(t, policy), &policy); status != http.StatusOK {
				t.Fatalf("set deadline: %d", status)
			}
			if answer := browserPost(t, e, form.FormKey, url.Values{"email": {"worklist@visitor.example"}, "message": {"Please call"}}, siteOrigin); answer.Status != http.StatusAccepted {
				t.Fatalf("submission: %d", answer.Status)
			}
			event := inquiryEvent(t, e, "worklist@visitor.example", "activity.captured")
			if err := compose.NewWorkflowEngine(e.DB()).HandleEvent(t.Context(), event); err != nil {
				t.Fatal(err)
			}
			var task crmcontracts.Activity
			var taskID ids.UUID
			if err := e.Owner.QueryRow(t.Context(), `SELECT id FROM activity WHERE kind='task' AND source_system=@system`, pgx.NamedArgs{"system": provenance.WebsiteInquirySource}).Scan(&taskID); err != nil {
				t.Fatal(err)
			}
			if status := e.Call(t, "GET", "/v1/activities/"+taskID.String(), nil, nil, &task); status != http.StatusOK {
				t.Fatalf("task: %d", status)
			}
			if task.DueAt == nil || task.Version == nil {
				t.Fatal("inquiry task has no deadline or version")
			}
			if status := e.Call(t, "POST", "/v1/activities", integration.AnyMap{"kind": "task", "subject": "Unrelated future work", "occurred_at": task.CreatedAt, "due_at": task.DueAt, "assignee_id": task.AssigneeId, "source": "manual"}, nil, nil); status != http.StatusCreated {
				t.Fatalf("unrelated task: %d", status)
			}
			path := "/v1/worklist?scope=all&view=today"
			var selected crmcontracts.Worklist
			if status := e.Call(t, "GET", path+"&filter=website_inquiries", nil, nil, &selected); status != http.StatusOK {
				t.Fatalf("website inquiry filter: %d", status)
			}
			if len(selected.Queue) != 1 || selected.Queue[0].Id != taskID.String() || selected.Queue[0].Kind == nil || *selected.Queue[0].Kind != provenance.WebsiteInquirySource {
				t.Fatalf("website filter returned other work: %+v", selected.Queue)
			}
			if !task.DueAt.After(selected.AsOf) {
				t.Fatalf("fixture does not exercise work before its deadline: due=%s as_of=%s", task.DueAt, selected.AsOf)
			}
			var today crmcontracts.Worklist
			if status := e.Call(t, "GET", path, nil, nil, &today); status != http.StatusOK {
				t.Fatalf("today: %d", status)
			}
			visible := false
			for _, item := range today.Queue {
				visible = visible || item.Id == taskID.String()
			}
			if !visible {
				t.Fatalf("accepted inquiry is hidden until its deadline: %+v", today.Queue)
			}
			var upcoming crmcontracts.Worklist
			if status := e.Call(t, "GET", path+"&filter=upcoming", nil, nil, &upcoming); status != http.StatusOK {
				t.Fatalf("upcoming: %d", status)
			}
			for _, item := range upcoming.Queue {
				if item.Id == taskID.String() {
					t.Fatal("accepted inquiry was listed a second time as future work")
				}
			}
			if status := e.Call(t, "PATCH", "/v1/activities/"+taskID.String(), integration.AnyMap{"is_done": true}, map[string]string{"If-Match": strconv.FormatInt(int64(*task.Version), 10)}, nil); status != http.StatusOK {
				t.Fatalf("mark handled: %d", status)
			}
			selected = crmcontracts.Worklist{}
			if status := e.Call(t, "GET", path+"&filter=website_inquiries", nil, nil, &selected); status != http.StatusOK || len(selected.Queue) != 0 {
				t.Fatalf("handled inquiry remains open: %d %+v", status, selected.Queue)
			}
		})
	}
}
