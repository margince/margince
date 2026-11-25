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
	"github.com/margince/margince/backend/internal/shared/apperrors"
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
	expired map[string][]ids.UUID
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
	carry := satelliteCarry{moved: map[string][]ids.UUID{}, dropped: map[string][]ids.UUID{}, expired: map[string][]ids.UUID{}}
	if err := carrySubjectArmRows(ctx, tx, from, to, carry); err != nil {
		return err
	}
	if err := carryQualifyingEvents(ctx, tx, from, to, carry); err != nil {
		return err
	}
	// admitACarry refuses contact to lead, so a contact source has a contact survivor.
	if !from.ContactID.IsZero() {
		if err := carryContactOnlyRows(ctx, tx, from.ContactID, to.ContactID, carry); err != nil {
			return err
		}
	}
	if !from.LeadID.IsZero() && !to.LeadID.IsZero() {
		if err := carryConfirmLinks(ctx, tx, subjectColumnLead, from.LeadID.UUID, to.LeadID.UUID, carry); err != nil {
			return err
		}
	}
	return auditSatelliteCarry(ctx, tx, from, to, carry)
}

// BindRequestsToAddressesTx implements contacts.ConsentSatelliteCarrier. Gated
// by admitACarry like the carry it comes before.
//
// A stop for the whole subject and a withdrawal are bound the same way as a
// request. An anonymize then pins each only to its own record's addresses. The
// trigger bind_requests_on_address_move binds the same rows when a merge runs
// on code that predates this call. The open privacy requests move here too,
// while the addresses they were filed for are still the merged-away record's.
func (s *Store) BindRequestsToAddressesTx(ctx context.Context, tx pgx.Tx, from, to ids.ContactID) error {
	if err := admitACarry(ctx, tx, commsauthz.ContactStopSubject(from), commsauthz.ContactStopSubject(to)); err != nil {
		return err
	}
	pair := []ids.UUID{from.UUID, to.UUID}
	if err := lockSubjectsInOrder(ctx, tx, pair...); err != nil {
		return err
	}
	if err := storekit.LockSubjectsStopAddresses(ctx, tx, pair...); err != nil {
		return err
	}
	sub, err := consentSubject(RecordInput{ContactID: from})
	if err != nil {
		return err
	}
	if err := bindQualifyingAddresses(ctx, tx, sub, to.UUID); err != nil {
		return err
	}
	if err := carryOpenRightsRequests(ctx, tx, from, to); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `
		UPDATE communication_suppression s SET recorded_for_addresses = `+storekit.RequestAddressesSQL("s.contact_id")+`
		 WHERE s.contact_id = ANY($1) AND s.address IS NULL AND s.revoked_at IS NULL
		   AND s.recorded_for_addresses IS NULL`, pair); err != nil {
		return fmt.Errorf("consent: binding the subject-wide stops to their addresses: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE contact_consent c SET recorded_for_addresses = `+storekit.RequestAddressesSQL("c.contact_id")+`
		 WHERE c.contact_id = ANY($1) AND c.state = 'withdrawn' AND c.recorded_for_addresses IS NULL`, pair); err != nil {
		return fmt.Errorf("consent: binding the withdrawals to their addresses: %w", err)
	}
	return nil
}

// ErrRightsRequestBeingAnswered refuses a merge while a privacy request on the
// merged-away contact is being fulfilled.
var ErrRightsRequestBeingAnswered = fmt.Errorf("%w: a privacy request on the merged-away contact is being answered; "+
	"merge again once it is closed", apperrors.ErrConflict)

// carryOpenRightsRequests moves the privacy requests still being answered onto
// the survivor, so the answer reaches the data the survivor now holds. That is
// a case linked to the retired contact, or one an officer opened naming its id
// (resolveDSRSubject reads either). A closed request stays on the record it
// acted on. Each moved request's audit row names the merge and the addresses of
// the record it was filed on, read before any address moves.
//
// The requests are locked without waiting. A fulfilment holds its request
// while the erasure runs in a second transaction that needs the contact this
// merge already holds. The database cannot see that cycle.
func carryOpenRightsRequests(ctx context.Context, tx pgx.Tx, from, to ids.ContactID) error {
	args := pgx.NamedArgs{carrySourceArg: from, carryTargetArg: to}
	open, err := collectIDs(ctx, tx, `SELECT id FROM data_subject_request
 WHERE (contact_id = @source OR (contact_id IS NULL AND subject_ref = @source::text))
   AND status IN ('open', 'in_progress') ORDER BY id FOR UPDATE NOWAIT`, args)
	if storekit.IsLockTimeout(err) {
		return ErrRightsRequestBeingAnswered
	}
	if err != nil || len(open) == 0 {
		return err
	}
	var filedFor []string
	if err := tx.QueryRow(ctx, `SELECT `+storekit.RequestAddressesSQL("@source::uuid"), args).Scan(&filedFor); err != nil {
		return fmt.Errorf("consent: reading the addresses the requests were filed for: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE data_subject_request SET contact_id = @target WHERE id = ANY(@requests)`,
		pgx.NamedArgs{carryTargetArg: to, "requests": open}); err != nil {
		return fmt.Errorf("consent: carrying the open privacy requests: %w", err)
	}
	return storekit.AuditEach(ctx, tx, "update", "data_subject_request", open,
		map[string]any{contactIDKey: from},
		map[string]any{contactIDKey: to, "merged_from": from, "filed_for_addresses": filedFor})
}

