// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The patch family on the REST door (modules/agents/command.go), as an agent on
// a colleague's passport meets it: the patch runs as that colleague's own would.

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// An agent on a colleague's passport overwrites a name a human typed at once,
// as the colleague could in the app. It stages nothing for anyone to approve.
func TestARestPatchOnAColleaguesPassportOverwritesAHumanWrittenNameAtOnce(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	contactID := createdID(t, e, "/v1/contacts", AnyMap{"source": "manual", "full_name": "Ines Human"})
	bearer := agentBearer(t, e, "patch agent")

	var problem struct {
		Code string `json:"code"`
	}
	if status := e.Call(t, "PATCH", "/v1/contacts/"+contactID,
		AnyMap{"full_name": "Ines Machine"}, bearer, &problem); status != http.StatusOK {
		t.Fatalf("agent overwrite of a human-written name → %d %q, want 200", status, problem.Code)
	}

	var fullName string
	if err := e.Owner.QueryRow(t.Context(),
		`SELECT full_name FROM contact WHERE id = $1`, contactID).Scan(&fullName); err != nil {
		t.Fatalf("reading the contact back: %v", err)
	}
	if fullName != "Ines Machine" {
		t.Errorf("the contact is named %q, want the agent's overwrite applied", fullName)
	}
	var staged int
	if err := e.Owner.QueryRow(t.Context(),
		`SELECT count(*) FROM approval WHERE target_entity_id = $1`, contactID).Scan(&staged); err != nil {
		t.Fatalf("counting approvals: %v", err)
	}
	if staged != 0 {
		t.Errorf("the overwrite staged %d approvals, want none", staged)
	}
}
