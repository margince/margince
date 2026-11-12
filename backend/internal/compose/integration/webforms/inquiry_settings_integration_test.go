// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package webforms

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/keyvault"
)

func inquirySettingsBody(t *testing.T, e *apptest.AppEnv, form createdForm) integration.AnyMap {
	t.Helper()
	var coordinator string
	if err := e.Owner.QueryRow(t.Context(), `SELECT id::text FROM app_user WHERE email='ada@example.com'`).Scan(&coordinator); err != nil {
		t.Fatal(err)
	}
	return integration.AnyMap{"name": "Website inquiries", "enabled": true, "policy": integration.AnyMap{"form_id": form.ID, "mode": "coordinator", "routing": integration.AnyMap{}, "coordinator_id": coordinator, "task_title": "Review the website inquiry", "due_in_days": 1, "timezone": "Europe/Berlin"}}
}

func TestSavingInquiryHandlingPinsBothReviewedVersions(t *testing.T) {
	e := setupFormsApp(t)
	form := createForm(t, e, integration.AnyMap{"name": "Inquiries", "sender": "browser", "allowed_origins": []string{siteOrigin}})
	body := inquirySettingsBody(t, e, form)
	path := "/v1/web-forms/" + form.ID + "/handling"
	var saved crmcontracts.WebsiteInquiryHandling
	if status := e.Call(t, "PUT", path, body, map[string]string{"If-Match": strconv.FormatInt(form.Version, 10)}, &saved); status != http.StatusOK {
		t.Fatalf("save reviewed policy: %d", status)
	}
	if saved.Form.HandlingAutomationId == nil || *saved.Form.HandlingAutomationId != saved.Automation.Id || saved.Automation.Status != "enabled" {
		t.Fatalf("policy and form did not land together: %+v", saved)
	}
	if status := e.Call(t, "PUT", path, body, map[string]string{"If-Match": strconv.FormatInt(form.Version, 10)}, nil); status != http.StatusConflict {
		t.Fatalf("stale form review: %d", status)
	}
	body["automation_id"] = saved.Automation.Id
	body["automation_version"] = 1
	if status := e.Call(t, "PUT", path, body, map[string]string{"If-Match": strconv.FormatInt(saved.Form.Version, 10)}, nil); status != http.StatusConflict {
		t.Fatalf("stale policy review: %d", status)
	}
	var count int
	if err := e.Owner.QueryRow(t.Context(), `SELECT count(*) FROM automation WHERE key='website_inquiry'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("stale reviews created %d policies", count)
	}
}

func TestAFailedFormLinkRollsBackThePolicyAndCanRetryTheSameReview(t *testing.T) {
	e := setupFormsApp(t)
	form := createForm(t, e, integration.AnyMap{"name": "Inquiries", "sender": "browser", "allowed_origins": []string{siteOrigin}})
	body := inquirySettingsBody(t, e, form)
	if _, err := e.Owner.Exec(t.Context(), `CREATE FUNCTION refuse_handling_link() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.handling_automation_id IS NOT NULL THEN RAISE EXCEPTION 'test form link unavailable'; END IF; RETURN NEW; END $$;
CREATE TRIGGER refuse_handling_link BEFORE UPDATE ON web_form FOR EACH ROW EXECUTE FUNCTION refuse_handling_link()`); err != nil {
		t.Fatal(err)
	}
	path := "/v1/web-forms/" + form.ID + "/handling"
	headers := map[string]string{"If-Match": strconv.FormatInt(form.Version, 10)}
	if status := e.Call(t, "PUT", path, body, headers, nil); status != http.StatusInternalServerError {
		t.Fatalf("unavailable form link: %d", status)
	}
	var policies, links int
	if err := e.Owner.QueryRow(t.Context(), `SELECT count(*) FROM automation WHERE key='website_inquiry'`).Scan(&policies); err != nil {
		t.Fatal(err)
	}
	if err := e.Owner.QueryRow(t.Context(), `SELECT count(*) FROM web_form WHERE handling_automation_id IS NOT NULL`).Scan(&links); err != nil {
		t.Fatal(err)
	}
	if policies != 0 || links != 0 {
		t.Fatalf("failed review left policies=%d links=%d", policies, links)
	}
	if _, err := e.Owner.Exec(t.Context(), `DROP TRIGGER refuse_handling_link ON web_form; DROP FUNCTION refuse_handling_link()`); err != nil {
		t.Fatal(err)
	}
	if status := e.Call(t, "PUT", path, body, headers, nil); status != http.StatusOK {
		t.Fatalf("retry same review: %d", status)
	}
}

func TestInquiryRequestTypeMustBeAnActiveLeadPicklist(t *testing.T) {
	e := apptest.SetupAppWithOptions(t, compose.WithKeyvault(keyvault.NewMemory()), compose.WithSchemaPool(integration.SchemaPool(t)))
	e.BootstrapWorkspace(t)
	picklist := apptest.CreateCustomField(t, e, integration.AnyMap{"object": "lead", "label": "Request type", "type": "picklist", "options": []string{"Quote", "Support"}, "source": "manual"})
	var retired struct {
		ID     string `json:"id"`
		Column string `json:"column_name"`
	}
	if status := e.Call(t, "POST", "/v1/custom-fields", integration.AnyMap{"object": "lead", "label": "Old request type", "type": "picklist", "options": []string{"Quote", "Support"}, "source": "manual"}, nil, &retired); status != http.StatusCreated {
		t.Fatalf("create old picklist: %d", status)
	}
	if status := e.Call(t, "POST", "/v1/custom-fields/"+retired.ID+"/retire", nil, nil, nil); status != http.StatusOK {
		t.Fatalf("retire old picklist: %d", status)
	}
	text := apptest.CreateCustomField(t, e, integration.AnyMap{"object": "lead", "label": "Notes", "type": "text", "source": "manual"})
	for _, tc := range []struct {
		name, field string
		status      int
	}{
		{"picklist", picklist, http.StatusOK},
		{"free text", text, http.StatusUnprocessableEntity},
		{"missing", "cf_missing", http.StatusUnprocessableEntity},
		{"retired", retired.Column, http.StatusUnprocessableEntity},
	} {
		t.Run(tc.name, func(t *testing.T) {
			form := createForm(t, e, integration.AnyMap{"name": tc.name, "sender": "browser", "allowed_origins": []string{siteOrigin}})
			body := inquirySettingsBody(t, e, form)
			policy, ok := body["policy"].(integration.AnyMap)
			if !ok {
				t.Fatal("policy fixture is missing")
			}
			policy["request_type_field"] = tc.field
			status := e.Call(t, "PUT", "/v1/web-forms/"+form.ID+"/handling", body, map[string]string{"If-Match": strconv.FormatInt(form.Version, 10)}, nil)
			if status != tc.status {
				t.Fatalf("request type %s: got %d, want %d", tc.field, status, tc.status)
			}
		})
	}
}

func TestHandingAnEnabledInquiryRuleToARepWhoCannotCreateTasksIsRefused(t *testing.T) {
	e := setupFormsApp(t)
	e.DescribeCompany(t)
	form := createForm(t, e, integration.AnyMap{"name": "Inquiries", "sender": "browser", "allowed_origins": []string{siteOrigin}})
	var saved crmcontracts.WebsiteInquiryHandling
	if status := e.Call(t, "PUT", "/v1/web-forms/"+form.ID+"/handling", inquirySettingsBody(t, e, form),
		map[string]string{"If-Match": strconv.FormatInt(form.Version, 10)}, &saved); status != http.StatusOK {
		t.Fatalf("save inquiry handling: %d", status)
	}
	var rep crmcontracts.User
	if status := e.Call(t, "POST", "/v1/users", integration.AnyMap{"email": "no-tasks@inquiry.example", "display_name": "Rep without tasks", "role": "rep"}, nil, &rep); status != http.StatusCreated {
		t.Fatalf("invite rep: %d", status)
	}
	apptest.SetPasswordByLink(t, e, rep.Id.String(), "Inquiry-reader-2026!")
	if _, err := e.Owner.Exec(t.Context(), `
		WITH readonly AS (
		  INSERT INTO role (key, name, permissions) VALUES ('inquiry_reader', 'Inquiry reader',
		    '{"objects":{"lead":{"read":true,"update":true},"contact":{"read":true},"activity":{"read":true}},"row_scope":"all"}'::jsonb)
		  RETURNING id)
		UPDATE role_assignment SET role_id = (SELECT id FROM readonly) WHERE user_id = $1`, rep.Id); err != nil {
		t.Fatal(err)
	}

	path := "/v1/automations/" + saved.Automation.Id.String()
	handOn := func() int {
		var current crmcontracts.Automation
		if status := e.Call(t, "GET", path, nil, nil, &current); status != http.StatusOK || current.Version == nil {
			t.Fatalf("read the rule: %d", status)
		}
		return e.Call(t, "PATCH", path, integration.AnyMap{"owner_id": rep.Id},
			map[string]string{"If-Match": strconv.Itoa(*current.Version)}, nil)
	}
	if status := handOn(); status != http.StatusForbidden {
		t.Fatalf("hand on to a rep without activity:create: %d, want 403", status)
	}
	var kept crmcontracts.Automation
	if status := e.Call(t, "GET", path, nil, nil, &kept); status != http.StatusOK || kept.OwnerId == nil || saved.Automation.OwnerId == nil || *kept.OwnerId != *saved.Automation.OwnerId {
		t.Fatalf("refused hand-on moved the rule: %d owner %v, want %v", status, kept.OwnerId, saved.Automation.OwnerId)
	}

	if _, err := e.Owner.Exec(t.Context(), `UPDATE role_assignment SET role_id = (SELECT id FROM role WHERE key = 'rep') WHERE user_id = $1`, rep.Id); err != nil {
		t.Fatal(err)
	}
	if status := handOn(); status != http.StatusOK {
		t.Fatalf("hand on to a rep with the rep role: %d, want 200", status)
	}
}
