// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package webforms

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/identity"
	kevents "github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

func setupInquiryPolicy(t *testing.T, e *apptest.AppEnv, form createdForm, mode string) crmcontracts.Automation {
	t.Helper()
	var colleague string
	if err := e.Owner.QueryRow(t.Context(), `SELECT id::text FROM app_user WHERE email='ada@example.com'`).Scan(&colleague); err != nil {
		t.Fatal(err)
	}
	routing := integration.AnyMap{}
	if mode != "coordinator" {
		routing["owners"] = []string{colleague}
	}
	params := integration.AnyMap{"form_id": form.ID, "mode": mode, "routing": routing, "coordinator_id": colleague, "task_title": "Answer this inquiry", "due_in_days": 1, "timezone": "Europe/Berlin"}
	var policy crmcontracts.Automation
	if status := e.Call(t, "POST", "/v1/automations", integration.AnyMap{"key": provenance.WebsiteInquirySource, "name": "Website inquiries", "params": params}, nil, &policy); status != http.StatusCreated {
		t.Fatalf("create inquiry policy: %d", status)
	}
	if status := e.Call(t, "PATCH", "/v1/web-forms/"+form.ID, integration.AnyMap{"handling_automation_id": policy.Id}, map[string]string{"If-Match": strconv.FormatInt(form.Version, 10)}, nil); status != http.StatusOK {
		t.Fatalf("link form policy: %d", status)
	}
	if status := e.Call(t, "PATCH", "/v1/automations/"+policy.Id.String(), integration.AnyMap{"status": "enabled"}, inquiryVersion(t, policy), &policy); status != http.StatusOK {
		t.Fatalf("enable inquiry policy: %d", status)
	}
	return policy
}

func inquiryEvent(t *testing.T, e *apptest.AppEnv, email, kind string) kevents.Envelope {
	t.Helper()
	var raw json.RawMessage
	query := `SELECT envelope FROM event_outbox WHERE envelope->>'type'=$1 AND envelope->'entity'->>'id'=`
	if kind == "lead.created" {
		query += `(SELECT id::text FROM lead WHERE email=$2)`
	} else {
		query += `(SELECT id::text FROM activity WHERE kind='note' AND source_system='web_form' AND raw->>'email'=$2)`
	}
	if err := e.Owner.QueryRow(t.Context(), query, kind, email).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var event kevents.Envelope
	if err := json.Unmarshal(raw, &event); err != nil {
		t.Fatal(err)
	}
	return event
}

func TestAnAcceptedInquiryRoutesAndCreatesOneTaskAcrossEventIDs(t *testing.T) {
	e := setupFormsApp(t)
	form := createForm(t, e, integration.AnyMap{"name": "Inquiries", "sender": "browser", "allowed_origins": []string{siteOrigin}})
	policy := setupInquiryPolicy(t, e, form, "one_colleague")
	answer := browserPost(t, e, form.FormKey, url.Values{"email": {"new@visitor.example"}, "message": {"Please call"}, "_mrg_id": {"same-submission"}}, siteOrigin)
	if answer.Status != http.StatusAccepted {
		t.Fatalf("submission: %d %s", answer.Status, answer.Body)
	}
	event := inquiryEvent(t, e, "new@visitor.example", "activity.captured")
	engine := compose.NewWorkflowEngine(e.DB())
	if err := engine.HandleEvent(t.Context(), event); err != nil {
		t.Fatal(err)
	}
	event.EventID = ids.NewV7()
	if err := engine.HandleEvent(t.Context(), event); err != nil {
		t.Fatal(err)
	}
	var count int
	var owner, assignee, source ids.UUID
	if err := e.Owner.QueryRow(t.Context(), `SELECT count(*) FROM activity WHERE kind='task' AND source_system=$1`, provenance.WebsiteInquirySource).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("one occurrence across event IDs created %d tasks", count)
	}
	if err := e.Owner.QueryRow(t.Context(), `SELECT l.owner_id,a.assignee_id,a.source_activity_id FROM activity a JOIN activity_link x ON x.activity_id=a.id JOIN lead l ON l.id=x.lead_id WHERE a.source_system=$1`, provenance.WebsiteInquirySource).Scan(&owner, &assignee, &source); err != nil {
		t.Fatal(err)
	}
	if owner != assignee || source != event.Entity.ID {
		t.Fatalf("inquiry owner=%s task recipient=%s source=%s", owner, assignee, source)
	}
	var savedPolicy ids.UUID
	if err := e.Owner.QueryRow(t.Context(), `SELECT (planned->>'policy_id')::uuid FROM automation_effect_claim WHERE handler=$1 AND effect_fingerprint='inquiry_plan'`, provenance.WebsiteInquirySource).Scan(&savedPolicy); err != nil {
		t.Fatal(err)
	}
	if savedPolicy != ids.UUID(policy.Id) {
		t.Fatalf("assignment belongs to %s", savedPolicy)
	}
}

