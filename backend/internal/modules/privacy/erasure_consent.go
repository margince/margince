// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// The consent half of an Art. 17 erasure: the live capabilities over the
// subject's consent record, which are secrets other people hold rather than
// data the record stores. Its own file beside erasure_attachments.go and
// erasure_channels.go, because the package splits an erasure by the kind of
// thing being destroyed and this is a kind of its own.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// deleteConsentCapabilities destroys every live capability over the subject's
// consent record: preference_token, the emailed List-Unsubscribe URL;
// consent_doi_token, the 72-hour double-opt-in secret in the same mailbox
// whose only function is to authorise a GRANT; and confirm_token, the link that
// DISPLAYS the record and carries a marketing answer back. Each is a bearer
// secret rather
// than a stored attribute, acting on an edge that binds a system principal, so
// every RBAC gate downstream passes.
//
// Anonymize-in-place is why erasure reaches them here rather than leaning on
// the schema: the person row survives, so 0048's ON DELETE CASCADE never
// fires, and an erased subject would keep accruing person_consent,
// consent_event, audit and outbox rows through the exact capabilities this
// erasure certifies destroyed. That a grant is refused elsewhere is one probe,
// not a reason to leave the credential standing. Deleted rather than revoked,
// like the address and phone rows beside it — a revoked row still holds the
// person link.
//
// TWO statements rather than one loop over a table list, and the difference is
// not style. This tree's coverage gates read SQL string LITERALS —
// piicoverage_test.go proves Art. 17 reaches each PII table, and
// tableownership_test.go proves a package writes only what it owns. A table
// name arriving through a variable is invisible to both, so the tidier loop
// turns two proven writes into two unproven ones and the gates go quietly
// green. A third capability is added here as a third statement.
func deleteConsentCapabilities(ctx context.Context, tx pgx.Tx, personID ids.PersonID) error {
	if _, err := tx.Exec(ctx, `DELETE FROM preference_token WHERE person_id = $1`, personID); err != nil {
		return fmt.Errorf("privacy: destroying the subject's preference-center token: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM consent_doi_token WHERE person_id = $1`, personID); err != nil {
		return fmt.Errorf("privacy: destroying the subject's double-opt-in token: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM confirm_token WHERE person_id = $1`, personID); err != nil {
		return fmt.Errorf("privacy: destroying the subject's confirm-details link: %w", err)
	}

	// Not a capability but the subject's own words: what they proposed as a
	// correction, in their own name and address. Deleted rather than kept as
	// evidence, because an unaccepted proposal is data the workspace was asked
	// to hold and never agreed anything about.
	if _, err := tx.Exec(ctx, `DELETE FROM person_confirm_submission WHERE person_id = $1`, personID); err != nil {
		return fmt.Errorf("privacy: destroying the subject's confirm-page submissions: %w", err)
	}

	// The authorization record, and the one place here where DELETE would be
	// the wrong verb.
	//
	// communication_decision says why each message to this person was
	// permitted. That is the controller's own accountability record under
	// Art. 5(2), and destroying it would erase the evidence that the sending
	// was lawful — leaving the installation unable to answer for messages it
	// has already sent. What must go is the part that identifies the subject:
	// the address it went to, and the link back to the person row.
	//
	// So the address is tombstoned and the subject link cut, in place. The
	// verdict, the category, the reason and the ruleset survive as an
	// unattributed statistic about a send that happened.
	if _, err := tx.Exec(ctx, `
		UPDATE communication_decision
		   SET recipient_address = 'erased+' || id || '@example.invalid',
		       subject_id = NULL, subject_kind = NULL
		 WHERE subject_id = $1`, personID); err != nil {
		return fmt.Errorf("privacy: retiring the subject's authorization decisions: %w", err)
	}

	// A basis and a suppression are the opposite case: both exist only to say
	// something about THIS person, so neither has a life after them. Deleted,
	// like the address rows beside them.
	if _, err := tx.Exec(ctx, `DELETE FROM communication_basis WHERE person_id = $1`, personID); err != nil {
		return fmt.Errorf("privacy: destroying the subject's communication bases: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM communication_suppression WHERE person_id = $1`, personID); err != nil {
		return fmt.Errorf("privacy: destroying the subject's suppressions: %w", err)
	}
	return nil
}
