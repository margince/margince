// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// What the consent tables say about their subject, as opposed to what their
// one writer happens to do.
//
// Each held a row naming both a contact and a lead. Nothing then says which
// subject the consent governs, and the withdrawal path, the Art. 15 package
// and the suppression check each have to pick one.
//
// Raw SQL, because a statement the store would never emit is the case a
// constraint exists for. consentproof.go chooses one column at the call, so
// driving the store would re-confirm only that the store is careful.

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

// refusedBy names which rule refused a write, so a case cannot pass by being
// caught by a different one than it is about.
func refusedBy(err error) string {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return fmt.Sprintf("not a postgres error: %v", err)
	}
	return pgErr.ConstraintName
}

func TestAConsentRecordNamesOneSubject(t *testing.T) {
	e := setupChannelConsent(t)
	lead := seedLead(t, e, "Two Subjects", "two-subjects@example.test")
	for _, refused := range []struct {
		name       string
		constraint string
		statement  string
		args       []any
	}{
		{
			name:       "a consent event naming a contact and a lead",
			constraint: "consent_event_subject",
			statement: `INSERT INTO consent_event
				(contact_id, lead_id, purpose_id, new_state, source, captured_at, captured_by)
				VALUES ($1, $2, $3, 'granted', 'test', now(), 'human:x')`,
			args: []any{e.contact, lead, e.newsletter},
		},
		{
			name:       "a consent record naming a contact and a lead",
			constraint: "contact_consent_subject",
			statement: `INSERT INTO contact_consent (contact_id, lead_id, purpose_id)
				VALUES ($1, $2, $3)`,
			args: []any{e.contact, lead, e.newsletter},
		},
		{
			name:       "a consent event naming nobody",
			constraint: "consent_event_subject",
			statement: `INSERT INTO consent_event
				(purpose_id, new_state, source, captured_at, captured_by)
				VALUES ($1, 'granted', 'test', now(), 'human:x')`,
			args: []any{e.newsletter},
		},
	} {
		t.Run(refused.name, func(t *testing.T) {
			_, err := e.owner.Exec(context.Background(), refused.statement, refused.args...)
			if err == nil {
				t.Fatalf("the table accepted the row, so %s holds nothing", refused.constraint)
			}
			if got := refusedBy(err); got != refused.constraint {
				t.Errorf("refused by %q, want %q: the row is wrong for the reason this case is "+
					"about, and another constraint catching it means this one was never exercised",
					got, refused.constraint)
			}
		})
	}
}

// The admitted shapes, so what the constraint refuses is the second subject
// rather than the column. A rule that refused everything would read as green
// in the test above on its own.
func TestAConsentRecordStillAdmitsOneSubject(t *testing.T) {
	e := setupChannelConsent(t)
	lead := seedLead(t, e, "One Subject", "one-subject@example.test")
	for _, admitted := range []struct {
		name      string
		statement string
		args      []any
	}{
		{
			name: "an event about a contact",
			statement: `INSERT INTO consent_event
				(contact_id, purpose_id, new_state, source, captured_at, captured_by)
				VALUES ($1, $2, 'granted', 'test', now(), 'human:x')`,
			args: []any{e.contact, e.newsletter},
		},
		{
			name: "an event about a lead",
			statement: `INSERT INTO consent_event
				(lead_id, purpose_id, new_state, source, captured_at, captured_by)
				VALUES ($1, $2, 'granted', 'test', now(), 'human:x')`,
			args: []any{lead, e.newsletter},
		},
		{
			name:      "a record about a lead",
			statement: `INSERT INTO contact_consent (lead_id, purpose_id) VALUES ($1, $2)`,
			args:      []any{lead, e.newsletter},
		},
	} {
		t.Run(admitted.name, func(t *testing.T) {
			if _, err := e.owner.Exec(context.Background(), admitted.statement, admitted.args...); err != nil {
				t.Fatalf("the table refused a row with one subject: %v", err)
			}
		})
	}
}
