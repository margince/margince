// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// meetingCopyReason is the archive reason a retired meeting copy records.
const meetingCopyReason = "meeting_duplicate"

// meetingCopySurvivorKey names, in the archive evidence, the row the copy
// folded into.
const meetingCopySurvivorKey = "survivor"

// meetingCopyCarriedGuestsKey lists, in the archive evidence, the guest rows
// the fold added to the survivor.
const meetingCopyCarriedGuestsKey = "carried_guests"

// meetingCopyFoldedFromKey names, in the survivor's audit evidence, the copy
// whose document links it gained.
const meetingCopyFoldedFromKey = "folded_from"

// RetireMeetingCopyTx archives a meeting stored twice and moves its identities
// to the row that stays. The caller has proved the two rows are one meeting
// and that retiring the copy loses nothing. Only a system job or a calendar
// sync may call it: the fold acts on a colleague's row, which no human grant
// covers. The audit row names the survivor, so unarchiving undoes it.
func (s *Store) RetireMeetingCopyTx(ctx context.Context, tx pgx.Tx, duplicate, survivor ids.ActivityID) error {
	if err := requireFoldActor(ctx, "retires a meeting copy"); err != nil {
		return err
	}
	if err := TransferIdentities(ctx, tx, duplicate, survivor); err != nil {
		return err
	}
	carried, err := carryMissingParticipantsTx(ctx, tx, duplicate, survivor)
	if err != nil {
		return err
	}
	if err := carryMeetingDocumentLinksTx(ctx, tx, duplicate, survivor); err != nil {
		return err
	}
	p := storekit.NewPatch()
	p.Set("archived_at", nil, s.now().UTC())
	lock, err := storekit.LockRow(ctx, tx, "activity", duplicate.UUID, storekit.LiveOnly)
	if err != nil {
		return fmt.Errorf("activities: locking a meeting copy: %w", err)
	}
	if err := p.ApplyLocked(ctx, tx, lock); err != nil {
		return fmt.Errorf("activities: archiving a meeting copy: %w", err)
	}
	evidence := map[string]any{
		evidenceReasonKey: meetingCopyReason, meetingCopySurvivorKey: survivor.String(), meetingCopyCarriedGuestsKey: carried,
	}
	auditID, err := storekit.AuditWithEvidence(ctx, tx, "archive", "activity", duplicate.UUID, p.Before(), p.After(), evidence)
	if err != nil {
		return err
	}
	archived := crmcontracts.PublicEventActivityArchived{}
	archived.Imported, archived.ImportRunId = storekit.ImportMark(ctx)
	return storekit.EmitEvent(ctx, tx, auditID, duplicate.UUID, archived)
}

// requireFoldActor admits a system job or any connector sync to a write on a
// colleague's meeting row, which no human grant covers. verb completes "only a
// system job or a connector sync ...".
func requireFoldActor(ctx context.Context, verb string) error {
	actor, ok := principal.Actor(ctx)
	if !ok || (actor.Type != principal.PrincipalSystem && actor.Type != principal.PrincipalConnector) {
		return fmt.Errorf("activities: only a system job or a connector sync %s: %w", verb, apperrors.ErrPermissionDenied)
	}
	return nil
}

// meetingDocumentSource starts the identity of a meeting's notes-document link
// row, followed by the meeting's id. Capture writes the rows under it
// (capture.meetingDocumentSystem); the spelling is shared by convention, since
// neither module imports the other.
const meetingDocumentSource = "meeting:"

