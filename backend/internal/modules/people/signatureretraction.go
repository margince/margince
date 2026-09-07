// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// The other half of a signature enrichment: taking back what it wrote when the
// message it read is narrowed.
//
// Its own file rather than a tail on enrichsignature.go, because it answers a
// different question. That file asks what a signature block says about a person;
// this asks what happens to the answer when the message stops being readable by
// the people the answer is shown to.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
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
// visible to fewer people than the person it is on".
//
// THREE predicates, and each is here because matching on fewer deleted somebody's
// work in review:
//
//   - source_ref names the message. Necessary and nowhere near sufficient:
//     RestoreProfileField inherits the ref from the row it undoes, so a value a
//     person restored still names the signature's message.
//   - source = capture_enrich is what the signature pass writes. A restore
//     writes human_restore over it, which is what tells the two apart.
//   - no live `corrected` verdict. A human correcting a field records it in
//     ai_feedback and touches neither the column nor this row, so a corrected
//     field is still source=capture_enrich and passes both tests above. The row
//     is what person360 overlays that verdict onto, so deleting it takes the
//     correction off the screen with it. Same question refuseIfCorrected asks
//     before an undo, for the same reason.
//
// What survives is a field nobody has taken over: written by the signature pass,
// never restored, never corrected. That is the only kind this is entitled to.
func RetractSignatureFieldsTx(ctx context.Context, tx pgx.Tx, activityID ids.UUID) error {
	if _, err := tx.Exec(ctx, `
		DELETE FROM person_profile_field f
		 WHERE f.source_ref = $1 AND f.source = $2
		   AND NOT EXISTS (
		     SELECT 1 FROM ai_feedback
		      WHERE subject_type = 'person' AND subject_id = f.person_id
		        AND claim_kind = 'profile_field' AND verdict = 'corrected'
		        AND claim_key = encode(sha256(('profile_field:' || f.field)::bytea), 'hex'))`,
		"activity:"+activityID.String(), enrichSource); err != nil {
		return fmt.Errorf("people: retracting the narrowed message's signature fields: %w", err)
	}
	return nil
}

// RetractMisattributedSignatureFields takes back the fields an earlier pass
// wrote off a message the person did not send.
//
// Before the sender predicate existed, a candidate was any person the message
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
	if err := auth.Require(ctx, "person", principal.ActionUpdate); err != nil {
		return 0, err
	}
	var removed int64
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		// The MIRRORED column goes back to what the row it is undoing replaced,
		// which is what makes this a retraction rather than a deletion. A field
		// with a person column (today: title) is displayed off that column, so
		// dropping only the sidecar row would take the evidence away and leave
		// the wrong title on the page — the visible half of the defect, kept.
		// superseded_value is the undo buffer applyObservedField fills, so it
		// holds whatever stood there before the signature overwrote it, and
		// NULL when nothing did.
		tag, err := tx.Exec(ctx, `
			WITH wrong AS (
			    SELECT f.id, f.person_id, f.field, f.superseded_value
			      FROM person_profile_field f
			      JOIN activity a ON f.source_ref = 'activity:' || a.id::text
			     WHERE f.source = $1
			       AND NOT `+SenderPredicate("f.person_id", "a")+`
			       AND NOT EXISTS (
			         SELECT 1 FROM ai_feedback
			          WHERE subject_type = 'person' AND subject_id = f.person_id
			            AND claim_kind = 'profile_field' AND verdict = 'corrected'
			            AND claim_key = encode(sha256(('profile_field:' || f.field)::bytea), 'hex'))
			), restored AS (
			    UPDATE person p SET title = w.superseded_value
			      FROM wrong w
			     WHERE p.id = w.person_id AND w.field = $2 AND p.archived_at IS NULL
			)
			DELETE FROM person_profile_field f USING wrong w WHERE f.id = w.id`,
			enrichSource, fieldTitle)
		if err != nil {
			return fmt.Errorf("people: retracting misattributed signature fields: %w", err)
		}
		removed = tag.RowsAffected()
		if removed == 0 {
			return nil
		}
		// The watermark said "this person's mail has been read up to here", and
		// that reading was wrong. Clearing it lets the next pass look again, so
		// a contact whose only signature was somebody else's can still gain
		// their own from a later message.
		if _, err := tx.Exec(ctx, `
			DELETE FROM person_signature_enrich_state st
			 USING activity a
			 WHERE st.activity_id = a.id AND NOT `+SenderPredicate("st.person_id", "a")); err != nil {
			return fmt.Errorf("people: reopening the misread signature watermarks: %w", err)
		}
		return nil
	})
	return removed, err
}
