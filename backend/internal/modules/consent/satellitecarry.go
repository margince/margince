// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// Carrying a retiring subject's links and bases onto the record that survives
// them.
//
// contacts owns the merge and calls this inside its transaction (contacts's
// ConsentSatelliteCarrier seam, wired in compose). A merge says the two records
// are one human, so the links sitting in that human's inbox must act on the
// survivor: an unsubscribe press withdraws the survivor, a preference link
// opens the survivor's centre, a confirm link resolves again because its
// subject is live.
//
// MOVE, unlike stopcarry.go's copy. A stop is evidence of an act by the retired
// record and stays with it; a link is a capability the human holds, and a
// second copy on the survivor would be a second credential nobody was sent.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// PreferenceRevokedMergedIntoSurvivor marks a preference link that could not
// follow its subject onto the survivor as itself, because the survivor already
// holds the link for that address slot or the address stayed behind archived.
// The link still withdraws and still opens the survivor's centre; see
// resolvePreferenceTokenTx and legacyPreferenceTokenAsWithdrawal.
const PreferenceRevokedMergedIntoSurvivor = "merged_into_survivor"

// satelliteCarry is what one carry moved and dropped, per table, for the
// audit row.
type satelliteCarry struct {
	moved   map[string][]ids.UUID
	dropped map[string][]ids.UUID
}

// noteRows keeps a table out of the audit evidence when nothing of it moved.
func noteRows(into map[string][]ids.UUID, table string, rows []ids.UUID) {
	if len(rows) > 0 {
		into[table] = append(into[table], rows...)
	}
}

func (c satelliteCarry) total() int {
	n := 0
	for _, rows := range c.moved {
		n += len(rows)
	}
	return n
}

// CarrySatellitesTx implements contacts.ConsentSatelliteCarrier.
//
// Gated by admitACarry for the reason CarryStopsTx is: it is exported and
// rewrites whose links these are.
func (s *Store) CarrySatellitesTx(ctx context.Context, tx pgx.Tx, from, to commsauthz.StopSubject) error {
	if err := admitACarry(ctx, tx, from, to); err != nil {
		return err
	}
	carry := satelliteCarry{moved: map[string][]ids.UUID{}, dropped: map[string][]ids.UUID{}}
	if err := carrySubjectArmRows(ctx, tx, from, to, carry); err != nil {
		return err
	}
	// The remaining tables name contacts only, so a lead has none to carry.
	// admitACarry refuses contact to lead, so a contact source has a contact survivor.
	if !from.ContactID.IsZero() {
		if err := carryContactOnlyRows(ctx, tx, from.ContactID, to.ContactID, carry); err != nil {
			return err
		}
	}
	return auditSatelliteCarry(ctx, tx, from, to, carry)
}

// carrySubjectArmRows moves the two tables that can name a lead or a contact.
// One statement sets both columns, so a promotion clears lead_id in the same
// write that sets contact_id and the one-subject CHECK holds throughout.
//
// No uniqueness binds either table by subject: several withdrawal links per
// address is their shape, and both halves' lawful bases are kept.
func carrySubjectArmRows(ctx context.Context, tx pgx.Tx, from, to commsauthz.StopSubject, carry satelliteCarry) error {
	args := []any{
		zeroAsNull(from.ContactID.UUID), zeroAsNull(from.LeadID.UUID),
		zeroAsNull(to.ContactID.UUID), zeroAsNull(to.LeadID.UUID),
	}
	withdrawal, err := collectIDs(ctx, tx, `
		UPDATE withdrawal_credential SET contact_id = $3, lead_id = $4
		 WHERE ($1::uuid IS NOT NULL AND contact_id = $1) OR ($2::uuid IS NOT NULL AND lead_id = $2)
		RETURNING id`, args...)
	if err != nil {
		return fmt.Errorf("consent: carrying the withdrawal links: %w", err)
	}
	bases, err := collectIDs(ctx, tx, `
		UPDATE communication_basis SET contact_id = $3, lead_id = $4
		 WHERE ($1::uuid IS NOT NULL AND contact_id = $1) OR ($2::uuid IS NOT NULL AND lead_id = $2)
		RETURNING id`, args...)
	if err != nil {
		return fmt.Errorf("consent: carrying the recorded lawful bases: %w", err)
	}
	noteRows(carry.moved, "withdrawal_credential", withdrawal)
	noteRows(carry.moved, "communication_basis", bases)
	return nil
}

// carryContactOnlyRows moves the four tables that name a contact and nothing
// else.
func carryContactOnlyRows(ctx context.Context, tx pgx.Tx, from, to ids.ContactID, carry satelliteCarry) error {
	if err := carryPreferenceTokens(ctx, tx, from, to, carry); err != nil {
		return err
	}
	confirm, err := collectIDs(ctx, tx,
		`UPDATE confirm_token SET contact_id = $2 WHERE contact_id = $1 RETURNING id`, from, to)
	if err != nil {
		return fmt.Errorf("consent: carrying the confirm links: %w", err)
	}
	doi, err := collectIDs(ctx, tx,
		`UPDATE consent_doi_token SET contact_id = $2 WHERE contact_id = $1 RETURNING id`, from, to)
	if err != nil {
		return fmt.Errorf("consent: carrying the double-opt-in history: %w", err)
	}
	noteRows(carry.moved, "confirm_token", confirm)
	noteRows(carry.moved, "consent_doi_token", doi)
	return carryQualifyingEvents(ctx, tx, from, to, carry)
}

