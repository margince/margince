// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The other half of a signature enrichment: taking back what it wrote when the
// message it read is narrowed.
//
// Its own file rather than a tail on enrichsignature.go, because it answers a
// different question. That file asks what a signature block says about a contact;
// this asks what happens to the answer when the message stops being readable by
// the contacts the answer is shown to.

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// RetractSignatureFieldsTx removes the profile fields one message's signature
// wrote, for a message that has just been narrowed.
//
// The narrowing is what makes this owed: a field derived from a message the
// reader may no longer read is that message's content, restated on a record
// everybody can see. Deleting rather than narrowing, because a profile field
// carries no audience of its own — there is nowhere to put "this title is
// visible to fewer contacts than the contact it is on".
//
// THREE predicates, and each is here because matching on fewer deleted somebody's
// work in review:
//
//   - source_ref names the message. Necessary and nowhere near sufficient:
//     RestoreProfileField inherits the ref from the row it undoes, so a value a
//     contact restored still names the signature's message.
//   - source = capture_enrich is what the signature pass writes. A restore
//     writes human_restore over it, which is what tells the two apart.
//   - no live `corrected` verdict. A human correcting a field records it in
//     ai_feedback and touches neither the column nor this row, so a corrected
//     field is still source=capture_enrich and passes both tests above. The row
//     is what contact360 overlays that verdict onto, so deleting it takes the
//     correction off the screen with it. Same question refuseIfCorrected asks
//     before an undo, for the same reason.
//
// What survives is a field nobody has taken over: written by the signature pass,
// never restored, never corrected. That is the only kind this is entitled to.
func RetractSignatureFieldsTx(ctx context.Context, tx pgx.Tx, activityID ids.UUID) error {
	rows, err := tx.Query(ctx, `
		SELECT id FROM contact_profile_field WHERE source_ref = $1 AND source = $2`,
		"activity:"+activityID.String(), enrichSource)
	if err != nil {
		return fmt.Errorf("contacts: finding the narrowed message's signature fields: %w", err)
	}
	fields, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return fmt.Errorf("contacts: finding the narrowed message's signature fields: %w", err)
	}
	_, err = takeBackSignatureFieldsTx(ctx, tx, fields, "the mail it was read from was narrowed",
		func(ctx context.Context, tx pgx.Tx) (bool, error) { return sourceStillNarrowedTx(ctx, tx, activityID) })
	return err
}

// sourceStillNarrowedTx asks, under a share lock taken after the contacts, the
// order the signature writer takes them in, whether the message is still
// limited. A widening that committed since leaves the fields standing.
func sourceStillNarrowedTx(ctx context.Context, tx pgx.Tx, activityID ids.UUID) (bool, error) {
	var audience string
	err := tx.QueryRow(ctx, `SELECT audience FROM activity WHERE id = $1 FOR SHARE`, activityID).Scan(&audience)
	if errors.Is(err, pgx.ErrNoRows) {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("contacts: reading the narrowed message's audience: %w", err)
	}
	return audience != "workspace", nil
}

