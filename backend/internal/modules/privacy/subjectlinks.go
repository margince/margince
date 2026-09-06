// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// The links that still reach a subject's record after it has stopped naming
// them.
//
// Its own file because it is its own hazard: everything else in the retention
// actions clears a COLUMN or a ROW, and these clear a URL somebody is holding in
// a mailbox — a thing outside the database, which goes on working until the row
// behind it is gone.

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// deleteSubjectBearerLinks clears every live link that still reaches this
// subject's record, and what one of them carried back.
//
// Three deletes rather than one statement because they are three tables, and one
// concern: a URL somebody holds in a mailbox that still works after the record
// has been anonymized.
func deleteSubjectBearerLinks(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	// The double-opt-in token goes with the addresses it was sent to. It is a
	// bearer secret whose only function is to authorise a consent GRANT for this
	// subject, so one left standing after an anonymization is a live invitation
	// to record a lawful basis for somebody the row no longer names. An
	// anonymized subject may lawfully return, which is what the suppression list
	// is for — but they return by being invited again, not by an old token in an
	// old mailbox still working.
	if _, err := tx.Exec(ctx, `DELETE FROM consent_doi_token WHERE person_id = $1`, id); err != nil {
		return err
	}
	// The confirm-details link goes for the same reason, and a stronger one: it
	// does not merely authorise a grant, it DISPLAYS the record. A link left live
	// would show an old mailbox the fields the anonymization has just emptied.
	if _, err := tx.Exec(ctx, `DELETE FROM confirm_token WHERE person_id = $1`, id); err != nil {
		return err
	}
	// And what came back through it, which is the subject's own name and address
	// in plaintext — exactly the content the anonymization just cleared from the
	// person row.
	_, err := tx.Exec(ctx, `DELETE FROM person_confirm_submission WHERE person_id = $1`, id)
	return err
}
