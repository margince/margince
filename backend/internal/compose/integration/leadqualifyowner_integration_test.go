// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// heldWithLead logs a meeting with the lead that was held an hour ago.
func heldWithLead(t *testing.T, e *apptest.AppEnv, lead string) {
	t.Helper()
	createdID(t, e, "/v1/activities", AnyMap{
		"kind": "meeting", "subject": "Lead demo", "meeting_status": "held", "source": "manual",
		"occurred_at": clocktest.Now(t).Add(-time.Hour).UTC().Format(time.RFC3339),
		"links":       []AnyMap{{"entity_type": "lead", "entity_id": lead}},
	})
}

// qualifyTasks counts the lead's open qualify tasks, all of them or the owner's.
func qualifyTasks(t *testing.T, e *apptest.AppEnv, lead string, ownerOnly bool) int {
	t.Helper()
	var n int
	if err := e.Owner.QueryRow(context.Background(), `SELECT count(*) FROM activity a JOIN activity_link al ON al.activity_id = a.id
		JOIN lead l ON l.id = al.lead_id WHERE al.lead_id = $1 AND a.kind = 'task' AND a.source_system = $2 AND NOT a.is_done
		  AND (NOT $3 OR a.assignee_id IS NOT DISTINCT FROM l.owner_id)`, lead, provenance.LeadRepliedSource, ownerOnly).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func ownLeadByItsCreator(t *testing.T, e *apptest.AppEnv, lead string) {
	t.Helper()
	if _, err := e.Owner.Exec(context.Background(),
		`UPDATE lead SET owner_id = split_part(captured_by, ':', 2)::uuid WHERE id = $1`, lead); err != nil {
		t.Fatal(err)
	}
}

// A lead nobody owns has nobody to ask. Once someone owns it, the next held
// meeting asks them.
func TestAnOwnerlessLeadGetsNoQualifyTaskUntilSomeoneOwnsIt(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	lead := createdID(t, e, "/v1/leads", AnyMap{"full_name": "Lena Prospect", "email": "lena@prospect.example"})
	heldWithLead(t, e, lead)
	if n := qualifyTasks(t, e, lead, false); n != 0 {
		t.Fatalf("a lead nobody owns got %d qualify task(s), which no Worklist shows", n)
	}
	ownLeadByItsCreator(t, e, lead)
	heldWithLead(t, e, lead)
	if n := qualifyTasks(t, e, lead, true); n != 1 {
		t.Errorf("the new owner has %d qualify task(s) after the next held meeting, want 1", n)
	}
}

// A qualify task filed before its lead had an owner is nobody's. The next held
// meeting hands it to the owner instead of letting it block their task.
func TestAnUnassignedQualifyTaskGoesToTheLeadsOwner(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	lead := createdID(t, e, "/v1/leads", AnyMap{"full_name": "Lena Prospect", "email": "lena@prospect.example"})
	ownLeadByItsCreator(t, e, lead)
	var task string
	if err := e.Owner.QueryRow(context.Background(), `WITH t AS (
		INSERT INTO activity (kind, subject, occurred_at, source, captured_by, source_system)
		VALUES ('task', 'Review qualification for Lena Prospect', now() - interval '2 days', 'system', 'system:lead_replied', $2)
		RETURNING id)
		INSERT INTO activity_link (activity_id, entity_type, lead_id) SELECT id, 'lead', $1 FROM t RETURNING activity_id`,
		lead, provenance.LeadRepliedSource).Scan(&task); err != nil {
		t.Fatalf("seeding the unassigned task: %v", err)
	}
	heldWithLead(t, e, lead)
	if n := qualifyTasks(t, e, lead, true); n != 1 || qualifyTasks(t, e, lead, false) != 1 {
		t.Errorf("the owner has %d qualify task(s); the unassigned one %s should be theirs and the only one", n, task)
	}
	var adopted bool
	if err := e.Owner.QueryRow(context.Background(), `SELECT NOT a.is_done AND a.assignee_id IS NOT DISTINCT FROM l.owner_id
		FROM activity a, lead l WHERE a.id = $1 AND l.id = $2`, task, lead).Scan(&adopted); err != nil {
		t.Fatalf("reading the seeded task back: %v", err)
	}
	if !adopted {
		t.Errorf("the seeded task %s was replaced or left unassigned instead of handed to the owner", task)
	}
}