// carryPreferenceTokens keeps uq_preference_token_contact_address — one live
// link per (contact, address row), NULL included, expired rows counted — true
// on the survivor.
//
// A live link that would break it, or that names an address row the merge left
// behind archived, is marked merged_into_survivor rather than moved live. It
// still withdraws and still opens the survivor's centre, so nobody's link stops
// working; it just no longer holds the slot. Never merged_predecessor: the
// withdrawal path refuses that one.
func carryPreferenceTokens(ctx context.Context, tx pgx.Tx, from, to ids.ContactID, carry satelliteCarry) error {
	marked, err := collectIDs(ctx, tx, `
		UPDATE preference_token p
		   SET contact_id = $2, revoked_at = now(), revoked_reason = $3
		 WHERE p.contact_id = $1 AND p.revoked_at IS NULL
		   AND (EXISTS (SELECT 1 FROM preference_token h
		                 WHERE h.contact_id = $2 AND h.revoked_at IS NULL
		                   AND h.contact_email_id IS NOT DISTINCT FROM p.contact_email_id)
		     OR EXISTS (SELECT 1 FROM contact_email e
		                 WHERE e.id = p.contact_email_id
		                   AND (e.contact_id <> $2 OR e.archived_at IS NOT NULL)))
		RETURNING id`, from, to, PreferenceRevokedMergedIntoSurvivor)
	if err != nil {
		return fmt.Errorf("consent: retiring the preference links the survivor already holds: %w", err)
	}
	moved, err := collectIDs(ctx, tx,
		`UPDATE preference_token SET contact_id = $2 WHERE contact_id = $1 RETURNING id`, from, to)
	if err != nil {
		return fmt.Errorf("consent: carrying the preference links: %w", err)
	}
	noteRows(carry.moved, "preference_token", append(marked, moved...))
	return nil
}

// carryQualifyingEvents keeps one event per (contact, source record): where
// both halves recorded the same message or deal, the survivor's stands and the
// source's copy is dropped, because it states nothing the survivor's does not.
// Hand-recorded events name no source and always travel.
func carryQualifyingEvents(ctx context.Context, tx pgx.Tx, from, to ids.ContactID, carry satelliteCarry) error {
	dropped, err := collectIDs(ctx, tx, `
		DELETE FROM consent_qualifying_event a
		 WHERE a.contact_id = $1 AND a.source_entity_id IS NOT NULL
		   AND EXISTS (SELECT 1 FROM consent_qualifying_event b
		                WHERE b.contact_id = $2
		                  AND b.source_entity_type = a.source_entity_type
		                  AND b.source_entity_id = a.source_entity_id)
		RETURNING id`, from, to)
	if err != nil {
		return fmt.Errorf("consent: dropping qualifying events the survivor already records: %w", err)
	}
	moved, err := collectIDs(ctx, tx,
		`UPDATE consent_qualifying_event SET contact_id = $2 WHERE contact_id = $1 RETURNING id`, from, to)
	if err != nil {
		return fmt.Errorf("consent: carrying the qualifying events: %w", err)
	}
	noteRows(carry.moved, "consent_qualifying_event", moved)
	noteRows(carry.dropped, "consent_qualifying_event", dropped)
	return nil
}

// auditSatelliteCarry writes one row on the SURVIVOR, the record whose links
// now act differently, naming every row moved and where it came from.
// AuditEvent for the reason CarryStopsTx gives: the survivor held none of
// these rows before, so there is no before-image to record.
func auditSatelliteCarry(ctx context.Context, tx pgx.Tx, from, to commsauthz.StopSubject, carry satelliteCarry) error {
	if len(carry.moved) == 0 && len(carry.dropped) == 0 {
		return nil
	}
	entity, entityID := entityContact, to.ContactID.UUID
	if entityID.IsZero() {
		entity, entityID = entityLead, to.LeadID.UUID
	}
	_, err := storekit.AuditEventWithEvidence(ctx, tx, "update", entity, entityID,
		map[string]any{"consent_records_carried": carry.total()},
		map[string]any{
			"carried_from": subjectKey(from),
			"moved":        carry.moved,
			"dropped":      carry.dropped,
		})
	if err != nil {
		return fmt.Errorf("consent: auditing the carried links: %w", err)
	}
	return nil
}

// collectIDs runs a RETURNING id statement and reads every id it returned.
func collectIDs(ctx context.Context, tx pgx.Tx, sql string, args ...any) ([]ids.UUID, error) {
	rows, err := tx.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
}
