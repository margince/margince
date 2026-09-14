// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// Promotion carries the lead's timeline onto the contact it became
// (LEADS-FORM-5 step 3: "the lead's history, provenance, and activities carry
// over with nothing orphaned").
//
// This needs a database because the link row is CONVERTED rather than
// repointed: activity_link_shape admits exactly one target per row, so a row
// that kept lead_id while gaining contact_id violates the CHECK, and
// uq_activity_link rejects a converted row that duplicates a link the contact
// already had. Both are constraint interplay no unit test sees.
//
// The failure this guards is silent and total. Before the carry existed, every
// note and task a rep logged against a lead stayed on the retired lead row: the
// contact's timeline was empty, and the lead's page had already redirected away.
// Nothing errored.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// seedLeadActivity links one activity to a lead, the way the timeline write
// does, and answers its id so the assertions can name it.
func (e *promoteConsentEnv) seedLeadActivity(t *testing.T, lead ids.LeadID, subject string) ids.UUID {
	t.Helper()
	activity := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by)
		 VALUES ($1, 'note', $2, now(), 'manual', 'human:x')`,
		activity, subject); err != nil {
		t.Fatal(err)
	}
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO activity_link (id, activity_id, entity_type, lead_id)
		 VALUES ($1, $2, 'lead', $3)`,
		ids.NewV7(), activity, lead.UUID); err != nil {
		t.Fatal(err)
	}
	return activity
}

// linkTargets answers how one activity is linked, as (entity_type, target id)
// pairs — the shape the CHECK constrains, so a half-converted row is visible.
func (e *promoteConsentEnv) linkTargetsOf(t *testing.T, activity ids.UUID) map[string]ids.UUID {
	t.Helper()
	rows, err := e.owner.Query(context.Background(),
		`SELECT entity_type, coalesce(contact_id, company_id, deal_id, lead_id)
		 FROM activity_link WHERE activity_id = $1`, activity)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]ids.UUID{}
	for rows.Next() {
		var kind string
		var target ids.UUID
		if err := rows.Scan(&kind, &target); err != nil {
			t.Fatal(err)
		}
		out[kind] = target
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// The ordinary case: a lead nobody knew becomes a contact, and everything logged
// against it while it was a lead is now on that contact's timeline.
func TestPromotionCarriesTheLeadsActivitiesToTheContact(t *testing.T) {
	e := setupPromoteConsent(t)
	lead := e.seedLead(t, "carry@example.test")
	activity := e.seedLeadActivity(t, lead, "Called about the Q4 rollout")

	contact, merged, err := e.store.PromoteLead(e.ctx, lead, PromoteLeadInput{Trigger: "human_qualify"})
	if err != nil {
		t.Fatalf("promote: %v", err)
	}
	if merged {
		t.Fatal("no contact existed; promotion must create, not merge")
	}

	links := e.linkTargetsOf(t, activity)
	if got, ok := links["contact"]; !ok || got != ids.UUID(contact.Id) {
		t.Errorf("after promotion the activity links contact=%v (present=%t), want the promoted "+
			"contact %v — the lead's history did not follow it", got, ok, ids.UUID(contact.Id))
	}
	if leftBehind, ok := links["lead"]; ok {
		t.Errorf("the activity still links lead=%v — the row was copied rather than converted, "+
			"which activity_link_shape forbids and which leaves the history in two places",
			leftBehind)
	}
}

// The merge case, and the one that would corrupt rather than merely lose: the
// activity was ALREADY linked to the contact the lead merges into — a reply
// captured against a contact we already knew. uq_activity_link would reject the
// converted duplicate, so the lead-side row is dropped and the contact keeps the
// link it had.
func TestPromotionDropsALeadLinkTheContactAlreadyHas(t *testing.T) {
	e := setupPromoteConsent(t)
	const shared = "known@example.test"

	// The contact exists first, and the activity is already on their timeline.
	existing, err := e.store.CreateContact(e.ctx, CreateContactInput{
		FullName: "Known Contact",
		Emails:   []ContactEmailInput{{Email: shared, EmailType: "work", IsPrimary: true}},
		Source:   "manual",
	})
	if err != nil {
		t.Fatalf("seed the incumbent contact: %v", err)
	}
	lead := e.seedLead(t, shared)
	activity := e.seedLeadActivity(t, lead, "Replied to our outreach")
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO activity_link (id, activity_id, entity_type, contact_id)
		 VALUES ($1, $2, 'contact', $3)`,
		ids.NewV7(), activity, ids.UUID(existing.Id)); err != nil {
		t.Fatal(err)
	}

	contact, merged, err := e.store.PromoteLead(e.ctx, lead, PromoteLeadInput{Trigger: "inbound_reply"})
	if err != nil {
		t.Fatalf("promote into the existing contact: %v", err)
	}
	if !merged {
		t.Fatal("a live contact holds this email; promotion must merge, not create")
	}

	links := e.linkTargetsOf(t, activity)
	if got, ok := links["contact"]; !ok || got != ids.UUID(contact.Id) {
		t.Errorf("the activity links contact=%v (present=%t), want %v", got, ok, ids.UUID(contact.Id))
	}
	if leftBehind, ok := links["lead"]; ok {
		t.Errorf("the activity still links lead=%v after the merge", leftBehind)
	}
}
