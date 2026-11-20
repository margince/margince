// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// subjectDealIDsSQL names the deals a contact stands in as a stakeholder, whether
// or not the link is still live. The Art. 15 export and the Art. 17 erasure of a
// legal review's free text both ask this one question with $1 as the contact.
const subjectDealIDsSQL = `SELECT r.deal_id FROM relationship r
	WHERE r.kind = 'deal_stakeholder' AND r.contact_id = $1 AND r.deal_id IS NOT NULL`

// subjectLiveDealIDsSQL is the same question for the Art. 15 export, which lists
// only the deals the subject stands in now. Erasure keeps the wider set above.
const subjectLiveDealIDsSQL = subjectDealIDsSQL + ` AND r.archived_at IS NULL`

// clearSubjectLegalReviewText empties the note and the reviewer's comments of every
// legal review on a deal the subject is a stakeholder of, and tombstones each row it
// cleared.
//
// The text is a colleague's words about the deal and may name the subject, and
// nothing can tell the sentence that does from the one that does not. The row stays:
// its outcome settles the deal's stage gate, and the deal is not the subject's.
// Both erasure acts call this, because neither deletes the deal.
func clearSubjectLegalReviewText[ID ids.UUID | ids.ContactID](
	ctx context.Context, tx pgx.Tx, contactID ID, reason, cause string,
) error {
	rows, err := tx.Query(ctx, `
		UPDATE legal_review SET note = NULL, comments = NULL, version = version + 1, updated_at = now()
		 WHERE deal_id IN (`+subjectDealIDsSQL+`)
		   AND (note IS NOT NULL OR comments IS NOT NULL)
		RETURNING id`, contactID)
	if err != nil {
		return fmt.Errorf("privacy: clearing the legal review text on the subject's deals: %w", err)
	}
	cleared, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return fmt.Errorf("privacy: clearing the legal review text on the subject's deals: %w", err)
	}
	return tombstoneCollateralScrubs(ctx, tx, "legal_review", cleared, reason, cause)
}
