// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The patch family's half of the governance seam (modules/agents/command.go).
// As agentcommandtarget_integration_test.go does for archive, it asserts the
// staged approval row. The ErrRequiresApproval sentinel alone proves less: a
// refusal with nowhere to land answers it too.

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// A confirm-first PATCH stages the record TYPE and the ID the route named: the
// row a human's decision reads and the redemption pins. An agent overwriting a
// name a human typed is the patch that still asks first.
func TestARestPatchStagesTheRowWithID(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	contactID := createdID(t, e, "/v1/contacts", AnyMap{"source": "manual", "full_name": "Ines Human"})
	bearer := agentBearer(t, e, "patch-staging agent")

	var problem struct {
		Code   string `json:"code"`
		Detail string `json:"detail"`
	}
	if status := e.Call(t, "PATCH", "/v1/contacts/"+contactID,
		AnyMap{"full_name": "Ines Machine"}, bearer, &problem); status != http.StatusForbidden ||
		problem.Code != "approval_required" {
		t.Fatalf("agent overwrite of a human-written name → %d %q, want 403 approval_required", status, problem.Code)
	}
	approvalID := ExtractStagedApprovalID(t, problem.Detail)

	targetType, targetID, _, _ := readApproval(t, e, approvalID)
	if targetType != "contact" {
		t.Errorf("staged target_entity_type = %q, want \"contact\"", targetType)
	}
	if targetID == nil {
		t.Fatal("the staged approval names no target id — a decision about which contact was never captured")
	}
	if *targetID != contactID {
		t.Errorf("staged target_entity_id = %s, want %s", *targetID, contactID)
	}

	// The staged row is only half the proof. A door that staged the approval and
	// also applied the patch answers everything above unchanged.
	var fullName string
	if err := e.Owner.QueryRow(t.Context(),
		`SELECT full_name FROM contact WHERE id = $1`, contactID).Scan(&fullName); err != nil {
		t.Fatalf("reading the contact back: %v", err)
	}
	if fullName != "Ines Human" {
		t.Errorf("the contact is named %q — the agent performed unattended the write this "+
			"confirm-first tier should have staged", fullName)
	}
}
