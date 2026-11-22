// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"testing"
	"time"
)

// A disqualified lead is an archived record, so it leaves its Shortlists as a
// contact, company or deal does. The list records why each member went.
func TestADisqualifiedLeadLeavesItsShortlists(t *testing.T) {
	e := setupPromoteConsent(t)
	lead := e.seedLeadCreatedAt(t, "shortlisted-lead@example.test", time.Now().UTC().Add(-time.Hour))

	var list string
	if err := e.owner.QueryRow(e.ctx,
		`INSERT INTO list (name, entity_type) VALUES ('Spring outreach', 'lead') RETURNING id::text`).Scan(&list); err != nil {
		t.Fatalf("creating the list: %v", err)
	}
	if _, err := e.owner.Exec(e.ctx,
		`INSERT INTO list_member (list_id, entity_type, entity_id, added_by) VALUES ($1, 'lead', $2, 'human:seed')`,
		list, lead); err != nil {
		t.Fatalf("shortlisting the lead: %v", err)
	}

	if _, err := e.store.DisqualifyLead(e.ctx, lead, DisqualifyLeadInput{}); err != nil {
		t.Fatalf("disqualifying the lead: %v", err)
	}

	var members, removals int
	if err := e.owner.QueryRow(e.ctx,
		`SELECT count(*) FROM list_member WHERE entity_type = 'lead' AND entity_id = $1`, lead).Scan(&members); err != nil {
		t.Fatalf("counting the lead's memberships: %v", err)
	}
	if members != 0 {
		t.Errorf("the disqualified lead is still on %d Shortlist(s)", members)
	}
	if err := e.owner.QueryRow(e.ctx,
		`SELECT count(*) FROM list_member_event
		  WHERE entity_type = 'lead' AND entity_id = $1 AND action = 'removed' AND reason = 'record_archived'`,
		lead).Scan(&removals); err != nil {
		t.Fatalf("counting the removals: %v", err)
	}
	if removals != 1 {
		t.Errorf("the list logged %d archive removals for the lead, want 1", removals)
	}
}
