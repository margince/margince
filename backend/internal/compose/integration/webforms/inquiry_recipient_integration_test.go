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
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	kevents "github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

type inquiryRecipientFixture struct {
	e         *apptest.AppEnv
	role      crmcontracts.Role
	colleague crmcontracts.User
	form      createdForm
	saved     crmcontracts.WebsiteInquiryHandling
	note      crmcontracts.Activity
	event     kevents.Envelope
}

func setupInquiryRecipient(t *testing.T) inquiryRecipientFixture {
	t.Helper()
	e := setupFormsApp(t)
	e.DescribeCompany(t)
	f := inquiryRecipientFixture{e: e}
	if status := e.Call(t, "POST", "/v1/roles", integration.AnyMap{"copy_from": "rep", "name": "Inquiry colleague"}, nil, &f.role); status != http.StatusCreated {
		t.Fatalf("create role: %d", status)
	}
	if status := e.Call(t, "POST", "/v1/users", integration.AnyMap{"email": "recipient@inquiry.example", "display_name": "Inquiry colleague", "role": f.role.Key}, nil, &f.colleague); status != http.StatusCreated {
		t.Fatalf("invite colleague: %d", status)
	}
	apptest.SetPasswordByLink(t, e, f.colleague.Id.String(), "Inquiry-recipient-2026!")
	f.form = createForm(t, e, integration.AnyMap{"name": "Inquiries", "sender": "browser", "allowed_origins": []string{siteOrigin}})
	settings := inquirySettingsBody(t, e, f.form)
	policy, ok := settings["policy"].(integration.AnyMap)
	if !ok {
		t.Fatal("policy fixture missing")
	}
	policy["mode"], policy["routing"] = "one_colleague", integration.AnyMap{"owners": []string{f.colleague.Id.String()}}
	if status := e.Call(t, "PUT", "/v1/web-forms/"+f.form.ID+"/handling", settings, map[string]string{"If-Match": strconv.FormatInt(f.form.Version, 10)}, &f.saved); status != http.StatusOK {
		t.Fatalf("save handling: %d", status)
	}
	if answer := browserPost(t, e, f.form.FormKey, url.Values{"email": {"needs-review@visitor.example"}, "message": {"Please call"}}, siteOrigin); answer.Status != http.StatusAccepted {
		t.Fatalf("submission: %d", answer.Status)
	}
	f.event = inquiryEvent(t, e, "needs-review@visitor.example", "activity.captured")
	if status := e.Call(t, "GET", "/v1/activities/"+f.event.Entity.ID.String(), nil, nil, &f.note); status != http.StatusOK {
		t.Fatalf("source: %d", status)
	}
	return f
}

func (f inquiryRecipientFixture) preview(t *testing.T) crmcontracts.WebsiteInquiryPreviewItem {
	t.Helper()
	request := integration.AnyMap{"policy": f.saved.Automation.InquiryPolicy, "automation_id": f.saved.Automation.Id, "automation_version": f.saved.Automation.Version, "selected": []integration.AnyMap{{"id": f.note.Id, "version": f.note.Version}}}
	var out crmcontracts.WebsiteInquiryHandlingPreview
	if status := f.e.Call(t, "POST", "/v1/web-forms/"+f.form.ID+"/handling/preview", request, nil, &out); status != http.StatusOK || len(out.Items) != 1 {
		t.Fatalf("preview: %d %+v", status, out)
	}
	return out.Items[0]
}

func (f inquiryRecipientFixture) revoke(t *testing.T, object string) {
	t.Helper()
	grant := integration.AnyMap{"create": false, "read": false, "update": false, "delete": false}
	if status := f.e.Call(t, "PATCH", "/v1/roles/"+f.role.Key+"/objects/"+object, grant, map[string]string{"If-Match": strconv.FormatInt(f.role.Version, 10)}, nil); status != http.StatusOK {
		t.Fatalf("revoke recipient access: %d", status)
	}
}

