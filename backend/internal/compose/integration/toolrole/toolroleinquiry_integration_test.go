// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package toolrole

import (
	"strconv"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

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