// carrySubjectArmRows moves the two tables that can name a lead or a contact.
// One statement sets both columns, so a promotion clears lead_id in the same
// write that sets contact_id and the one-subject CHECK holds throughout.
//
// No uniqueness binds either table by subject: several withdrawal links per
// address is their shape, and both halves' lawful bases are kept.
func carrySubjectArmRows(ctx context.Context, tx pgx.Tx, from, to commsauthz.StopSubject, carry satelliteCarry) error {
	args := pgx.NamedArgs{
		carryFromContactArg: zeroAsNull(from.ContactID.UUID), carryFromLeadArg: zeroAsNull(from.LeadID.UUID),
		"to_contact": zeroAsNull(to.ContactID.UUID), "to_lead": zeroAsNull(to.LeadID.UUID),
	}
	withdrawal, err := collectIDs(ctx, tx, `UPDATE withdrawal_credential SET contact_id = @to_contact, lead_id = @to_lead
 WHERE `+carrySubjectPredicate+` RETURNING id`, args)
	if err != nil {
		return fmt.Errorf("consent: carrying the withdrawal links: %w", err)
	}
	bases, err := collectIDs(ctx, tx, `UPDATE communication_basis SET contact_id = @to_contact, lead_id = @to_lead
 WHERE `+carrySubjectPredicate+` RETURNING id`, args)
	if err != nil {
		return fmt.Errorf("consent: carrying the recorded lawful bases: %w", err)
	}
	noteRows(carry.moved, "withdrawal_credential", withdrawal)
	noteRows(carry.moved, "communication_basis", bases)
	return nil
}

// carryContactOnlyRows moves the tables that name a contact and nothing else.
func carryContactOnlyRows(ctx context.Context, tx pgx.Tx, from, to ids.ContactID, carry satelliteCarry) error {
	if err := carryPreferenceTokens(ctx, tx, from, to, carry); err != nil {
		return err
	}
	if err := carryConfirmLinks(ctx, tx, subjectColumnContact, from.UUID, to.UUID, carry); err != nil {
		return err
	}
	doi, err := collectIDs(ctx, tx,
		`UPDATE consent_doi_token SET contact_id = @target WHERE `+contactCarrySource+` RETURNING id`, pgx.NamedArgs{carrySourceArg: from, carryTargetArg: to})
	if err != nil {
		return fmt.Errorf("consent: carrying the double-opt-in history: %w", err)
	}
	noteRows(carry.moved, "consent_doi_token", doi)
	return nil
}

