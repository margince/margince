// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The confirm-first CREATE class, over the passport surface a real agent uses.
//
// A staged create names the target TYPE it will write and no target id — the
// record does not exist yet, so the gate has no `{id}` to take one from. That
// shape has to be decidable by the decision grants alone: there is no row whose
// scope could bound it, and `decidable` backs the inbox list, the single read
// and the decision alike, so a shape it rejects is an authority object no human
// can release or reject while the agent that staged it retries and mints more.

import (
	"net/http"
	"testing"

	"github.com/gradionhq/margince/backend/internal/compose/integration/apptest"
)

func TestStagedCreateWithNoTargetIDIsListedAndDecidable(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)

	var minted struct {
		Token string `json:"token"`
	}
	if status := e.Call(t, "POST", "/v1/passports", apptest.AnyMap{
		"label": "create agent", "scopes": []string{"read", "write"},
	}, nil, &minted); status != http.StatusCreated {
		t.Fatalf("issue passport → %d", status)
	}
	bearer := map[string]string{"Authorization": "Bearer " + minted.Token}

	// createWebhookSubscription is confirm-first, so the agent's call stages
	// instead of writing. The identical body is the redemption key, so it is
	// sent twice.
	body := apptest.AnyMap{"target_url": "https://example.test/hook", "event_types": []string{"organization.created"}}
	var problem struct {
		Code   string `json:"code"`
		Detail string `json:"detail"`
	}
	if status := e.Call(t, "POST", "/v1/webhook-subscriptions", body, bearer, &problem); status != http.StatusForbidden ||
		problem.Code != "approval_required" {
		t.Fatalf("agent webhook-subscription create → %d %q, want 403 approval_required", status, problem.Code)
	}
	approvalID := ExtractStagedApprovalID(t, problem.Detail)

	// The shape this test is about: the type the approved call will write, and
	// no row for a scope probe to resolve.
	var targetType, targetID *string
	if err := e.Owner.QueryRow(t.Context(),
		`SELECT target_entity_type, target_entity_id FROM approval WHERE id = $1`,
		approvalID).Scan(&targetType, &targetID); err != nil {
		t.Fatal(err)
	}
	if targetType == nil || *targetType != "webhook_subscription" || targetID != nil {
		t.Fatalf("staged target = (%v, %v), want (project, NULL) — this scenario is the id-less shape",
			targetType, targetID)
	}

	// Seeing it is deciding it: the row is in the pending inbox and answers
	// the single read.
	assertDecidableInTheInbox(t, e, approvalID, "webhook_subscription")

	// And the decision releases the identical call, which lands the project.
	if status := e.Call(t, "POST", "/v1/approvals/"+approvalID+"/approve", apptest.AnyMap{}, nil, nil); status != http.StatusOK {
		t.Fatalf("human approve → %d", status)
	}
	withToken := map[string]string{
		"Authorization": "Bearer " + minted.Token, "X-Approval-Token": approvalID,
	}
	// The approved retry gets PAST the gate, which is what this test is about.
	// Where it lands after that is the webhook module's business — this
	// composition wires no outbound delivery, so the create answers 503 — and
	// the distinction that matters is 403 (refused by the gate, the approval
	// did nothing) versus anything else (released, and now the module's own
	// answer).
	status := e.Call(t, "POST", "/v1/webhook-subscriptions", body, withToken, nil)
	if status == http.StatusForbidden {
		t.Fatalf("approved retry → 403 — the approval did not release the staged create")
	}
}
