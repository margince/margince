// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package webforms

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

func TestPromotingALeadKeepsItsInquiryOpenOnTheContact(t *testing.T) {
	f := setupInquiryRecipient(t)
	engine := compose.NewWorkflowEngine(f.e.DB())
	if err := engine.HandleEvent(t.Context(), f.event); err != nil {
		t.Fatal(err)
	}
	var lead ids.UUID
	var version int64
	if err := f.e.Owner.QueryRow(t.Context(), `SELECT l.id,l.version FROM lead l JOIN activity_link a ON a.lead_id=l.id WHERE a.activity_id=@source`, pgx.NamedArgs{"source": f.note.Id}).Scan(&lead, &version); err != nil {
		t.Fatal(err)
	}
	var before crmcontracts.ActivityListResponse
	if status := f.e.Call(t, "GET", "/v1/activities?source_activity_id="+f.note.Id.String(), nil, nil, &before); status != http.StatusOK || len(before.Data) != 1 {
		t.Fatalf("accepted inquiry task: %d %+v", status, before)
	}
	var promoted crmcontracts.PromoteLeadResponse
	if status := f.e.Call(t, "POST", "/v1/leads/"+lead.String()+"/promote", integration.AnyMap{"trigger": "human_qualify"}, map[string]string{"If-Match": strconv.FormatInt(version, 10)}, &promoted); status != http.StatusOK || ids.UUID(promoted.Contact.Id).IsZero() {
		t.Fatalf("promote inquiry lead: %d %+v", status, promoted)
	}
	event := inquiryEventByID(t, f.e, crmcontracts.Id(lead), "lead.promoted")
	if err := engine.HandleEvent(t.Context(), event); err != nil {
		t.Fatal(err)
	}
	var after crmcontracts.Activity
	if status := f.e.Call(t, "GET", "/v1/activities/"+before.Data[0].Id.String(), nil, nil, &after); status != http.StatusOK || after.IsDone == nil || *after.IsDone {
		t.Fatalf("promotion completed an unanswered inquiry: %d %+v", status, after)
	}
	if after.Links == nil {
		t.Fatal("inquiry lost its record links")
	}
	linked := false
	for _, link := range *after.Links {
		linked = linked || (link.EntityType == "contact" && ids.UUID(link.EntityId) == ids.UUID(promoted.Contact.Id))
	}
	if !linked {
		t.Fatalf("inquiry did not follow its contact: %+v", after.Links)
	}
	if err := engine.HandleEvent(t.Context(), f.event); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := f.e.Owner.QueryRow(t.Context(), `SELECT count(*) FROM activity WHERE kind='task' AND source_system=@source`, pgx.NamedArgs{"source": provenance.WebsiteInquirySource}).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("promotion replay created %d tasks", count)
	}
}