func TestInquiryRecipientAccessLossUsesCoordinatorWithoutAssigningLead(t *testing.T) {
	for _, object := range []string{"lead", "activity"} {
		t.Run(object, func(t *testing.T) {
			f := setupInquiryRecipient(t)
			before := f.preview(t)
			if before.RecipientId == nil || *before.RecipientId != f.colleague.Id {
				t.Fatalf("initial recipient: %+v", before)
			}
			f.revoke(t, object)
			preview := f.preview(t)
			coordinator := f.saved.Automation.InquiryPolicy.CoordinatorId
			if preview.Status != crmcontracts.WebsiteInquiryPreviewItemStatusEligible || preview.RecipientId == nil || *preview.RecipientId != coordinator || preview.Reason == nil || *preview.Reason != "recipient_no_longer_eligible" {
				t.Fatalf("preview did not explain coordinator fallback: %+v", preview)
			}
			if err := compose.NewWorkflowEngine(f.e.DB()).HandleEvent(t.Context(), f.event); err != nil {
				t.Fatal(err)
			}
			var owner *ids.UUID
			var assignee ids.UUID
			var reason string
			if err := f.e.Owner.QueryRow(t.Context(), `SELECT owner_id FROM lead WHERE email='needs-review@visitor.example'`).Scan(&owner); err != nil {
				t.Fatal(err)
			}
			if err := f.e.Owner.QueryRow(t.Context(), `SELECT assignee_id FROM activity WHERE kind='task' AND source_system=@system`, pgx.NamedArgs{"system": provenance.WebsiteInquirySource}).Scan(&assignee); err != nil {
				t.Fatal(err)
			}
			if err := f.e.Owner.QueryRow(t.Context(), `SELECT planned->>'reason' FROM automation_effect_claim WHERE effect_fingerprint='inquiry_plan'`).Scan(&reason); err != nil {
				t.Fatal(err)
			}
			if owner != nil || assignee != ids.UUID(coordinator) || reason != "recipient_no_longer_eligible" {
				t.Fatalf("fallback changed lead ownership or lost its reason: owner=%v assignee=%s reason=%s", owner, assignee, reason)
			}
		})
	}
}

func TestCompletedInquiryRemainsHandledAfterItsRecipientLosesAccess(t *testing.T) {
	for _, object := range []string{"lead", "activity"} {
		t.Run(object, func(t *testing.T) {
			f := setupInquiryRecipient(t)
			engine := compose.NewWorkflowEngine(f.e.DB())
			if err := engine.HandleEvent(t.Context(), f.event); err != nil {
				t.Fatal(err)
			}
			var tasks crmcontracts.ActivityListResponse
			if status := f.e.Call(t, "GET", "/v1/activities?source_activity_id="+f.note.Id.String(), nil, nil, &tasks); status != http.StatusOK || len(tasks.Data) != 1 {
				t.Fatalf("inquiry task: %d %+v", status, tasks)
			}
			task := tasks.Data[0]
			if task.Version == nil {
				t.Fatal("task has no version")
			}
			if status := f.e.Call(t, "PATCH", "/v1/activities/"+task.Id.String(), integration.AnyMap{"is_done": true}, map[string]string{"If-Match": strconv.FormatInt(int64(*task.Version), 10)}, nil); status != http.StatusOK {
				t.Fatalf("complete inquiry: %d", status)
			}
			f.revoke(t, object)
			preview := f.preview(t)
			if preview.Status != crmcontracts.WebsiteInquiryPreviewItemStatusAlreadyHandled || preview.RecipientId != nil || preview.DueAt != nil {
				t.Fatalf("completed inquiry was replanned: %+v", preview)
			}
			if err := engine.HandleEvent(t.Context(), f.event); err != nil {
				t.Fatal(err)
			}
			var count int
			var done bool
			if err := f.e.Owner.QueryRow(t.Context(), `SELECT count(*),bool_and(is_done) FROM activity WHERE kind='task' AND source_system=@system`, pgx.NamedArgs{"system": provenance.WebsiteInquirySource}).Scan(&count, &done); err != nil {
				t.Fatal(err)
			}
			if count != 1 || !done {
				t.Fatalf("completed inquiry was recreated: count=%d done=%v", count, done)
			}
		})
	}
}
