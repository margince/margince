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
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

func TestInquiryPreviewWritesNothingAndAgreesWithLiveHandling(t *testing.T) {
	e := setupFormsApp(t)
	form := createForm(t, e, integration.AnyMap{"name": "Inquiries", "sender": "browser", "allowed_origins": []string{siteOrigin}})
	policy := setupInquiryPolicy(t, e, form, "one_colleague")
	answer := browserPost(t, e, form.FormKey, url.Values{"email": {"preview@visitor.example"}, "message": {"Please call"}}, siteOrigin)
	if answer.Status != http.StatusAccepted {
		t.Fatalf("submission: %d %s", answer.Status, answer.Body)
	}
	event := inquiryEvent(t, e, "preview@visitor.example", "activity.captured")
	var version int64
	var beforeAudit, beforeEvents int
	if err := e.Owner.QueryRow(t.Context(), `SELECT version,(SELECT count(*) FROM audit_log),(SELECT count(*) FROM event_outbox) FROM activity WHERE id=@id`, pgx.NamedArgs{"id": event.Entity.ID}).Scan(&version, &beforeAudit, &beforeEvents); err != nil {
		t.Fatal(err)
	}
	body := integration.AnyMap{"policy": policy.InquiryPolicy, "automation_id": policy.Id, "automation_version": policy.Version, "selected": []integration.AnyMap{{"id": event.Entity.ID, "version": version}}}
	path := "/v1/web-forms/" + form.ID + "/handling/preview"
	var preview crmcontracts.WebsiteInquiryHandlingPreview
	if status := e.Call(t, "POST", path, body, nil, &preview); status != http.StatusOK {
		t.Fatalf("preview: %d", status)
	}
	if len(preview.Items) != 1 || preview.Items[0].Status != crmcontracts.WebsiteInquiryPreviewItemStatusEligible || preview.Items[0].RecipientId == nil || preview.Items[0].DueAt == nil {
		t.Fatalf("preview did not plan the selected source: %+v", preview)
	}
	var afterAudit, afterEvents int
	var owner *ids.UUID
	if err := e.Owner.QueryRow(t.Context(), `SELECT (SELECT count(*) FROM audit_log),(SELECT count(*) FROM event_outbox),owner_id FROM lead WHERE email='preview@visitor.example'`).Scan(&afterAudit, &afterEvents, &owner); err != nil {
		t.Fatal(err)
	}
	if beforeAudit != afterAudit || beforeEvents != afterEvents || owner != nil {
		t.Fatalf("preview wrote: audit %d/%d events %d/%d owner %v", beforeAudit, afterAudit, beforeEvents, afterEvents, owner)
	}
	if err := compose.NewWorkflowEngine(e.DB()).HandleEvent(t.Context(), event); err != nil {
		t.Fatal(err)
	}
	var assignee ids.UUID
	var due time.Time
	if err := e.Owner.QueryRow(t.Context(), `SELECT assignee_id,due_at FROM activity WHERE source_system=@source AND kind='task'`, pgx.NamedArgs{"source": provenance.WebsiteInquirySource}).Scan(&assignee, &due); err != nil {
		t.Fatal(err)
	}
	if assignee != ids.UUID(*preview.Items[0].RecipientId) || !due.Equal(*preview.Items[0].DueAt) {
		t.Fatalf("live task differs from preview: recipient %s due %s; %+v", assignee, due, preview.Items[0])
	}
	preview = crmcontracts.WebsiteInquiryHandlingPreview{}
	if status := e.Call(t, "POST", path, body, nil, &preview); status != http.StatusOK || preview.Items[0].Status != crmcontracts.WebsiteInquiryPreviewItemStatusAlreadyHandled {
		t.Fatalf("handled source preview: %d %+v", status, preview)
	}
	if preview.Items[0].RecipientId != nil || preview.Items[0].DueAt != nil {
		t.Fatalf("existing task reported an unapplied proposed assignment: %+v", preview.Items[0])
	}
	body["selected"] = []integration.AnyMap{{"id": event.Entity.ID, "version": version + 1}, {"id": ids.NewV7(), "version": 1}}
	preview = crmcontracts.WebsiteInquiryHandlingPreview{}
	if status := e.Call(t, "POST", path, body, nil, &preview); status != http.StatusOK || len(preview.Items) != 2 {
		t.Fatalf("stale/missing source preview: %d %+v", status, preview)
	}
	if preview.Items[0].Status != crmcontracts.WebsiteInquiryPreviewItemStatusChanged || preview.Items[1].Status != crmcontracts.WebsiteInquiryPreviewItemStatusUnavailable || preview.Items[1].TargetId != nil || preview.Items[1].RecipientId != nil {
		t.Fatalf("stale/missing source leaked a plan: %+v", preview)
	}
	selected := make([]integration.AnyMap, 51)
	for i := range selected {
		selected[i] = integration.AnyMap{"id": ids.NewV7(), "version": 1}
	}
	body["selected"] = selected
	if status := e.Call(t, "POST", path, body, nil, nil); status != http.StatusUnprocessableEntity {
		t.Fatalf("selection over 50: %d", status)
	}
}

func TestDisqualifyingAnOwnedLeadSkipsItsPendingInquiryWithoutRecreatingWork(t *testing.T) {
	e := setupFormsApp(t)
	form := createForm(t, e, integration.AnyMap{"name": "Inquiries", "sender": "browser", "allowed_origins": []string{siteOrigin}})
	policy := setupInquiryPolicy(t, e, form, "one_colleague")
	answer := browserPost(t, e, form.FormKey, url.Values{"email": {"closed@visitor.example"}, "message": {"Please call"}}, siteOrigin)
	if answer.Status != http.StatusAccepted {
		t.Fatalf("submission: %d %s", answer.Status, answer.Body)
	}
	event := inquiryEvent(t, e, "closed@visitor.example", "activity.captured")
	var lead ids.UUID
	var version int64
	if err := e.Owner.QueryRow(t.Context(), `SELECT id,version FROM lead WHERE email='closed@visitor.example'`).Scan(&lead, &version); err != nil {
		t.Fatal(err)
	}
	if policy.InquiryPolicy == nil {
		t.Fatal("typed policy is missing")
	}
	path := "/v1/leads/" + lead.String()
	if status := e.Call(t, "PATCH", path, integration.AnyMap{"owner_id": policy.InquiryPolicy.CoordinatorId}, map[string]string{"If-Match": strconv.FormatInt(version, 10)}, nil); status != http.StatusOK {
		t.Fatalf("assign lead: %d", status)
	}
	if status := e.Call(t, "DELETE", path, nil, nil, nil); status != http.StatusOK {
		t.Fatalf("disqualify lead: %d", status)
	}
	if err := compose.NewWorkflowEngine(e.DB()).HandleEvent(t.Context(), event); err != nil {
		t.Fatalf("retired target was retried as a failure: %v", err)
	}
	var count int
	if err := e.Owner.QueryRow(t.Context(), `SELECT count(*) FROM activity WHERE kind='task' AND source_system=@source`, pgx.NamedArgs{"source": provenance.WebsiteInquirySource}).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("retired lead gained %d inquiry tasks", count)
	}
	var status string
	if err := e.Owner.QueryRow(t.Context(), `SELECT status FROM workflow_run WHERE handler=@handler AND trigger_event=@event`, pgx.NamedArgs{"handler": provenance.WebsiteInquirySource, "event": event.EventID}).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "skipped" {
		t.Fatalf("retired inquiry status: %s", status)
	}
}
