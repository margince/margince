// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"reflect"
	"testing"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A FactsOnly read keeps each row's mail facts and reads none of what a screen
// draws beside a row. The same rows read both ways agree on the facts.
func TestAFactsOnlyReadKeepsTheMailFactsAndReadsNoAffordances(t *testing.T) {
	e := setupPromises(t)
	note := e.seedLinkedNote(t, e.rep)
	mail := ids.NewV7()
	e.exec(t, `INSERT INTO activity (id, kind, direction, subject, occurred_at, source, captured_by, counterparty_email)
		VALUES ($1, 'email', 'inbound', 'Can you send the quote?', now(), 'gmail', 'connector:gmail:x', 'buyer@customer.test')`, mail)
	e.exec(t, `INSERT INTO activity_link (id, activity_id, entity_type, contact_id)
		SELECT $1, $2, 'contact', contact_id FROM activity_link WHERE activity_id = $3`, ids.NewV7(), mail, note)
	store := NewStore(database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws)))
	wanted := []ids.ActivityID{ids.From[ids.ActivityKind](note), ids.From[ids.ActivityKind](mail)}

	full, _, err := store.ListActivities(e.asEditor(), ListActivitiesInput{IDs: wanted})
	if err != nil {
		t.Fatal(err)
	}
	facts, _, err := store.ListActivities(e.asEditor(), ListActivitiesInput{IDs: wanted, FactsOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(full) != 2 || len(facts) != 2 {
		t.Fatalf("read %d rows in full and %d as facts, want both seeded rows each time", len(full), len(facts))
	}
	for i := range full {
		if full[i].Id != facts[i].Id {
			t.Fatalf("row %d is %s in full and %s as facts, want one order", i, full[i].Id, facts[i].Id)
		}
		if ids.UUID(full[i].Id) == mail && facts[i].EmailSummary == nil {
			t.Errorf("the facts read of the mail carries no summary, so it kept no mail facts")
		}
		if full[i].Writable == nil {
			t.Errorf("the full read of %s carries no writable flag, so the comparison below proves nothing", full[i].Id)
		}
		if facts[i].Writable != nil || facts[i].Archivable != nil || facts[i].CalendarManaged != nil {
			t.Errorf("the facts read of %s stamped affordances: writable=%v archivable=%v calendar=%v",
				facts[i].Id, facts[i].Writable, facts[i].Archivable, facts[i].CalendarManaged)
		}
		if !reflect.DeepEqual(full[i].EmailSummary, facts[i].EmailSummary) {
			t.Errorf("the mail facts of %s differ: full %+v, facts %+v", full[i].Id, full[i].EmailSummary, facts[i].EmailSummary)
		}
	}
}