// confirmLinkCarries are, per subject column, the statement that expires the
// merged-away record's live confirm links and the one that moves them all.
var confirmLinkCarries = map[string][2]string{
	subjectColumnContact: {
		`UPDATE confirm_token SET expires_at = GREATEST(now(), issued_at + interval '1 microsecond')
 WHERE contact_id = @source AND consumed_at IS NULL AND expires_at > now() RETURNING id`,
		`UPDATE confirm_token SET contact_id = @target WHERE contact_id = @source RETURNING id`,
	},
	subjectColumnLead: {
		`UPDATE confirm_token SET expires_at = GREATEST(now(), issued_at + interval '1 microsecond')
 WHERE lead_id = @source AND consumed_at IS NULL AND expires_at > now() RETURNING id`,
		`UPDATE confirm_token SET lead_id = @target WHERE lead_id = @source RETURNING id`,
	},
}

// carryConfirmLinks moves the merged-away record's confirm links onto the
// survivor, and first expires each one still live. An answer through a link
// acts on the record it names. The link was mailed for the retired record
// alone, so it gets no say over the survivor, and the person asks for a new one.
func carryConfirmLinks(ctx context.Context, tx pgx.Tx, subjectColumn string, from, to ids.UUID, carry satelliteCarry) error {
	statements, ok := confirmLinkCarries[subjectColumn]
	if !ok {
		return fmt.Errorf("consent: no confirm-link carry for the subject column %q", subjectColumn)
	}
	args := pgx.NamedArgs{carrySourceArg: from, carryTargetArg: to}
	expired, err := collectIDs(ctx, tx, statements[0], args)
	if err != nil {
		return fmt.Errorf("consent: expiring the merged-away record's confirm links: %w", err)
	}
	moved, err := collectIDs(ctx, tx, statements[1], args)
	if err != nil {
		return fmt.Errorf("consent: carrying the confirm links: %w", err)
	}
	noteRows(carry.expired, "confirm_token", expired)
	noteRows(carry.moved, "confirm_token", moved)
	return nil
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
 UPDATE preference_token p SET contact_id=@target, revoked_at=now(), revoked_reason=@reason
 WHERE p.contact_id=@source AND `+preferenceCarryRetires+` RETURNING id`,
		pgx.NamedArgs{carrySourceArg: from, carryTargetArg: to, fieldReason: PreferenceRevokedMergedIntoSurvivor, "moving_emails": []ids.UUID{}})
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
//
// The dropped copy's addresses join the kept row first: both records saw the
// same message, so the request covers the addresses of each. NULL on either
// side covers every address and stays NULL.
func carryQualifyingEvents(ctx context.Context, tx pgx.Tx, from, to commsauthz.StopSubject, carry satelliteCarry) error {
	source, err := consentSubject(RecordInput{ContactID: from.ContactID, LeadID: from.LeadID})
	if err != nil {
		return err
	}
	target, err := consentSubject(RecordInput{ContactID: to.ContactID, LeadID: to.LeadID})
	if err != nil {
		return err
	}
	return carryQualifyingSubjectEvents(ctx, tx, source, target, carry)
}

func carryQualifyingSubjectEvents(ctx context.Context, tx pgx.Tx, source, target subject, carry satelliteCarry) error {
	if err := bindQualifyingAddresses(ctx, tx, source); err != nil {
		return err
	}
	if err := bindQualifyingAddresses(ctx, tx, target); err != nil {
		return err
	}
	fromColumn := pgx.Identifier{source.column}.Sanitize()
	toColumn := pgx.Identifier{target.column}.Sanitize()
	args := pgx.NamedArgs{carrySourceArg: source.id, carryTargetArg: target.id}
	if _, err := tx.Exec(ctx, `
 UPDATE consent_qualifying_event b SET recorded_for_addresses = CASE
   WHEN b.recorded_for_addresses IS NULL OR a.recorded_for_addresses IS NULL THEN NULL
   ELSE ARRAY(SELECT DISTINCT x FROM unnest(b.recorded_for_addresses || a.recorded_for_addresses) AS x ORDER BY x) END,
 recorded_for_entities = CASE
   WHEN b.recorded_for_entities IS NULL AND a.recorded_for_entities IS NULL THEN NULL
   ELSE ARRAY(SELECT DISTINCT x FROM unnest(coalesce(b.recorded_for_entities,'{}'::uuid[]) ||
     coalesce(a.recorded_for_entities,'{}'::uuid[])) AS x ORDER BY x) END
   FROM consent_qualifying_event a
  WHERE b.`+toColumn+` = @target AND a.`+fromColumn+` = @source AND a.source_entity_id IS NOT NULL
    AND b.source_entity_type = a.source_entity_type AND b.source_entity_id = a.source_entity_id`,
		args); err != nil {
		return fmt.Errorf("consent: joining the addresses of a qualifying event both records hold: %w", err)
	}
	// A link naming a duplicate names the survivor's copy instead, since the
	// link moves to the survivor. A grant cannot be rewritten and stays on the
	// source, so the request it answered stays there with it.
	if _, err := tx.Exec(ctx, `
 UPDATE confirm_token r SET subject_request_id = b.id
   FROM consent_qualifying_event a JOIN consent_qualifying_event b
     ON b.`+toColumn+` = @target AND b.source_entity_type = a.source_entity_type
    AND b.source_entity_id = a.source_entity_id
  WHERE r.subject_request_id = a.id AND a.`+fromColumn+` = @source AND a.source_entity_id IS NOT NULL`, args); err != nil {
		return fmt.Errorf("consent: pointing a link at the request the survivor keeps: %w", err)
	}
	dropped, err := collectIDs(ctx, tx, `
 DELETE FROM consent_qualifying_event a WHERE a.`+fromColumn+`=@source AND `+qualifyingCarryDuplicate(target.column)+`
   AND NOT `+answeredByAGrant+` RETURNING id`, args)
	if err != nil {
		return fmt.Errorf("consent: dropping qualifying events the survivor already records: %w", err)
	}
	moved, err := collectIDs(ctx, tx,
		`UPDATE consent_qualifying_event a SET contact_id = @contact, lead_id = @lead
 WHERE a.`+fromColumn+` = @source AND NOT (`+qualifyingCarryDuplicate(target.column)+` AND `+answeredByAGrant+`)
 RETURNING id`, pgx.StrictNamedArgs{
			entityContact: zeroAsNull(subjectKindID(target, entityContact)), entityLead: zeroAsNull(subjectKindID(target, entityLead)),
			carrySourceArg: source.id, carryTargetArg: target.id,
		})
	if err != nil {
		return fmt.Errorf("consent: carrying the qualifying events: %w", err)
	}
	noteRows(carry.moved, "consent_qualifying_event", moved)
	noteRows(carry.dropped, "consent_qualifying_event", dropped)
	return nil
}

// answeredByAGrant matches a qualifying event `a` a grant names as the request
// it answered.
const answeredByAGrant = `EXISTS (SELECT 1 FROM consent_event ce WHERE ce.subject_request_id = a.id)`

// auditSatelliteCarry writes one row on the SURVIVOR, the record whose links
// now act differently, naming every row moved and where it came from.
// AuditEvent for the reason CarryStopsTx gives: the survivor held none of
// these rows before, so there is no before-image to record.
func auditSatelliteCarry(ctx context.Context, tx pgx.Tx, from, to commsauthz.StopSubject, carry satelliteCarry) error {
	if len(carry.moved) == 0 && len(carry.dropped) == 0 && len(carry.expired) == 0 {
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
			"expired":      carry.expired,
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
