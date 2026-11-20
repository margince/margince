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

// redactSubjectLegalReviewText replaces the subject's name and addresses in the
// note and comments of legal reviews on deals they are or were a stakeholder of.
// Each changed row gets a tombstone.
//
// The text is a colleague's prose about the deal and is shared with its other
// stakeholders. Only what names the subject goes, as with an unlinked note, using
// the same pattern from the same helper. Both erasure acts call this, as neither
// deletes the deal.
func redactSubjectLegalReviewText(
	ctx context.Context, tx pgx.Tx, contactID ids.UUID, emails []string, displayName, reason, cause string,
) error {
	// With no pattern there is nothing to match on. The text goes whole, because
	// what names the subject cannot be told apart from the rest.
	pattern := subjectTextPattern(emails, displayName)
	rows, err := tx.Query(ctx, `
		UPDATE legal_review SET note = CASE WHEN $2 = '' THEN NULL ELSE regexp_replace(note, $2, $3, 'gi') END,
		       comments = CASE WHEN $2 = '' THEN NULL ELSE regexp_replace(comments, $2, $3, 'gi') END
		 WHERE deal_id IN (`+subjectDealIDsSQL+`)
		   AND (($2 = '' AND (note IS NOT NULL OR comments IS NOT NULL))
		        OR ($2 <> '' AND (note IS DISTINCT FROM regexp_replace(note, $2, $3, 'gi')
		                          OR comments IS DISTINCT FROM regexp_replace(comments, $2, $3, 'gi'))))
		RETURNING id`, contactID, pattern, erasedTextMention)
	if err != nil {
		return fmt.Errorf("privacy: redacting the subject from legal review text: %w", err)
	}
	changed, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return fmt.Errorf("privacy: redacting the subject from legal review text: %w", err)
	}
	return tombstoneCollateralScrubs(ctx, tx, "legal_review", changed, reason, cause)
}
