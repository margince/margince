// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// seedSuggestedAndreas leaves one suggestion: a contact spelled without
// the umlaut at the employer the export names.
func (e *dedupeEnv) seedSuggestedAndreas(t *testing.T) {
	t.Helper()
	andreas := e.seedVisibleContact(t, "Andreas Muller")
	e.employ(t, andreas, e.seedAcmeCompany(t))
	e.importAndMatch(t)
}

// seedForeignGhost adds a suggestion of another member's network that names
// the contact, so a sweep for the importer has a row it must not touch.
func (e *dedupeEnv) seedForeignGhost(t *testing.T, contact ids.UUID) ids.UUID {
	t.Helper()
	member := e.seedMember(t, "Other Member")
	id := ids.NewV7()
	ctx := e.as()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO linkedin_connection
			    (id, owner_user_id, full_name, normalized_name, normalized_company,
			     matched_contact_id, match_status, source, synced_at)
			VALUES ($1, $2, 'Foreign Ghost', 'foreign ghost', '', $3, 'suggested', 'csv_export', now())`,
			id, member, contact)
		return err
	}); err != nil {
		t.Fatalf("seeding the other member's ghost: %v", err)
	}
	return id
}

func (e *dedupeEnv) statusOf(t *testing.T, id ids.UUID) (string, *ids.UUID) {
	t.Helper()
	var status string
	var contact *ids.UUID
	ctx := e.as()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT match_status, matched_contact_id FROM linkedin_connection WHERE id = $1`,
			id).Scan(&status, &contact)
	}); err != nil {
		t.Fatalf("reading connection %s: %v", id, err)
	}
	return status, contact
}

func TestASuggestionWhoseContactIsArchivedIsReleasedToUnmatched(t *testing.T) {
	e := setupDedupe(t)
	e.seedSuggestedAndreas(t)
	_, contact := e.ghostStatus(t, suggestedGhost)
	if contact == nil {
		t.Fatal("fixture: the ghost has no suggested contact")
	}
	foreign := e.seedForeignGhost(t, *contact)
	if _, err := e.store.ArchiveContact(e.as(), ids.From[ids.ContactKind](*contact), nil); err != nil {
		t.Fatalf("archiving the suggested contact: %v", err)
	}

	if _, err := e.store.MatchLinkedInConnections(e.as(), e.rep); err != nil {
		t.Fatalf("matching: %v", err)
	}

	status, kept := e.ghostStatus(t, suggestedGhost)
	if status != "unmatched" || kept != nil {
		t.Errorf("an orphaned suggestion is %q with contact %v, want unmatched and no contact", status, kept)
	}
	if status, kept := e.statusOf(t, foreign); status != "suggested" || kept == nil {
		t.Errorf("another member's suggestion is %q with contact %v, want it left alone", status, kept)
	}
	var audits int
	ctx := e.as()
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

func TestAMergeMovesASuggestionToTheSurvivor(t *testing.T) {
	e := setupDedupe(t)
	e.seedSuggestedAndreas(t)
	ghost := e.ghostID(t)
	_, source := e.ghostStatus(t, suggestedGhost)
	survivor := e.seedVisibleContact(t, "Andreas Survivor")

	if _, err := e.store.MergeContact(e.as(), ids.From[ids.ContactKind](*source), survivor, nil); err != nil {
		t.Fatalf("merging: %v", err)
	}
	if _, err := e.store.MatchLinkedInConnections(e.as(), e.rep); err != nil {
		t.Fatalf("matching: %v", err)
	}

	status, contact := e.statusOf(t, ghost)
	if status != "suggested" || contact == nil || *contact != survivor.UUID {
		t.Errorf("after the merge the suggestion is %q for %v, want suggested for the survivor %s", status, contact, survivor)
	}
}

func TestReleasingOrphansLeavesConfirmedAndRefusedRowsAlone(t *testing.T) {
	e := setupDedupe(t)
	e.seedSuggestedAndreas(t)
	_, contact := e.ghostStatus(t, suggestedGhost)
	rejected := ids.NewV7()
	ctx := e.as()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE linkedin_connection SET match_status = 'confirmed' WHERE matched_contact_id = $1`, *contact); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO linkedin_connection
			    (id, owner_user_id, full_name, normalized_name, normalized_company,
			     match_status, source, synced_at)
			VALUES ($1, $2, 'Refused Ghost', 'refused ghost', '', 'rejected', 'csv_export', now())`, rejected, e.rep)
		return err
	}); err != nil {
		t.Fatalf("seeding a confirmed and a refused row: %v", err)
	}
	if _, err := e.store.ArchiveContact(e.as(), ids.From[ids.ContactKind](*contact), nil); err != nil {
		t.Fatalf("archiving: %v", err)
	}

	if _, err := e.store.MatchLinkedInConnections(e.as(), e.rep); err != nil {
		t.Fatalf("matching: %v", err)
	}

	if status, kept := e.ghostStatus(t, suggestedGhost); status != "confirmed" || kept == nil || *kept != *contact {
		t.Errorf("a confirmed row is %q with contact %v, want it unchanged", status, kept)
	}
	if status, _ := e.statusOf(t, rejected); status != "rejected" {
		t.Errorf("a refused row is %q, want rejected", status)
	}
}
