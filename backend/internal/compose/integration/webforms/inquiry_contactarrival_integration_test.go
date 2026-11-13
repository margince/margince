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
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

type contactArrivalFixture struct {
	inquiryRecipientFixture
	receipt crmcontracts.Activity
	task    crmcontracts.Activity
}

func setupContactArrivalBeforeHandling(t *testing.T) contactArrivalFixture {
	t.Helper()
	base := setupInquiryRecipient(t)
	var lead ids.UUID
	var version int64
	if err := base.e.Owner.QueryRow(t.Context(), `SELECT l.id,l.version FROM lead l JOIN activity_link a ON a.lead_id=l.id WHERE a.activity_id=@id`, pgx.NamedArgs{"id": base.note.Id}).Scan(&lead, &version); err != nil {
		t.Fatal(err)
	}
	path := "/v1/leads/" + lead.String()
	if status := base.e.Call(t, "PATCH", path, integration.AnyMap{"owner_id": base.colleague.Id}, map[string]string{"If-Match": strconv.FormatInt(version, 10)}, nil); status != http.StatusOK {
		t.Fatalf("assign contact's lead: %d", status)
	}
	if status := base.e.Call(t, "POST", path+"/promote", integration.AnyMap{"trigger": "human_qualify"}, nil, nil); status != http.StatusOK {
		t.Fatalf("promote contact: %d", status)
	}
	if answer := browserPost(t, base.e, base.form.FormKey, url.Values{"email": {"needs-review@visitor.example"}, "message": {"Another question"}, "_mrg_id": {ids.NewV7().String()}}, siteOrigin); answer.Status != http.StatusAccepted {
		t.Fatalf("contact inquiry: %d", answer.Status)
	}
	var notes crmcontracts.ActivityListResponse
	if status := base.e.Call(t, "GET", "/v1/activities?website_form_id="+base.form.ID+"&limit=1", nil, nil, &notes); status != http.StatusOK || len(notes.Data) != 1 || notes.Data[0].SourceId == nil {
		t.Fatalf("contact receipt: %d %+v", status, notes)
	}
	out := contactArrivalFixture{inquiryRecipientFixture: base, receipt: notes.Data[0]}
	system, key := provenance.WebsiteInquiryTaskSource(*out.receipt.SourceId, true)
	var task ids.UUID
	if err := base.e.Owner.QueryRow(t.Context(), `SELECT id FROM activity WHERE source_system=@system AND source_id=@key`, pgx.NamedArgs{"system": system, "key": key}).Scan(&task); err != nil {
		t.Fatal(err)
	}
	if status := base.e.Call(t, "GET", "/v1/activities/"+task.String(), nil, nil, &out.task); status != http.StatusOK || out.task.SourceActivityId != nil || out.task.Version == nil {
		t.Fatalf("immediate task before handling: %d %+v", status, out.task)
	}
	return out
}

func TestWebsiteHandlingPreservesAnEarlierManualTaskAssignment(t *testing.T) {
	f := setupContactArrivalBeforeHandling(t)
	actor := apptest.SessionUserID(t, f.e)
	var preview crmcontracts.WebsiteInquiryAssignment
	if status := f.e.Call(t, "POST", "/v1/activities/"+f.task.Id.String()+"/inquiry-assignment/preview", integration.AnyMap{"mode": "take", "recipient_id": actor}, nil, &preview); status != http.StatusOK {
		t.Fatalf("review task before handling: %d", status)
	}
	if status := applyInquiryAssignment(t, f.e, "take", preview); status != http.StatusOK {
		t.Fatalf("take before handling: %d", status)
	}
	if err := compose.NewWorkflowEngine(f.e.DB()).HandleEvent(t.Context(), inquiryEventByID(t, f.e, f.receipt.Id, "activity.captured")); err != nil {
		t.Fatal(err)
	}
	var task crmcontracts.Activity
	if status := f.e.Call(t, "GET", "/v1/activities/"+f.task.Id.String(), nil, nil, &task); status != http.StatusOK || task.AssigneeId == nil || task.AssigneeId.String() != actor {
		t.Fatalf("automatic handling replaced the chosen holder: %d holder=%v", status, task.AssigneeId)
	}
	if task.SourceActivityId == nil || *task.SourceActivityId != f.receipt.Id || task.DueAt == nil {
		t.Fatalf("handling did not finish the accepted task: %+v", task)
	}
}

func TestWebsiteHandlingPreservesAnEarlierManualTaskDeadline(t *testing.T) {
	f := setupContactArrivalBeforeHandling(t)
	due := f.receipt.CreatedAt.Add(48 * time.Hour)
	if status := f.e.Call(t, "PATCH", "/v1/activities/"+f.task.Id.String(), integration.AnyMap{"due_at": due, "due_has_time": true}, map[string]string{"If-Match": strconv.FormatInt(int64(*f.task.Version), 10)}, nil); status != http.StatusOK {
		t.Fatalf("set deadline before handling: %d", status)
	}
	if err := compose.NewWorkflowEngine(f.e.DB()).HandleEvent(t.Context(), inquiryEventByID(t, f.e, f.receipt.Id, "activity.captured")); err != nil {
		t.Fatal(err)
	}
	var task crmcontracts.Activity
	if status := f.e.Call(t, "GET", "/v1/activities/"+f.task.Id.String(), nil, nil, &task); status != http.StatusOK || task.DueAt == nil || !task.DueAt.Equal(due) {
		t.Fatalf("handling replaced the chosen deadline: %d due=%v want=%v", status, task.DueAt, due)
	}
	if task.SourceActivityId == nil || *task.SourceActivityId != f.receipt.Id {
		t.Fatalf("handling lost the accepted receipt: %+v", task)
	}
}