// carryMeetingDocumentLinksTx moves the copy's notes and transcript links to the
// survivor, re-keyed under the survivor's id so a later sync of either calendar
// finds them there. A document the survivor already holds stays with the copy,
// which is archived. Each move is audited with the meeting it left, and the
// survivor announces the documents it gained.
func carryMeetingDocumentLinksTx(ctx context.Context, tx pgx.Tx, duplicate, survivor ids.ActivityID) error {
	rows, err := tx.Query(ctx, `
		UPDATE attachment d
		   SET entity_id = $2::uuid, activity_id = $2::uuid, external_source_id = $3 || $2::uuid::text
		 WHERE d.entity_type = 'activity' AND d.entity_id = $1::uuid AND d.category = '`+categoryLink+`'
		   AND d.archived_at IS NULL AND d.external_source_id = $3 || $1::uuid::text
		   AND NOT EXISTS (SELECT 1 FROM attachment s
		                    WHERE s.external_source_id = $3 || $2::uuid::text AND s.external_part_id = d.external_part_id)
		RETURNING d.id`, duplicate, survivor, meetingDocumentSource)
	if err != nil {
		return fmt.Errorf("activities: moving a meeting copy's document links: %w", err)
	}
	moved, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return fmt.Errorf("activities: moving a meeting copy's document links: %w", err)
	}
	if len(moved) == 0 {
		return nil
	}
	for _, id := range moved {
		if _, err := storekit.Audit(ctx, tx, "update", "attachment", id,
			map[string]any{"entity_id": duplicate.String()}, map[string]any{"entity_id": survivor.String()}); err != nil {
			return fmt.Errorf("activities: auditing a moved document link: %w", err)
		}
	}
	// The survivor gained documents: announced on it, as a later sync that
	// finds new notes announces them.
	auditID, err := storekit.AuditEventWithEvidence(ctx, tx, "update", "activity", survivor.UUID,
		map[string]any{"meeting_documents_added": len(moved)}, map[string]any{meetingCopyFoldedFromKey: duplicate.String()})
	if err != nil {
		return fmt.Errorf("activities: auditing the documents a meeting copy handed on: %w", err)
	}
	return storekit.EmitEvent(ctx, tx, auditID, survivor.UUID, crmcontracts.PublicEventActivityUpdated{})
}

// carryMissingParticipantsTx copies onto the survivor each party of the copy
// it does not list yet. A party is matched by address, else seat, else contact.
// The survivor's guest list and the seats it holds busy stay whole.
func carryMissingParticipantsTx(ctx context.Context, tx pgx.Tx, duplicate, survivor ids.ActivityID) ([]ids.UUID, error) {
	rows, err := tx.Query(ctx, `
		INSERT INTO activity_participant (activity_id, user_id, contact_id, address, address_domain_folded, channel_user_id,
		                                  role, display_name, roster_seat, response, past_party_cap)
		SELECT $2, d.user_id, d.contact_id, d.address, d.address_domain_folded, d.channel_user_id,
		       d.role, d.display_name, d.roster_seat, d.response, d.past_party_cap
		  FROM activity_participant d
		 WHERE d.activity_id = $1
		   AND NOT EXISTS (SELECT 1 FROM activity_participant s WHERE s.activity_id = $2 AND CASE
		         WHEN d.address IS NOT NULL THEN lower(s.address) = lower(d.address)
		         WHEN d.user_id IS NOT NULL THEN s.user_id = d.user_id
		         WHEN d.contact_id IS NOT NULL THEN s.contact_id = d.contact_id
		         ELSE s.channel_user_id = d.channel_user_id END)
		ON CONFLICT DO NOTHING
		RETURNING id`, duplicate, survivor)
	if err != nil {
		return nil, fmt.Errorf("activities: carrying a meeting copy's guests: %w", err)
	}
	carried, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return nil, fmt.Errorf("activities: carrying a meeting copy's guests: %w", err)
	}
	return carried, nil
}

// IsMeetingCopyArchive reports whether an archive's audit evidence records a
// meeting copy folded into another row. Its identities and guests moved to
// that row, so unarchiving it alone would bring back a second, hollow meeting.
func IsMeetingCopyArchive(evidence []byte) bool {
	var recorded map[string]any
	if json.Unmarshal(evidence, &recorded) != nil {
		return false
	}
	reason, ok := recorded[evidenceReasonKey].(string)
	return ok && reason == meetingCopyReason
}
