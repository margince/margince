// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package webforms

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	kevents "github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

func TestInquiryWorklistUsesValidatedSubmittedTypesAndTheSavedChoice(t *testing.T) {
	e := apptest.SetupAppWithOptions(t, compose.WithKeyvault(keyvault.NewMemory()), compose.WithSchemaPool(integration.SchemaPool(t)))
	e.BootstrapWorkspace(t)
	field := apptest.CreateCustomField(t, e, integration.AnyMap{"object": "lead", "label": "Request type", "type": "picklist", "options": []string{"Quote", "Support"}, "source": "manual"})
	form := createForm(t, e, integration.AnyMap{"name": "Inquiries", "sender": "browser", "allowed_origins": []string{siteOrigin}})
	policy := setupInquiryPolicy(t, e, form, "coordinator")
	params := policy.Params
	params["request_type_field"] = field
	if status := e.Call(t, "PATCH", "/v1/automations/"+policy.Id.String(), integration.AnyMap{"params": params}, inquiryVersion(t, policy), &policy); status != http.StatusOK {
		t.Fatalf("choose request type: %d", status)
	}
	seen := map[crmcontracts.Id]bool{}
	for _, value := range []string{"Quote", "Support", "Not an allowed value"} {
		answer := browserPost(t, e, form.FormKey, url.Values{"email": {"request-type@visitor.example"}, "message": {"A question"}, "_mrg_id": {ids.NewV7().String()}, field: {value}}, siteOrigin)
		if answer.Status != http.StatusAccepted {
			t.Fatalf("submission: %d", answer.Status)
		}
		var source crmcontracts.ActivityListResponse
		if status := e.Call(t, "GET", "/v1/activities?website_form_id="+form.ID+"&limit=1", nil, nil, &source); status != http.StatusOK || len(source.Data) != 1 {
			t.Fatalf("accepted source: %d %+v", status, source)
		}
		seen[source.Data[0].Id] = true
		event := inquiryEventByID(t, e, source.Data[0].Id, "activity.captured")
		if err := compose.NewWorkflowEngine(e.DB()).HandleEvent(t.Context(), event); err != nil {
			t.Fatal(err)
		}
	}
	var receipts, tasks, open int
	if err := e.Owner.QueryRow(t.Context(), `SELECT count(*) FILTER (WHERE kind='note' AND source_system='web_form'),count(*) FILTER (WHERE kind='task' AND source_system='website_inquiry'), count(*) FILTER (WHERE kind='task' AND source_system='website_inquiry' AND NOT is_done AND archived_at IS NULL) FROM activity`).Scan(&receipts, &tasks, &open); err != nil {
		t.Fatal(err)
	}
	if receipts != 3 || tasks != 3 || open != 3 || len(seen) != 3 {
		t.Fatalf("accepted %d notes, dispatched %d distinct notes and created %d tasks (%d open); want three of each", receipts, len(seen), tasks, open)
	}
	delete(params, "request_type_field")
	if status := e.Call(t, "PATCH", "/v1/automations/"+policy.Id.String(), integration.AnyMap{"params": params}, inquiryVersion(t, policy), nil); status != http.StatusOK {
		t.Fatalf("change future request type: %d", status)
	}
	var work crmcontracts.Worklist
	if status := e.Call(t, "GET", "/v1/worklist?scope=all&view=today&filter=website_inquiries", nil, nil, &work); status != http.StatusOK || len(work.Queue) != 3 {
		t.Fatalf("website worklist: %d %+v", status, work.Queue)
	}
	types := map[string]int{}
	missing := 0
	for _, row := range work.Queue {
		facts := row.WebsiteInquiry
		if facts == nil || facts.SourceActivityId == nil || facts.ReceivedAt == nil || facts.AssignmentReason == nil || *facts.AssignmentReason != "coordinator_review" {
			t.Fatalf("missing saved handling facts: %+v", row)
		}
		var note crmcontracts.Activity
		if status := e.Call(t, "GET", "/v1/activities/"+facts.SourceActivityId.String(), nil, nil, &note); status != http.StatusOK || !facts.ReceivedAt.Equal(note.CreatedAt) {
			t.Fatalf("receipt time was not read from the accepted source: %d %+v", status, facts)
		}
		if facts.RequestType == nil {
			missing++
		} else {
			types[*facts.RequestType]++
		}
	}
	if types["Quote"] != 1 || types["Support"] != 1 || len(types) != 2 || missing != 1 {
		t.Fatalf("submission types were replaced by current record values or accepted unchecked: %v missing=%d", types, missing)
	}
	var plans string
	if err := e.Owner.QueryRow(t.Context(), `SELECT string_agg(planned::text,'') FROM automation_effect_claim WHERE handler=@handler AND effect_fingerprint='inquiry_plan'`, pgx.NamedArgs{"handler": provenance.WebsiteInquirySource}).Scan(&plans); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"Quote", "Support", "Not an allowed value", "A question"} {
		if strings.Contains(plans, value) {
			t.Fatalf("saved assignment copied submitted content %q", value)
		}
	}
}

