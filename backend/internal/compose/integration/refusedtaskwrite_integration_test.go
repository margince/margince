// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// The Worklist, the deal card and the company brief show create_task and
// add_task refused to these seats. The write behind both, POST /tasks, refuses
// them on its own, so the shown refusal is never the only one.
func TestTheTaskWriteRefusesEverySeatTheVerbIsShownRefused(t *testing.T) {
	const email = "refused-task@fable.test"
	e := apptest.SetupApp(t)
	apptest.BootstrapWorkspaceSession(t, e, "Refused Task", email, "Admin")
	task := AnyMap{"subject": "Send the revised quote", "source": "manual"}
	for _, seat := range []struct {
		name, role, tier, code string
	}{
		{"a full seat without activity.create", "read_only", "full", "permission_denied"},
		{"a read seat holding activity.create", "admin", "read", "seat_tier_insufficient"},
	} {
		apptest.AssignRoleTo(t, e, email, seat.role)
		e.SetWorkspaceSeat(t, seat.tier)
		if status, code := callForCode(t, e, http.MethodPost, "/v1/tasks", task); status != http.StatusForbidden || code != seat.code {
			t.Errorf("%s: POST /tasks → %d %q, want 403 %q", seat.name, status, code, seat.code)
		}
	}
	var written int
	if err := e.Owner.QueryRow(t.Context(), `SELECT count(*) FROM activity WHERE kind = 'task'`).Scan(&written); err != nil {
		t.Fatal(err)
	}
	if written != 0 {
		t.Fatalf("%d tasks were written by seats the write refused", written)
	}
	// The same body from a writer is filed, so the refusals above are the
	// seats' and not a malformed request's.
	apptest.AssignRoleTo(t, e, email, "admin")
	e.SetWorkspaceSeat(t, "full")
	if status, code := callForCode(t, e, http.MethodPost, "/v1/tasks", task); status != http.StatusCreated {
		t.Fatalf("an admin on a full seat: POST /tasks → %d %q, want 201", status, code)
	}
}