func TestAPausedInquiryPolicyDoesNotReactivateLegacyLeadTasks(t *testing.T) {
	e := setupFormsApp(t)
	form := createForm(t, e, integration.AnyMap{"name": "Inquiries", "sender": "browser", "allowed_origins": []string{siteOrigin}})
	policy := setupInquiryPolicy(t, e, form, "coordinator")
	if status := e.Call(t, "PATCH", "/v1/automations/"+policy.Id.String(), integration.AnyMap{"status": "paused"}, inquiryVersion(t, policy), nil); status != http.StatusOK {
		t.Fatalf("pause policy: %d", status)
	}
	var legacy crmcontracts.Automation
	if status := e.Call(t, "POST", "/v1/automations", integration.AnyMap{"key": "route_lead", "name": "Legacy follow-up", "params": integration.AnyMap{}}, nil, &legacy); status != http.StatusCreated {
		t.Fatalf("create legacy: %d", status)
	}
	if status := e.Call(t, "PATCH", "/v1/automations/"+legacy.Id.String(), integration.AnyMap{"status": "enabled"}, inquiryVersion(t, legacy), nil); status != http.StatusOK {
		t.Fatalf("enable legacy: %d", status)
	}
	answer := browserPost(t, e, form.FormKey, url.Values{"email": {"paused@visitor.example"}, "message": {"Please call"}}, siteOrigin)
	if answer.Status != http.StatusAccepted {
		t.Fatalf("submission: %d %s", answer.Status, answer.Body)
	}
	engine := compose.NewWorkflowEngine(e.DB())
	for _, kind := range []string{"lead.created", "activity.captured"} {
		if err := engine.HandleEvent(t.Context(), inquiryEvent(t, e, "paused@visitor.example", kind)); err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err := e.Owner.QueryRow(t.Context(), `SELECT count(*) FROM activity WHERE kind='task'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("paused policy let legacy create %d tasks", count)
	}
	var status string
	if err := e.Owner.QueryRow(t.Context(), `SELECT status FROM workflow_run WHERE handler='route_lead'`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "skipped" {
		t.Fatalf("legacy run=%s", status)
	}
}

func inquiryVersion(t *testing.T, policy crmcontracts.Automation) map[string]string {
	t.Helper()
	if policy.Version == nil {
		t.Fatal("the automation response has no edit version")
	}
	return map[string]string{"If-Match": strconv.Itoa(*policy.Version)}
}

func inquirySeatContext(t *testing.T, e *apptest.AppEnv, user ids.UUID) context.Context {
	t.Helper()
	workspace, err := e.DB().Workspace(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	scoped := principal.WithWorkspaceID(t.Context(), workspace.UUID)
	scoped = principal.WithCorrelationID(scoped, ids.NewV7())
	live, err := identity.NewService(e.Pool).EffectiveRBAC(scoped, workspace.UUID, user)
	if err != nil {
		t.Fatal(err)
	}
	return principal.WithActor(scoped, principal.Principal{Type: principal.PrincipalHuman, ID: principal.HumanIDPrefix + user.String(), UserID: user, Permissions: live.Permissions, TeamIDs: live.TeamIDs})
}
