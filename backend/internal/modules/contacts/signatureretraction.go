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
	"fmt"

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
	if _, err := tx.Exec(ctx, `
		DELETE FROM contact_profile_field f
		 WHERE f.source_ref = $1 AND f.source = $2
		   AND NOT EXISTS (
		     SELECT 1 FROM ai_feedback
		      WHERE subject_type = 'contact' AND subject_id = f.contact_id
		        AND claim_kind = 'profile_field' AND verdict = 'corrected'
		        AND claim_key = encode(sha256(('profile_field:' || f.field)::bytea), 'hex'))`,
		"activity:"+activityID.String(), enrichSource); err != nil {
		return fmt.Errorf("contacts: retracting the narrowed message's signature fields: %w", err)
	}
	return nil
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
		// The MIRRORED column goes back to what the row it is undoing replaced,
		// which is what makes this a retraction rather than a deletion. A field
		// with a contact column (today: title) is displayed off that column, so
		// dropping only the sidecar row would take the evidence away and leave
		// the wrong title on the page — the visible half of the defect, kept.
		// superseded_value is the undo buffer applyObservedField fills, so it
		// holds whatever stood there before the signature overwrote it, and
		// NULL when nothing did.
		rows, err := tx.Query(ctx, `
			WITH wrong AS (
			    SELECT f.id, f.contact_id, f.field, f.value, f.superseded_value
			      FROM contact_profile_field f
			      JOIN activity a ON f.source_ref = 'activity:' || a.id::text
			     WHERE f.source = $1
			       AND NOT `+SenderPredicate("f.contact_id", "a")+`
			       AND NOT EXISTS (
			         SELECT 1 FROM ai_feedback
			          WHERE subject_type = 'contact' AND subject_id = f.contact_id
			            AND claim_kind = 'profile_field' AND verdict = 'corrected'
			            AND claim_key = encode(sha256(('profile_field:' || f.field)::bytea), 'hex'))
			), restored AS (
			    -- COMPARE AND SWAP, on the value the sidecar says this pass
			    -- wrote. A blind restore would undo an edit made AFTER the bad
			    -- enrichment: the machine writes "Partner", a human corrects it
			    -- to "CTO" through UpdateContact — which touches the column and
			    -- not this table — and a sweep that wrote superseded_value back
			    -- would take their answer away. Matching on f.value first means
			    -- a column somebody else has since moved is left exactly where
			    -- they put it, and the stale sidecar row is still withdrawn.
			    UPDATE contact p SET title = w.superseded_value
			      FROM wrong w
			     WHERE p.id = w.contact_id AND w.field = $2 AND p.archived_at IS NULL
			       AND p.title IS NOT DISTINCT FROM w.value
			)
			DELETE FROM contact_profile_field f USING wrong w WHERE f.id = w.id
			 RETURNING f.contact_id, f.field`,
			enrichSource, fieldTitle)
		if err != nil {
			return fmt.Errorf("contacts: retracting misattributed signature fields: %w", err)
		}
		touched, err := auditRetractions(ctx, tx, rows)
		if err != nil {
			return err
		}
		removed = touched
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

// auditRetractions writes the write shape for each contact the sweep touched.
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
func auditRetractions(ctx context.Context, tx pgx.Tx, rows pgx.Rows) (int64, error) {
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
				fieldKeyReason: "signature read off a message this contact did not send",
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
