// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// The claims read OUT of a subject's conversations, scoped to the subject.
//
// Its own file for the size reason the timeline has one, and it belongs to both
// destructive engines rather than to either: the Art. 17 spine and the
// retention sweep's contact/anonymize call the same function, which is what the
// anonymize-parity gate requires.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// deleteConversationClaimsFor drops what a colleague or an agent read out of
// one subject's conversations — a commitment, an open question, a decision —
// and, on every row, the VERBATIM snippet it was read from.
//
// Two arms, and both are needed. The first is the subject's own claims; the
// second is claims about ANYBODY that quote the subject's messages, because the
// quotation is the subject's words wherever the claim was filed. A link walk
// alone would leave a colleague's claim standing with a sentence the subject
// wrote inside it.
//
// One spelling for both acts, typed over the two id forms the callers hold,
// like the reply verdicts above. The schema means it to go by cascade —
// contact_id is NOT NULL and the foreign key cascades — but erasure ANONYMIZES
// the contact row in place, so nothing ever fires it; the identical reason
// transcript_read is deleted by statement.
func deleteConversationClaimsFor[ID ids.UUID | ids.ContactID](ctx context.Context, tx pgx.Tx, contactID ID) error {
	if _, err := tx.Exec(ctx, `
		DELETE FROM conversation_claim
		 WHERE contact_id = $1
		    OR source_activity_id IN (SELECT l.activity_id FROM activity_link l WHERE l.contact_id = $1)`,
		contactID); err != nil {
		return fmt.Errorf("privacy: clearing the subject's conversation claims: %w", err)
	}
	return nil
}
