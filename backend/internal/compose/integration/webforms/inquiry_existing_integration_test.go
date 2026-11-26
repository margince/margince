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

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

func TestSelectedExistingInquiriesHaveIndependentResultsAndDoNotEnableAPausedPolicy(t *testing.T) {
	e := setupFormsApp(t)
	form := createForm(t, e, integration.AnyMap{"name": "Inquiries", "sender": "browser", "allowed_origins": []string{siteOrigin}})
	var notes []crmcontracts.Activity
	for _, email := range []string{"selected-a@visitor.example", "selected-b@visitor.example"} {
		answer := browserPost(t, e, form.FormKey, url.Values{"email": {email}, "message": {"Please call"}}, siteOrigin)
		if answer.Status != http.StatusAccepted {
			t.Fatalf("submission: %d %s", answer.Status, answer.Body)
		}
		event := inquiryEvent(t, e, email, "activity.captured")
		var note crmcontracts.Activity
		if status := e.Call(t, "GET", "/v1/activities/"+event.Entity.ID.String(), nil, nil, &note); status != http.StatusOK || note.Version == nil {
			t.Fatalf("accepted source: %d %+v", status, note)
		}
		notes = append(notes, note)
	}
	other := createForm(t, e, integration.AnyMap{"name": "Other inquiries", "sender": "browser", "allowed_origins": []string{siteOrigin}})
	if answer := browserPost(t, e, other.FormKey, url.Values{"email": {"other-form@visitor.example"}, "message": {"Separate form"}}, siteOrigin); answer.Status != http.StatusAccepted {
		t.Fatalf("other form submission: %d", answer.Status)
	}
	var listed crmcontracts.ActivityListResponse
	if status := e.Call(t, "GET", "/v1/activities?website_form_id="+form.ID, nil, nil, &listed); status != http.StatusOK {
		t.Fatalf("accepted inquiry list: %d", status)
	}
	if len(listed.Data) != len(notes) {
		t.Fatalf("form list returned %d notes, want %d", len(listed.Data), len(notes))
	}
	for _, note := range listed.Data {
		if note.Id != notes[0].Id && note.Id != notes[1].Id {
			t.Fatalf("form list exposed another form's activity: %s", note.Id)
		}
	}
	settings := inquirySettingsBody(t, e, form)
	settings["enabled"] = false
	config, ok := settings["policy"].(integration.AnyMap)
	if !ok {
		t.Fatal("policy fixture is missing")
	}
	coordinator, ok := config["coordinator_id"].(string)
	if !ok {
		t.Fatal("coordinator fixture is missing")
	}
	config["mode"], config["routing"] = "one_colleague", integration.AnyMap{"owners": []string{coordinator}}
	var saved crmcontracts.WebsiteInquiryHandling
	if status := e.Call(t, "PUT", "/v1/web-forms/"+form.ID+"/handling", settings, map[string]string{"If-Match": strconv.FormatInt(form.Version, 10)}, &saved); status != http.StatusOK {
		t.Fatalf("save paused handling: %d", status)
	}
	selected := []integration.AnyMap{
		{"id": notes[0].Id, "version": notes[0].Version},
		{"id": notes[1].Id, "version": int64(*notes[1].Version) + 1},
		{"id": ids.NewV7(), "version": 1},
	}
	body := integration.AnyMap{"automation_id": saved.Automation.Id, "automation_version": saved.Automation.Version, "selected": selected}
	path := "/v1/web-forms/" + form.ID + "/handling/existing"
	var result crmcontracts.WebsiteInquiryHandlingResults
	if status := e.Call(t, "POST", path, body, nil, &result); status != http.StatusOK {
		t.Fatalf("handle selection: %d", status)
	}
	if len(result.Items) != 3 || result.Items[0].Status != crmcontracts.WebsiteInquiryHandlingItemStatusTaskCreated || result.Items[1].Status != crmcontracts.WebsiteInquiryHandlingItemStatusChanged || result.Items[2].Status != crmcontracts.WebsiteInquiryHandlingItemStatusUnavailable {
		t.Fatalf("independent results: %+v", result)
	}
	if result.Items[0].TaskId == nil || result.Items[0].RunId == nil || result.Items[0].Version == nil || *result.Items[0].Version <= *notes[0].Version {
		t.Fatalf("created task has no durable references: %+v", result.Items[0])
	}
	var current crmcontracts.Automation
	if status := e.Call(t, "GET", "/v1/automations/"+saved.Automation.Id.String(), nil, nil, &current); status != http.StatusOK || current.Status != "paused" {
		t.Fatalf("explicit selection enabled future handling: %d %+v", status, current)
	}
	result = crmcontracts.WebsiteInquiryHandlingResults{}
	if status := e.Call(t, "POST", path, body, nil, &result); status != http.StatusOK {
		t.Fatalf("replay selection: %d", status)
	}
	var count int
	if err := e.Owner.QueryRow(t.Context(), `SELECT count(*) FROM activity WHERE kind='task' AND source_system=@source`, pgx.NamedArgs{"source": provenance.WebsiteInquirySource}).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("selection replay created %d tasks", count)
	}
	tooMany := make([]integration.AnyMap, 51)
	for i := range tooMany {
		tooMany[i] = integration.AnyMap{"id": ids.NewV7(), "version": 1}
	}
	body["selected"] = tooMany
	if status := e.Call(t, "POST", path, body, nil, nil); status != http.StatusUnprocessableEntity {
		t.Fatalf("selection over50: %d", status)
	}
	body["selected"] = []integration.AnyMap{selected[0], selected[0]}
	if status := e.Call(t, "POST", path, body, nil, nil); status != http.StatusUnprocessableEntity {
		t.Fatalf("duplicate selection: %d", status)
	}
}
