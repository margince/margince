// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The writes that reach outside. enrich stays confirm-first even on a
// colleague's passport, so each door stages one approval of the same kind.
// invite_meeting runs at once on that passport, as the colleague's own would.

import (
	"net/http"
	"strings"
	"testing"
)

// A company enrichment is staged, not run, on either door. Only the staging is
// compared: an approved run reads the company's website, and the harness has no
// stand-in for the web.
func TestEnrichStagesTheSameApprovalOnEitherDoor(t *testing.T) {
	d := newTwoDoors(t, "enrich-write-doors", "read", "write", "enrich")
	company := func(name, domain string) string {
		return createdID(t, d.e, "/v1/companies", AnyMap{"display_name": name, "domains": []AnyMap{{"domain": domain, "is_primary": true}}, "source": "manual"})
	}
	viaREST, viaTool := company("Enrich Rest", "rest.doors.example"), company("Enrich Tool", "tool.doors.example")

	var refusal fileRefusal
	if status := d.e.Call(t, "POST", "/v1/companies/"+viaREST+"/enrich", AnyMap{}, d.bearer, &refusal); status != http.StatusForbidden || refusal.Code != "approval_required" {
		t.Fatalf("an enrichment over REST → %d %q, want 403 approval_required", status, refusal.Code)
	}
	d.toolRefused(t, "enrich", map[string]any{"company_id": viaTool})
	card := d.sameStaged(t, "enrich", viaREST, viaTool, "Enrich Rest", "Enrich Tool")
	if card.Status != "pending" || card.TargetType != "company" {
		t.Errorf("the staged enrichment is %+v, want a pending one on the company", card)
	}
}

// A calendar invitation on a colleague's passport is not staged on either door:
// it goes to the calendar step at once. The harness has no calendar provider, so
// only the absence of a staging is compared.
func TestInviteMeetingOnAColleaguesPassportIsStagedOnNeitherDoor(t *testing.T) {
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
	if status := d.e.Call(t, "POST", "/v1/scheduling/invitations", invitation(viaREST), d.bearer, &refusal); refusal.Code == "approval_required" {
		t.Fatalf("an invitation over REST → %d %q, want it past the approval gate", status, refusal.Code)
	}
	if refused := d.toolRefused(t, "invite_meeting", invitation(viaTool)); strings.Contains(refused, "staged as approval") {
		t.Fatalf("an invitation over the tool was staged:\n%s", refused)
	}
	var staged int
	if err := d.e.Owner.QueryRow(t.Context(), `SELECT count(*) FROM approval`).Scan(&staged); err != nil {
		t.Fatalf("counting approvals: %v", err)
	}
	if staged != 0 {
		t.Errorf("the two invitations staged %d approvals, want none", staged)
	}
}
