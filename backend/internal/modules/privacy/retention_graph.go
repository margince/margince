// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// The time-based sweep's reach into the relationship graph (ADR-0078). It runs
// inside the retention engine's per-record transaction and is reached only from
// the person/anonymize action — a separate file, not a separate obligation.
//
// retentionSweepFiles in the PII-coverage gate lists this file alongside
// retention.go, so a table swept only here still counts as swept — and it stays
// OUT of erasureCascadeFiles, because a nightly sweep is never an answer to an
// Art. 17 request.

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// scrubPersonGraphTraces removes the anonymized subject from the relationship
// graph. Those structures hold the subject as surely as the person columns do,
// and the time-based sweep reaches them for the same reason the request-driven
// eraser does: an anonymized person who is still named on a participant row,
// still counted in an interaction edge, or still listed in an imported address
// book is not anonymized. This sweep is the path nobody asks for, which is
// exactly why it must not be the thinner one.
//
// subjectEmails and subjectName are the caller's, read before the
// anonymization overwrote them.
func scrubPersonGraphTraces(ctx context.Context, tx pgx.Tx, id ids.UUID, subjectEmails []string, subjectName string, linkedInHandles []string) error {
	// Delete then null, in that order and for the reason the
	// eraser documents: a participant row must name somebody, so a
	// row whose only identity is the subject cannot be blanked,
	// while one that also names a colleague is not the subject's
	// to remove.
	_, err := tx.Exec(ctx, `
		DELETE FROM activity_participant
		 WHERE user_id IS NULL
		   AND (person_id = $1 OR (address IS NOT NULL AND address = ANY($2)))`,
		id, subjectEmails)
	if err == nil {
		_, err = tx.Exec(ctx, `
			UPDATE activity_participant SET person_id = NULL, address = NULL, display_name = NULL
			 WHERE user_id IS NOT NULL
			   AND (person_id = $1 OR (address IS NOT NULL AND address = ANY($2)))`,
			id, subjectEmails)
	}
	if err == nil {
		_, err = tx.Exec(ctx,
			`DELETE FROM graph_interaction_edge WHERE person_id = $1`, id)
	}
	if err == nil {
		// Both endpoint columns: the swept party can stand on either end of a
		// contact↔contact edge, and the row re-identifies them from the graph
		// alone whichever side they are on.
		_, err = tx.Exec(ctx,
			`DELETE FROM graph_contact_edge WHERE person_a = $1 OR person_b = $1`, id)
	}
	if err == nil {
		// The SAME reach the request-driven eraser uses, by calling it rather
		// than by restating it. This path used to carry its own copy of the
		// predicate and the copy had drifted: no profile-URL arm, and an
		// employer compared only for equality where the eraser also matches a
		// longer name. Both gaps left a named third party's row standing after
		// the clock said it was gone, and the comment here claimed the reaches
		// were identical the whole time.
		//
		// This is the path nobody asks for, which is exactly why it must not be
		// the thinner one.
		err = deleteSubjectLinkedInGhosts(ctx, tx, ids.From[ids.PersonKind](id), subjectEmails, subjectName, linkedInHandles)
	}
	return err
}
