// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// Qualifying a lead into an existing contact fills that contact. Its own
// history says so, and its event names that audit row.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAPromotionMergedIntoAContactIsToldOnTheContact(t *testing.T) {
	e := setupPromoteConsent(t)
	email := "lena@history.example"
	lead := e.seedLead(t, email)
	contactID := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO contact (id, full_name, source, captured_by) VALUES ($1, 'Lena Contact', 'manual', 'human:x')`, contactID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO contact_email (contact_id, email, email_type, is_primary, position, source, captured_by)
		 VALUES ($1, lower($2), 'work', true, 1, 'manual', 'human:x')`, contactID, email); err != nil {
		t.Fatal(err)
	}

	if _, merged, err := e.store.PromoteLead(e.ctx, lead, PromoteLeadInput{Trigger: "human_qualify"}); err != nil || !merged {
		t.Fatalf("promote: merged=%v err=%v, want a merge into the existing contact", merged, err)
	}

	var auditID ids.UUID
	var before, after map[string]any
	if err := e.owner.QueryRow(context.Background(),
		`SELECT id, before, after FROM audit_log
		  WHERE entity_type = 'contact' AND entity_id = $1 AND action = 'update'`, contactID).
		Scan(&auditID, &before, &after); err != nil {
		t.Fatalf("the contact's history holds no update for the fill: %v", err)
	}
	if after["converted_from_lead_id"] != lead.String() {
		t.Errorf("the contact's row names %v as the lead it came from, want %s", after["converted_from_lead_id"], lead.UUID)
	}
	if value, present := before["converted_from_lead_id"]; !present || value != nil {
		t.Errorf("before = %v, want the pointer held nothing", before)
	}
	var events int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM event_outbox WHERE envelope->>'type' = 'contact.updated' AND envelope->'trace'->>'audit_log_id' = $1`,
		auditID.String()).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if events != 1 {
		t.Errorf("%d contact.updated events name the contact's audit row, want exactly 1", events)
	}
}
