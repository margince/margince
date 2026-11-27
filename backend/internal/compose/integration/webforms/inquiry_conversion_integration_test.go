// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package webforms

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/url"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

func TestAWebsiteRetryKeepsTheLegacyTaskThatPredatesItsAcceptedNote(t *testing.T) {
	e := setupFormsApp(t)
	form := createForm(t, e, integration.AnyMap{"name": "Inquiries", "sender": "browser", "allowed_origins": []string{siteOrigin}})
	var legacy crmcontracts.Automation
	if status := e.Call(t, "POST", "/v1/automations", integration.AnyMap{"key": "route_lead", "name": "Legacy follow-up", "params": integration.AnyMap{}}, nil, &legacy); status != http.StatusCreated {
		t.Fatalf("create legacy: %d", status)
	}
	if status := e.Call(t, "PATCH", "/v1/automations/"+legacy.Id.String(), integration.AnyMap{"status": "enabled"}, inquiryVersion(t, legacy), nil); status != http.StatusOK {
		t.Fatalf("enable legacy: %d", status)
	}
	if _, err := e.Owner.Exec(t.Context(), `CREATE FUNCTION refuse_website_note() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.kind='note' AND NEW.source_system='web_form' THEN RAISE EXCEPTION 'test note writer unavailable'; END IF; RETURN NEW; END $$;
CREATE TRIGGER refuse_website_note BEFORE INSERT ON activity FOR EACH ROW EXECUTE FUNCTION refuse_website_note()`); err != nil {
		t.Fatal(err)
	}
	body := url.Values{"email": {"retry@visitor.example"}, "message": {"Please call"}, "_mrg_id": {"retry-across-conversion"}}
	answer := browserPost(t, e, form.FormKey, body, siteOrigin)
	if answer.Status != http.StatusInternalServerError {
		t.Fatalf("failed note was acknowledged: %d %s", answer.Status, answer.Body)
	}
	if _, err := e.Owner.Exec(t.Context(), `DROP TRIGGER refuse_website_note ON activity; DROP FUNCTION refuse_website_note()`); err != nil {
		t.Fatal(err)
	}
	if count, _, _ := leadsWithEmail(t, e, "retry@visitor.example"); count != 0 {
		t.Fatalf("the failed note left %d leads", count)
	}
	// Older versions committed the lead before its note. Seed that retained state
	// through the real writer so the conversion still covers an older retry.
	system, email := provenance.WebFormSource, body.Get("email")
	digest := sha256.Sum256([]byte(body.Get("_mrg_id")))
	key := provenance.WebFormSourceID(form.ID, hex.EncodeToString(digest[:16]))
	ctx := inquirySeatContext(t, e, ids.MustParse(apptest.SessionUserID(t, e)))
	if _, _, err := contacts.NewStore(e.DB()).CreateLead(ctx, contacts.CreateLeadInput{
		Email: &email, SourceSystem: &system, SourceID: &key, Source: system,
	}); err != nil {
		t.Fatal(err)
	}
	engine := compose.NewWorkflowEngine(e.DB())
	if err := engine.HandleEvent(t.Context(), inquiryEvent(t, e, "retry@visitor.example", "lead.created")); err != nil {
		t.Fatal(err)
	}
	var original ids.UUID
	if err := e.Owner.QueryRow(t.Context(), `SELECT id FROM activity WHERE kind='task'`).Scan(&original); err != nil {
		t.Fatal(err)
	}
	setupInquiryPolicy(t, e, form, "one_colleague")
	answer = browserPost(t, e, form.FormKey, body, siteOrigin)
	if answer.Status != http.StatusAccepted {
		t.Fatalf("retried note: %d %s", answer.Status, answer.Body)
	}
	if err := engine.HandleEvent(t.Context(), inquiryEvent(t, e, "retry@visitor.example", "activity.captured")); err != nil {
		t.Fatal(err)
	}
	var count int
	var task ids.UUID
	if err := e.Owner.QueryRow(t.Context(), `SELECT count(*),min(id::text)::uuid FROM activity WHERE kind='task'`).Scan(&count, &task); err != nil {
		t.Fatal(err)
	}
	if count != 1 || task != original {
		t.Fatalf("conversion changed legacy work: count=%d task=%s original=%s", count, task, original)
	}
	var owner *string
	if err := e.Owner.QueryRow(t.Context(), `SELECT raw->>'handling_automation_id' FROM activity WHERE kind='note' AND source_system='web_form'`).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	if owner != nil {
		t.Fatalf("retry transferred its legacy occurrence to %s", *owner)
	}
}
