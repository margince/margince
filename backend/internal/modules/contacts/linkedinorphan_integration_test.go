// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"testing"

	"github.com/jackc/pgx/v5"
)

// seedSuggestedAndreas leaves one suggestion: a contact spelled without
// the umlaut at the employer the export names.
func (e *dedupeEnv) seedSuggestedAndreas(t *testing.T) {
	t.Helper()
	andreas := e.seedVisibleContact(t, "Andreas Muller")
	e.employ(t, andreas, e.seedAcmeCompany(t))
	e.importAndMatch(t)
}

func TestASuggestionWhoseContactIsArchivedIsReleasedToUnmatched(t *testing.T) {
	e := setupDedupe(t)
	e.seedSuggestedAndreas(t)

	status, contact := e.ghostStatus(t, suggestedGhost)
	if status != "suggested" || contact == nil {
		t.Fatalf("fixture: ghost is %q with contact %v, want a suggestion", status, contact)
	}
	ctx := e.as()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE contact SET archived_at = now() WHERE id = $1`, *contact)
		return err
	}); err != nil {
		t.Fatalf("archiving the suggested contact: %v", err)
	}

	if _, err := e.store.MatchLinkedInConnections(e.as(), e.rep); err != nil {
		t.Fatalf("matching: %v", err)
	}

	status, contact = e.ghostStatus(t, suggestedGhost)
	if status != "unmatched" || contact != nil {
		t.Errorf("an orphaned suggestion is %q with contact %v, want unmatched and no contact", status, contact)
	}
	var audits int
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT count(*) FROM audit_log a JOIN linkedin_connection c ON c.id = a.entity_id
			 WHERE a.entity_type = 'linkedin_connection' AND c.full_name = $1`, suggestedGhost).Scan(&audits)
	}); err != nil {
		t.Fatalf("reading the audit trail: %v", err)
	}
	if audits == 0 {
		t.Error("releasing the suggestion wrote no audit row")
	}
}

func TestReleasingOrphansLeavesConfirmedAndRefusedRowsAlone(t *testing.T) {
	e := setupDedupe(t)
	e.seedSuggestedAndreas(t)
	_, contact := e.ghostStatus(t, suggestedGhost)
	ctx := e.as()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE linkedin_connection SET match_status = 'confirmed' WHERE matched_contact_id = $1`, *contact); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE contact SET archived_at = now() WHERE id = $1`, *contact)
		return err
	}); err != nil {
		t.Fatalf("confirming then archiving: %v", err)
	}

	if _, err := e.store.MatchLinkedInConnections(e.as(), e.rep); err != nil {
		t.Fatalf("matching: %v", err)
	}

	status, kept := e.ghostStatus(t, suggestedGhost)
	if status != "confirmed" || kept == nil || *kept != *contact {
		t.Errorf("a confirmed row is %q with contact %v, want it unchanged", status, kept)
	}
}
