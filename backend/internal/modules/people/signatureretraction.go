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

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
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