// RetractMisattributedSignatureFields takes back the fields an earlier pass
// wrote off a message the contact did not send.
//
// Before the sender predicate existed, a candidate was any contact the message
// REACHED, so a signature was applied to everyone a thread was linked to. The
// fix stops new ones; it cannot reach the rows already written, and those are
// the ones a rep is looking at — the whole visible damage of the defect is
// historical. Sweeping them here rather than in a migration keeps one spelling
// of "the sender" (senderPredicate) deciding both what may be written and what
// may stand, so the two can never disagree.
//
// It deletes only what the enrichment pass itself wrote and nobody has taken
// over — the same three predicates RetractSignatureFieldsTx explains at length,
// asked here of every row at once instead of one message's.
//
// Returns how many rows it removed, which the pass logs: a repair that silently
// removes nothing and a repair that silently removes everything look identical
// from outside, and the count is what tells them apart.
func (s *Store) RetractMisattributedSignatureFields(ctx context.Context) (int64, error) {
	if err := auth.Require(ctx, "contact", principal.ActionUpdate); err != nil {
		return 0, err
	}
	var removed int64
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT f.id FROM contact_profile_field f
			  JOIN activity a ON a.id = `+sourceRefActivityID+`
			 WHERE f.source = $1 AND NOT `+SenderPredicate("f.contact_id", "a"), enrichSource)
		if err != nil {
			return fmt.Errorf("contacts: finding misattributed signature fields: %w", err)
		}
		wrong, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
		if err != nil {
			return fmt.Errorf("contacts: finding misattributed signature fields: %w", err)
		}
		if removed, err = RetractSignatureFieldsByIDTx(ctx, tx, wrong,
			"signature read off a message this contact did not send"); err != nil {
			return err
		}
		// The watermark is cleared whether or not a field was deleted, and that
		// is the whole point of doing it separately. A misattributed message
		// that yielded NO field still advanced the cursor past itself, so a
		// contact whose valid signature arrived EARLIER would never be offered
		// again — an empty repair leaving a permanent gap, which is exactly the
		// under-recognition shape that reports success.
		//
		// The watermark said "this contact's mail has been read up to here", and
		// that reading was wrong. Clearing it lets the next pass look again, so
		// a contact whose only signature was somebody else's can still gain
		// their own from a later message.
		if _, err := tx.Exec(ctx, `
			DELETE FROM contact_signature_enrich_state st
			 USING activity a
			 WHERE st.activity_id = a.id AND NOT `+SenderPredicate("st.contact_id", "a")); err != nil {
			return fmt.Errorf("contacts: reopening the misread signature watermarks: %w", err)
		}
		return nil
	})
	return removed, err
}

// RetractSignatureFieldsByIDTx takes back the named signature fields. It
// removes the value each put on the contact and the evidence row. Each contact
// gets one audit row and one contact.updated event, with the reason.
//
// Only rows the pass wrote and no human corrected are taken back (the
// predicates RetractSignatureFieldsTx explains). A value a human has since
// changed stays, and only its evidence goes. The count it returns is logged.
func RetractSignatureFieldsByIDTx(ctx context.Context, tx pgx.Tx, fields []ids.UUID, reason string) (int64, error) {
	return takeBackSignatureFieldsTx(ctx, tx, fields, reason, nil)
}

// takeBackSignatureFieldsTx is RetractSignatureFieldsByIDTx with an optional
// question asked once the contacts are held: owed false takes nothing back.
//
// The contacts are locked first, in id order, then the evidence rows. That is
// the signature writer's order (applySignature), so the two wait instead of
// deadlocking.
func takeBackSignatureFieldsTx(ctx context.Context, tx pgx.Tx, fields []ids.UUID, reason string,
	owed func(context.Context, pgx.Tx) (bool, error),
) (int64, error) {
	if len(fields) == 0 {
		return 0, nil
	}
	if err := auth.Require(ctx, "contact", principal.ActionUpdate); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx, `
		SELECT c.id FROM contact c
		 WHERE c.id IN (SELECT contact_id FROM contact_profile_field WHERE id = ANY($1))
		 ORDER BY c.id
		 FOR UPDATE OF c`, fields); err != nil {
		return 0, fmt.Errorf("contacts: holding the contacts whose signature fields are taken back: %w", err)
	}
	if owed != nil {
		if due, err := owed(ctx, tx); err != nil || !due {
			return 0, err
		}
	}
	rows, err := tx.Query(ctx, `
		SELECT f.id, f.contact_id, f.field, f.value, f.superseded_value
		  FROM contact_profile_field f
		 WHERE f.id = ANY($1) AND f.source = $2
		   AND NOT EXISTS (
		     SELECT 1 FROM ai_feedback
		      WHERE subject_type = 'contact' AND subject_id = f.contact_id
		        AND claim_kind = 'profile_field' AND verdict = 'corrected'
		        AND claim_key = encode(sha256(('profile_field:' || f.field)::bytea), 'hex'))
		 ORDER BY f.id
		 FOR UPDATE OF f`, fields, enrichSource)
	if err != nil {
		return 0, fmt.Errorf("contacts: reading the signature fields to take back: %w", err)
	}
	wrong, err := pgx.CollectRows(rows, pgx.RowToStructByPos[retractedField])
	if err != nil {
		return 0, fmt.Errorf("contacts: reading the signature fields to take back: %w", err)
	}
	removing, err := withdrawValues(ctx, tx, wrong)
	if err != nil {
		return 0, err
	}
	deleted, err := tx.Query(ctx, `
		DELETE FROM contact_profile_field WHERE id = ANY($1) RETURNING contact_id, field`, removing)
	if err != nil {
		return 0, fmt.Errorf("contacts: removing the signature evidence: %w", err)
	}
	return auditRetractions(ctx, tx, deleted, reason)
}

// withdrawValues takes each field's value off its contact and returns the
// evidence rows to remove. The display goes back only when every name part
// did. A part a human typed since means the display is theirs too.
func withdrawValues(ctx context.Context, tx pgx.Tx, wrong []retractedField) ([]ids.UUID, error) {
	removing := make([]ids.UUID, 0, len(wrong))
	nameRestored := map[ids.ContactID]bool{}
	for _, field := range wrong {
		restored, err := field.withdrawValue(ctx, tx)
		if err != nil {
			return nil, err
		}
		if observedNameFields[field.Field] {
			previous, seen := nameRestored[field.ContactID]
			nameRestored[field.ContactID] = restored && (previous || !seen)
		}
		removing = append(removing, field.ID)
	}
	for contactID, restored := range nameRestored {
		if !restored {
			continue
		}
		if err := restoreSignatureDisplayTx(ctx, tx, contactID); err != nil {
			return nil, err
		}
	}
	return removing, nil
}

// retractedField is one evidence row being taken back, and what it put on the
// record.
type retractedField struct {
	ID         ids.UUID
	ContactID  ids.ContactID
	Field      string
	Value      string
	Superseded *string
}

// withdrawValue takes the field's value off the contact, as a compare and swap
// on that value. It reports whether the record still carried it. A human who
// typed "CTO" over the pass's "Partner" keeps "CTO".
//
// A mirrored column (title, first and last name) goes back to what the read
// replaced (superseded_value, NULL when nothing). A phone is a row of its own:
// the one this read added is archived, and the number it replaced comes back.
func (f retractedField) withdrawValue(ctx context.Context, tx pgx.Tx) (bool, error) {
	if f.Field == fieldPhone {
		return true, withdrawSignatureNumber(ctx, tx, f.ContactID, f.Value)
	}
	column, mirrored := observedFieldColumn(f.Field)
	if !mirrored {
		return true, nil
	}
	col := pgx.Identifier{column}.Sanitize()
	tag, err := tx.Exec(ctx, `
		UPDATE contact SET `+col+` = $3
		 WHERE id = $1 AND archived_at IS NULL AND `+col+` IS NOT DISTINCT FROM $2`,
		f.ContactID, f.Value, f.Superseded)
	if err != nil {
		return false, fmt.Errorf("contacts: putting back the %s a signature replaced: %w", f.Field, err)
	}
	return tag.RowsAffected() > 0, nil
}

// restoreSignatureDisplayTx puts back the display name followSignatureDisplay
// replaced. It acts only while the newest display change is the pass's own and
// the contact still shows it. A display anybody changed since stays.
func restoreSignatureDisplayTx(ctx context.Context, tx pgx.Tx, contactID ids.ContactID) error {
	var current string
	var previous *string
	err := tx.QueryRow(ctx, `
		SELECT c.full_name, last.before->>'full_name'
		  FROM contact c
		  JOIN LATERAL (
		       SELECT before, after, actor_id FROM audit_log
		        WHERE entity_type = 'contact' AND entity_id = c.id AND after ? 'full_name'
		        ORDER BY occurred_at DESC, id DESC
		        LIMIT 1) last ON true
		 WHERE c.id = $1 AND c.archived_at IS NULL
		   AND last.actor_id = $2 AND last.after->>'full_name' = c.full_name`,
		contactID, enrichCapturedBy).Scan(&current, &previous)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("contacts: reading the display a signature replaced: %w", err)
	}
	if previous == nil || strings.TrimSpace(*previous) == "" {
		return nil
	}
	_, err = rewriteFullNameTx(ctx, tx, contactID, current, *previous)
	return err
}

// withdrawSignatureNumber archives the live number a signature read added, and
// revives the number it replaced. A number a human has since saved by hand
// carries their source and stays.
func withdrawSignatureNumber(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, phone string) error {
	rows, err := tx.Query(ctx, `
		UPDATE contact_phone SET archived_at = now()
		 WHERE contact_id = $1 AND phone = $2 AND source = $3 AND archived_at IS NULL
		RETURNING superseded_phone_id`, contactID, phone, enrichSource)
	if err != nil {
		return fmt.Errorf("contacts: archiving the number a signature read added: %w", err)
	}
	replaced, err := pgx.CollectRows(rows, pgx.RowTo[*ids.UUID])
	if err != nil {
		return fmt.Errorf("contacts: archiving the number a signature read added: %w", err)
	}
	for _, id := range replaced {
		if id == nil {
			continue
		}
		if _, err := revivePhoneTx(ctx, tx, contactID, *id); err != nil {
			return err
		}
	}
	return nil
}

// sourceRefActivityID is the activity a profile field's source_ref names, as a
// uuid, so the retraction joins activity through its primary key.
//
// Comparing text instead (`f.source_ref = 'activity:' || a.id::text`) cannot use
// any index: it renders every activity id as text and hashes the lot, on every
// pass, to find the few hundred a signature was read from.
//
// The pattern admits exactly the refs that comparison could match and no other.
// A uuid's text form is its canonical lowercase spelling, which is also how the
// writer renders the ref (enrichsignature.go), so anything else — an uppercase
// or unhyphenated spelling, a ref of another kind such as site_read — could
// never have joined, and here reads as NULL and joins nothing. The CASE is what
// keeps the cast from ever seeing such a ref: an unguarded cast would raise on
// the first one and fail the whole pass.
const sourceRefActivityID = `CASE WHEN f.source_ref ~ '^activity:[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'
			           THEN substr(f.source_ref, 10)::uuid END`

// auditRetractions writes the write shape for each contact a retraction touched.
//
// A field this pass wrote landed with an audit row and a contact.updated event;
// taking it back is the same size of change to the same record, and a repair
// that left no trace would make a title change on its own between two reads of
// the history. Grouped per contact rather than per field, because that is the
// mutation a reader sees: one contact, the fields that were withdrawn from them.
//
// NAMED, NOT QUOTED, like the writer it undoes: the values came out of somebody
// else's message, and audit_log outlives the erasure of the record they came
// from.
func auditRetractions(ctx context.Context, tx pgx.Tx, rows pgx.Rows, reason string) (int64, error) {
	byContact := map[ids.ContactID][]string{}
	var order []ids.ContactID
	var removed int64
	for rows.Next() {
		var contactID ids.ContactID
		var field string
		if err := rows.Scan(&contactID, &field); err != nil {
			rows.Close()
			return 0, fmt.Errorf("contacts: reading the retracted fields: %w", err)
		}
		if _, seen := byContact[contactID]; !seen {
			order = append(order, contactID)
		}
		byContact[contactID] = append(byContact[contactID], field)
		removed++
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("contacts: reading the retracted fields: %w", err)
	}
	for _, contactID := range order {
		fields := byContact[contactID]
		auditID, err := storekit.AuditWithEvidence(ctx, tx, "update", entityContact, contactID.UUID,
			map[string]any{}, map[string]any{},
			map[string]any{
				auditKeySource: enrichSource,
				auditKeyFields: fields,
				fieldKeyReason: reason,
			})
		if err != nil {
			return 0, err
		}
		if err := storekit.EmitEvent(ctx, tx, auditID, contactID.UUID,
			crmcontracts.PublicEventContactUpdated{
				ChangedFields: map[string]any{auditKeyFields: fields, auditKeySource: enrichSource},
			}); err != nil {
			return 0, err
		}
	}
	return removed, nil
}
