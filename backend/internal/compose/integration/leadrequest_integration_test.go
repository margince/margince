// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func TestALeadRequestSupportsItsOwnRecipientWithoutConversionOrSending(t *testing.T) {
	e := setupNewAddress(t)
	const address = "requester@lead.test"
	var lead struct {
		ID string `json:"id"`
	}
	if status := e.Call(t, "POST", "/v1/leads", AnyMap{
		"full_name": "Requested information", "email": address, "source": "manual",
	}, nil, &lead); status != http.StatusCreated {
		t.Fatalf("create lead = %d", status)
	}
	message := AnyMap{"to": []string{address}, "links": []AnyMap{{"entity_type": "company", "entity_id": e.company}}, "communication_context": "requested_followup"}
	preview := func() crmcontracts.SendAuthorizationPreview {
		t.Helper()
		var out crmcontracts.SendAuthorizationPreview
		if status := e.Call(t, "POST", "/v1/emails:preview", message, nil, &out); status != http.StatusOK {
			t.Fatalf("preview = %d", status)
		}
		if len(out.Recipients) != 1 {
			t.Fatalf("preview has %d recipients", len(out.Recipients))
		}
		return out
	}
	before := preview()
	if before.Allowed {
		t.Fatal("a lead without evidence was permitted")
	}
	subject := before.Recipients[0].EvidenceSubject
	if subject == nil || string(subject.EntityType) != "lead" || subject.EntityId.String() != lead.ID {
		t.Fatalf("evidence subject = %+v; want the existing lead", subject)
	}
	path := "/v1/leads/" + lead.ID + "/consent/qualifying-events"
	fact := AnyMap{"kind": "requested_by_subject", "note": "Asked by phone for this information.", "occurred_at": "2020-03-02T09:00:00Z"}
	var recorded crmcontracts.QualifyingEventRecord
	if status := e.Call(t, "POST", path, fact, nil, &recorded); status != http.StatusCreated {
		t.Fatalf("record request = %d", status)
	}
	if after := preview(); !after.Allowed {
		t.Fatalf("recorded request did not support the lead: %+v", after.Recipients)
	}
	var deliveries, contacts int
	var promoted *string
	if err := e.Owner.QueryRow(t.Context(), `SELECT (SELECT count(*) FROM comms_outbound),
 (SELECT count(*) FROM contact_email WHERE email=$1), promoted_contact_id::text FROM lead WHERE id=$2`, address, lead.ID).Scan(&deliveries, &contacts, &promoted); err != nil {
		t.Fatal(err)
	}
	if deliveries != 0 || contacts != 0 || promoted != nil {
		t.Fatalf("recording sent %d deliveries, made %d contact addresses, promoted to %v", deliveries, contacts, promoted)
	}
	if status := e.Call(t, "POST", path+"/"+recorded.Id.String()+"/retract", nil, nil, nil); status != http.StatusNoContent {
		t.Fatalf("retract = %d", status)
	}
	if after := preview(); after.Allowed {
		t.Fatal("a retracted request still supported the message")
	}
	var retried crmcontracts.QualifyingEventRecord
	if status := e.Call(t, "POST", path, fact, nil, &retried); status != http.StatusCreated {
		t.Fatalf("repeat recording = %d", status)
	}
	if retried.Id != recorded.Id || !retried.Retracted {
		t.Fatalf("retry revived or duplicated the request: %+v", retried)
	}
}