func inquiryEventByID(t *testing.T, e *apptest.AppEnv, id crmcontracts.Id, kind string) kevents.Envelope {
	t.Helper()
	var raw json.RawMessage
	if err := e.Owner.QueryRow(t.Context(), `SELECT envelope FROM event_outbox WHERE envelope->>'type'=@kind AND envelope->'entity'->>'id'=@id`, pgx.NamedArgs{"kind": kind, "id": id.String()}).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var event kevents.Envelope
	if err := json.Unmarshal(raw, &event); err != nil {
		t.Fatal(err)
	}
	return event
}

func TestInquiryWorklistShowsHandlingWithoutSettingsAccessAndWithholdsPrivateReceipt(t *testing.T) {
	f := setupInquiryRecipient(t)
	if err := compose.NewWorkflowEngine(f.e.DB()).HandleEvent(t.Context(), f.event); err != nil {
		t.Fatal(err)
	}
	f.revoke(t, "automation")
	colleague := f.e.AsSecondSeat(t, "recipient@inquiry.example", "Inquiry-recipient-2026!")
	if status := colleague.Call(t, "GET", "/v1/automations/"+f.saved.Automation.Id.String(), nil, nil, nil); status != http.StatusForbidden {
		t.Fatalf("automation settings should be refused: %d", status)
	}
	var before crmcontracts.Worklist
	if status := colleague.Call(t, "GET", "/v1/worklist?view=today&filter=website_inquiries", nil, nil, &before); status != http.StatusOK || len(before.Queue) != 1 {
		t.Fatalf("colleague inquiry: %d %+v", status, before.Queue)
	}
	facts := before.Queue[0].WebsiteInquiry
	if facts == nil || facts.AssignmentReason == nil || facts.SourceActivityId == nil || *facts.SourceActivityId != f.note.Id {
		t.Fatalf("readable task lost its handling facts: %+v", facts)
	}
	if f.note.Version == nil {
		t.Fatal("source has no version")
	}
	if status := f.e.Call(t, "PATCH", "/v1/activities/"+f.note.Id.String()+"/audience", integration.AnyMap{"audience": "selected", "members": []integration.AnyMap{{"subject_type": "user", "subject_id": apptest.SessionUserID(t, f.e)}}}, map[string]string{"If-Match": strconv.FormatInt(int64(*f.note.Version), 10)}, nil); status != http.StatusOK {
		t.Fatalf("restrict source audience: %d", status)
	}
	var after crmcontracts.Worklist
	if status := colleague.Call(t, "GET", "/v1/worklist?view=today&filter=website_inquiries", nil, nil, &after); status != http.StatusOK {
		t.Fatalf("private receipt worklist: %d", status)
	}
	for _, row := range after.Queue {
		if row.WebsiteInquiry != nil && (row.WebsiteInquiry.SourceActivityId != nil || row.WebsiteInquiry.ReceivedAt != nil || row.WebsiteInquiry.SourceLabel != nil || row.WebsiteInquiry.RequestType != nil) {
			t.Fatalf("private receipt leaked facts: %+v", row.WebsiteInquiry)
		}
	}
}
