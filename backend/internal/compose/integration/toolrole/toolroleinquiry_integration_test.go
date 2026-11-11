// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package toolrole

import (
	"slices"
	"strconv"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestInquiryPreviewRequiresPermissionToCreateTheWorkItPlans(t *testing.T) {
	recorded := newToolCheckRecorder()
	const slug = "inquiry-preview-authority"
	d := toolRoleApp(t, slug, compose.WithToolChecksRecordedForTest(recorded.note))
	var calls []toolRoleCall
	for _, call := range toolRoleCalls(compose.NewRegistry(nil, compose.SendPath{}).Specs(), routeShapes()) {
		if call.verb == "preview_website_inquiries" {
			calls = append(calls, call)
		}
	}
	if len(calls) != 1 {
		t.Fatalf("preview calls = %d", len(calls))
	}
	pool := seedToolRoleRecords(t, d.E, calls, "")
	args := calls[0].args(pool)
	human := apptest.SessionUserID(t, d.E)
	apptest.AssignRoleTo(t, d.E, slug+"@fable.test", "read_only")
	got := d.MCP.Call(t, calls[0].verb, args)
	run, reached := recorded.take(calls[0].verb, human)
	if !reached || apptest.VerdictOf(got, run.Refused) != apptest.VerdictRefused || !slices.Contains(run.Refused, "activity.create") {
		t.Fatalf("preview refusal = %s, recorded = %+v, reached = %v", got.Text, run, reached)
	}
}

func seedExistingWebsiteInquiry(s sweepSeed) string {
	var form crmcontracts.WebForm
	s.call("POST", "/v1/web-forms", integration.AnyMap{"name": uniqueName(), "sender": "browser", "allowed_origins": []string{"https://example.test"}}, nil, &form)
	s.call("POST", "/v1/public/forms/"+form.FormKey, integration.AnyMap{"email": ids.NewV7().String() + "@visitor.example", "message": "Please call"}, map[string]string{"Origin": "https://example.test"}, nil)
	var sources crmcontracts.ActivityListResponse
	s.call("GET", "/v1/activities?website_form_id="+form.Id.String(), nil, nil, &sources)
	if len(sources.Data) != 1 || sources.Data[0].Version == nil {
		s.t.Fatalf("accepted inquiry fixture has no versioned source: %+v", sources)
	}
	source := sources.Data[0]
	var saved crmcontracts.WebsiteInquiryHandling
	s.call("PUT", "/v1/web-forms/"+form.Id.String()+"/handling", integration.AnyMap{
		"name": "Review old website inquiries", "enabled": false, "policy": integration.AnyMap{
			"form_id": form.Id, "mode": "coordinator", "routing": integration.AnyMap{}, "coordinator_id": apptest.SessionUserID(s.t, s.e), "task_title": "Review the inquiry", "due_in_days": 1, "timezone": "Etc/UTC",
		},
	}, map[string]string{"If-Match": strconv.FormatInt(int64(form.Version), 10)}, &saved)
	id := form.Id.String()
	s.pool.note(id, "policy", saved.Automation.Id.String())
	if saved.Automation.Version == nil {
		s.t.Fatal("inquiry fixture policy has no version")
	}
	s.pool.note(id, "version", strconv.FormatInt(int64(*saved.Automation.Version), 10))
	s.pool.note(id, "source", source.Id.String())
	s.pool.note(source.Id.String(), "version", strconv.FormatInt(int64(*source.Version), 10))
	return id
}

func seedAssignableWebsiteInquiry(s sweepSeed) string {
	form := seedExistingWebsiteInquiry(s)
	source := s.pool.fact(form, "source")
	var handled crmcontracts.WebsiteInquiryHandlingResults
	s.call("POST", "/v1/web-forms/"+form+"/handling/existing", integration.AnyMap{
		"automation_id": s.pool.fact(form, "policy"), "automation_version": versionOf(s.pool, form),
		"selected": []integration.AnyMap{{"id": source, "version": versionOf(s.pool, source)}},
	}, nil, &handled)
	if len(handled.Items) != 1 || handled.Items[0].TaskId == nil {
		s.t.Fatalf("inquiry fixture has no task: %+v", handled)
	}
	id := handled.Items[0].TaskId.String()
	recipient := apptest.SessionUserID(s.t, s.e)
	var preview crmcontracts.WebsiteInquiryAssignment
	s.call("POST", "/v1/activities/"+id+"/inquiry-assignment/preview", integration.AnyMap{"mode": "assign_and_follow_up", "recipient_id": recipient}, nil, &preview)
	if preview.Task.Version == nil || preview.RecordVersion == nil {
		s.t.Fatalf("assignment preview has no reviewed versions: %+v", preview)
	}
	// Agents cannot claim unowned leads; this fixture exercises an owned inquiry.
	s.call("PATCH", "/v1/leads/"+preview.RecordId.String(), integration.AnyMap{"owner_id": recipient}, map[string]string{"If-Match": strconv.FormatInt(int64(*preview.RecordVersion), 10)}, nil)
	s.call("POST", "/v1/activities/"+id+"/inquiry-assignment/preview", integration.AnyMap{"mode": "assign_and_follow_up", "recipient_id": recipient}, nil, &preview)
	s.pool.note(id, "version", strconv.FormatInt(int64(*preview.Task.Version), 10))
	s.pool.note(id, "record", preview.RecordId.String())
	s.pool.note(preview.RecordId.String(), "version", strconv.FormatInt(int64(*preview.RecordVersion), 10))
	s.pool.note(id, "record_type", string(preview.RecordType))
	s.pool.note(id, "recipient", recipient)
	return id
}

//craft:ignore naked-any the fixture supplies the JSON fields declared by the route's schema
func inquiryAssignmentArgs(p *recordPool, apply bool) map[string]any {
	id := p.take("assignable_website_inquiry")
	args := map[string]any{"id": id, "request_body": map[string]any{
		"mode": "assign_and_follow_up", "recipient_id": p.fact(id, "recipient"),
		"record_id": p.fact(id, "record"), "record_type": p.fact(id, "record_type"), "record_version": versionOf(p, p.fact(id, "record")),
	}}
	if apply {
		args["if_version"] = versionOf(p, id)
	}
	return args
}

func TestSelectedWebsiteInquiryToolUsesTheSavedPausedPolicy(t *testing.T) {
	d := toolRoleApp(t, "selected-website-inquiry")
	pool := &recordPool{fresh: map[string][]string{}, facts: map[string]map[string]string{}}
	form := seedExistingWebsiteInquiry(sweepSeed{t: t, e: d.E, pool: pool})
	pool.fresh["existing_website_inquiry"] = []string{form}
	args := setupFixtures["handle_website_inquiries"](pool)
	got, refused := callThroughApproval(t, d.E, d.MCP, "handle_website_inquiries", args)
	if got.IsError || len(refused) != 0 {
		t.Fatalf("selected inquiry tool refused: %v %s", refused, got.Text)
	}
	var result crmcontracts.WebsiteInquiryHandlingResults
	got.JSON(t, &result)
	if len(result.Items) != 1 || result.Items[0].Status != crmcontracts.WebsiteInquiryHandlingItemStatusTaskCreated || result.Items[0].TaskId == nil {
		t.Fatalf("selected inquiry tool did not create its task: %+v", result)
	}
	var policy crmcontracts.Automation
	sweepSeed{t: t, e: d.E, pool: pool}.call("GET", "/v1/automations/"+pool.fact(form, "policy"), nil, nil, &policy)
	if policy.Status != "paused" {
		t.Fatalf("selected inquiry tool enabled the paused rule: %s", policy.Status)
	}
}
