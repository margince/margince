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

func TestAnInquiryTaskFailureKeepsItsAssignmentAndOriginalTaskAcrossPolicyEdits(t *testing.T) {
	e := setupFormsApp(t)
	form := createForm(t, e, integration.AnyMap{"name": "Inquiries", "sender": "browser", "allowed_origins": []string{siteOrigin}})
	policy := setupInquiryPolicy(t, e, form, "one_colleague")
	answer := browserPost(t, e, form.FormKey, url.Values{"email": {"recover@visitor.example"}, "message": {"Please call"}}, siteOrigin)
	if answer.Status != http.StatusAccepted {
		t.Fatalf("submission: %d %s", answer.Status, answer.Body)
	}
	if _, err := e.Owner.Exec(t.Context(), `CREATE FUNCTION refuse_inquiry_task() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.kind='task' AND NEW.source_system='website_inquiry' THEN RAISE EXCEPTION 'test task writer unavailable'; END IF; RETURN NEW; END $$;
CREATE TRIGGER refuse_inquiry_task BEFORE INSERT ON activity FOR EACH ROW EXECUTE FUNCTION refuse_inquiry_task()`); err != nil {
		t.Fatal(err)
	}
	event := inquiryEvent(t, e, "recover@visitor.example", "activity.captured")
	if err := compose.NewWorkflowEngine(e.DB()).HandleEvent(t.Context(), event); err == nil {
		t.Fatal("the unavailable task writer did not fail the firing")
	}
	var owner ids.UUID
	if err := e.Owner.QueryRow(t.Context(), `SELECT owner_id FROM lead WHERE email='recover@visitor.example'`).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	var run ids.UUID
	var version int64
	var due time.Time
	if err := e.Owner.QueryRow(t.Context(), `SELECT id FROM workflow_run WHERE handler=$1 AND status='failed'`, provenance.WebsiteInquirySource).Scan(&run); err != nil {
		t.Fatal(err)
	}
	if err := e.Owner.QueryRow(t.Context(), `SELECT (planned->>'policy_version')::bigint,(planned->>'due_at')::timestamptz FROM automation_effect_claim WHERE handler=$1 AND effect_fingerprint='inquiry_plan'`, provenance.WebsiteInquirySource).Scan(&version, &due); err != nil {
		t.Fatal(err)
	}
	params := policy.Params
	params["task_title"] = "A later policy's different wording"
	params["due_in_days"] = 10
	if status := e.Call(t, "PATCH", "/v1/automations/"+policy.Id.String(), integration.AnyMap{"params": params}, inquiryVersion(t, policy), nil); status != http.StatusOK {
		t.Fatalf("edit policy: %d", status)
	}
	if _, err := e.Owner.Exec(t.Context(), `DROP TRIGGER refuse_inquiry_task ON activity; DROP FUNCTION refuse_inquiry_task()`); err != nil {
		t.Fatal(err)
	}
	var retry struct {
		Retried bool    `json:"retried"`
		Refusal *string `json:"refusal"`
	}
	if status := e.Call(t, "POST", "/v1/automations/runs/"+run.String()+"/retry", nil, nil, &retry); status != http.StatusOK {
		t.Fatalf("retry failed task: %d", status)
	}
	if !retry.Retried {
		t.Fatalf("the retry was refused: %+v", retry)
	}
	var assignee, currentOwner ids.UUID
	var title string
	var actualDue time.Time
	if err := e.Owner.QueryRow(t.Context(), `SELECT assignee_id,subject,due_at FROM activity WHERE source_system=$1`, provenance.WebsiteInquirySource).Scan(&assignee, &title, &actualDue); err != nil {
		t.Fatal(err)
	}
	if err := e.Owner.QueryRow(t.Context(), `SELECT owner_id FROM lead WHERE email='recover@visitor.example'`).Scan(&currentOwner); err != nil {
		t.Fatal(err)
	}
	if assignee != owner || currentOwner != owner || title != "Answer this inquiry" || !actualDue.Equal(due) {
		t.Fatalf("retry changed the original work: assignee=%s owner=%s title=%s due=%s want=%s", assignee, currentOwner, title, actualDue, due)
	}
	var frozenVersion int64
	if err := e.Owner.QueryRow(t.Context(), `SELECT (planned->>'policy_version')::bigint FROM automation_effect_claim WHERE handler=$1 AND effect_fingerprint='inquiry_plan'`, provenance.WebsiteInquirySource).Scan(&frozenVersion); err != nil {
		t.Fatal(err)
	}
	if frozenVersion != version {
		t.Fatalf("retry rewrote policy version %d to %d", version, frozenVersion)
	}
}
