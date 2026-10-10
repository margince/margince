// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The writes that reach outside: enrich and invite_meeting are confirm-first
// for an agent. Each door stages one approval of the same kind and runs nothing
// until a human approves. Each test asks both doors about two equal records.

import (
	"net/http"
	"testing"
)

// A company enrichment is staged, not run, on either door.
func TestEnrichStagesTheSameApprovalOnEitherDoor(t *testing.T) {
	d := newTwoDoors(t, "enrich-write-doors", "read", "write", "enrich")
	company := func(name string) string {
		return createdID(t, d.e, "/v1/companies", AnyMap{"display_name": name, "domain": "doors.example", "source": "manual"})
	}
	viaREST, viaTool := company("Enrich Rest"), company("Enrich Tool")

	var refusal fileRefusal
	if status := d.e.Call(t, "POST", "/v1/companies/"+viaREST+"/enrich", AnyMap{}, d.bearer, &refusal); status != http.StatusForbidden || refusal.Code != "approval_required" {
		t.Fatalf("an enrichment over REST → %d %q, want 403 approval_required", status, refusal.Code)
	}
	d.toolRefused(t, "enrich", map[string]any{"company_id": viaTool})
	card := d.sameStagedKind(t, "enrich", viaREST, viaTool)
	if card.Status != "pending" || card.TargetType != "company" {
		t.Errorf("the staged enrichment is %+v, want a pending one on the company", card)
	}
}

// A calendar invitation is staged, not sent, on either door.
func TestInviteMeetingStagesTheSameApprovalOnEitherDoor(t *testing.T) {
	d := newTwoDoors(t, "invite-write-doors", "read", "write", "send")
	contact := func(name string) string {
		return createdID(t, d.e, "/v1/contacts", AnyMap{"full_name": name, "source": "manual"})
	}
	viaREST, viaTool := contact("Invite Rest"), contact("Invite Tool")
	invitation := func(contactID string) map[string]any {
		return map[string]any{
			"contact_id": contactID, "attendee_email": "buyer@doors.example",
			"start": "2036-10-05T09:00:00Z", "end": "2036-10-05T09:30:00Z",
			"subject": "Discovery", "description": "Project scope", "location": "Video call",
		}
	}

	var refusal fileRefusal
	if status := d.e.Call(t, "POST", "/v1/scheduling/invitations", invitation(viaREST), d.bearer, &refusal); status != http.StatusForbidden || refusal.Code != "approval_required" {
		t.Fatalf("an invitation over REST → %d %q, want 403 approval_required", status, refusal.Code)
	}
	d.toolRefused(t, "invite_meeting", invitation(viaTool))
	card := d.sameStagedKind(t, "invite", viaREST, viaTool)
	if card.Status != "pending" {
		t.Errorf("the staged invitation is %+v, want a pending one", card)
	}
}
