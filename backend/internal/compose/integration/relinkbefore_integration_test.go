// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"testing"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// removedLinksNamed counts the activity's relink audit rows whose before names
// the given ids of the given entity type and no others.
func removedLinksNamed(t *testing.T, e *Env, activity ids.UUID, entityType string, removed ...ids.UUID) int {
	t.Helper()
	return e.WsCount(t, `SELECT count(*) FROM audit_log
		 WHERE action = 'activity_relink' AND entity_id = $1
		   AND before->>'entity_type' = $2
		   AND jsonb_array_length(before->'entity_ids') = cardinality($3::uuid[])
		   AND (SELECT bool_and(before->'entity_ids' ? r) FROM unnest($3::uuid[]) AS r_id, LATERAL (SELECT r_id::text AS r) x)`,
		activity, entityType, removed)
}

// A replacing relink names the links it removed in the audit row's before.
// The single door and the named-set door both do it.
func TestReplacingRelinkNamesTheRemovedLinksInTheAuditBefore(t *testing.T) {
	e := Setup(t)
	admin := e.As(e.Rep1, nil, AdminPerms)
	a, b, c := e.SeedContact(t, "Ingrid Sattler", nil), e.SeedContact(t, "Tomas Berg", nil), e.SeedContact(t, "Petra Lang", nil)
	single := seedRelinkActivity(t, e, "Single")
	bulk := seedRelinkActivity(t, e, "Bulk")
	for _, act := range []ids.UUID{single, bulk} {
		for _, p := range []ids.UUID{a, b} {
			e.WsExec(t, `INSERT INTO activity_link (activity_id, entity_type, contact_id) VALUES ($1, 'contact', $2)`, act, p)
		}
	}
	in := activities.RelinkActivityInput{EntityType: "contact", EntityID: c, ReplaceExistingOfType: true}
	if _, err := e.Activities.RelinkActivity(admin, ids.From[ids.ActivityKind](single), in); err != nil {
		t.Fatalf("single relink: %v", err)
	}
	if _, err := e.Activities.RelinkActivities(admin, []ids.UUID{bulk}, in); err != nil {
		t.Fatalf("bulk relink: %v", err)
	}
	for name, act := range map[string]ids.UUID{"single": single, "bulk": bulk} {
		if n := removedLinksNamed(t, e, act, "contact", a, b); n != 1 {
			t.Errorf("%s: audit rows naming both removed contacts = %d, want 1", name, n)
		}
	}

	// An additive relink removed nothing and keeps an empty before.
	added := seedRelinkActivity(t, e, "Additive")
	if _, err := e.Activities.RelinkActivity(admin, ids.From[ids.ActivityKind](added),
		activities.RelinkActivityInput{EntityType: "contact", EntityID: a}); err != nil {
		t.Fatalf("additive relink: %v", err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE action = 'activity_relink' AND entity_id = $1 AND before IS NULL`, added); n != 1 {
		t.Errorf("additive relink audit rows with an empty before = %d, want 1", n)
	}
}

// The project link is the second kind a replace removes.
func TestReplacingAProjectLinkNamesTheOldProjectInTheAuditBefore(t *testing.T) {
	e := Setup(t)
	admin := e.As(e.Rep1, nil, AdminPerms)
	company := e.SeedCompany(t, "Acme", &e.Rep1)
	oldProject, newProject := ids.NewV7(), ids.NewV7()
	for _, p := range []ids.UUID{oldProject, newProject} {
		e.WsExec(t, `INSERT INTO project (id, name, company_id, source, captured_by) VALUES ($1, 'P', $2, 'manual', 'human:x')`, p, company)
	}
	act := seedRelinkActivity(t, e, "Filed")
	e.WsExec(t, `INSERT INTO activity_link (activity_id, entity_type, project_id) VALUES ($1, 'project', $2)`, act, oldProject)
	if _, err := e.Activities.RelinkActivity(admin, ids.From[ids.ActivityKind](act),
		activities.RelinkActivityInput{EntityType: "project", EntityID: newProject, ReplaceExistingOfType: true}); err != nil {
		t.Fatalf("project relink: %v", err)
	}
	if n := removedLinksNamed(t, e, act, "project", oldProject); n != 1 {
		t.Errorf("audit rows naming the removed project = %d, want 1", n)
	}
}
