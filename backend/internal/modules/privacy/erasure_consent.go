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
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// deleteConsentCapabilities destroys every live capability over the subject's
// consent record: preference_token, the emailed List-Unsubscribe URL;
// withdrawal_credential, the longer-lived link that replaced it on that header;
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
func deleteConsentCapabilities(ctx context.Context, tx pgx.Tx, personID ids.PersonID, emails []string) error {
	if _, err := tx.Exec(ctx, `DELETE FROM preference_token WHERE person_id = $1`, personID); err != nil {
		return fmt.Errorf("privacy: destroying the subject's preference-center token: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM consent_doi_token WHERE person_id = $1`, personID); err != nil {
		return fmt.Errorf("privacy: destroying the subject's double-opt-in token: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM confirm_token WHERE person_id = $1`, personID); err != nil {
		return fmt.Errorf("privacy: destroying the subject's confirm-details link: %w", err)
	}
	// DELETED, not revoked, and that distinction matters here more than on the
	// three above: the row carries the ADDRESS the link was written to, which
	// outlives the person_id an anonymize-in-place erasure nulls. Revoking
	// would leave that address standing in a table nothing else scrubs.
	//
	// It is also the longest-lived capability the subject holds — 24 months,
	// where the preference token is 30 days — so a missed one is a working
	// bearer credential for an erased person for the better part of two years.
	if _, err := tx.Exec(ctx, `DELETE FROM withdrawal_credential WHERE person_id = $1`, personID); err != nil {
		return fmt.Errorf("privacy: destroying the subject's withdrawal link: %w", err)
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

	// A REVIEW IS UNFINISHED WORK, not accountability evidence, and that is why
	// it is scrubbed rather than kept whole like the decision above.
	//
	// The decision records a message that WAS sent and why it was permitted,
	// which the controller must be able to answer for. A review records a
	// message that was refused and never went — there is nothing to answer for,
	// and what it holds is a snapshot of somebody's addresses and the reasons
	// they were refused, which is exactly the material an erasure destroys.
	//
	// The row survives with its refusals emptied rather than being deleted: the
	// work may still be in front of a human, and a row vanishing under them
	// leaves a queue pointing at nothing. What is left says a send was refused
	// and no longer says who for.
	if err := clearRefusedSendReviews(ctx, tx, personID.UUID, emails); err != nil {
		return err
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

// lowerAll folds the subject's addresses for comparison, because a refusal
// records the address as the caller typed it and a person who writes their own
// mail in mixed case must still be erased from it.
func lowerAll(values []string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		out = append(out, strings.ToLower(strings.TrimSpace(v)))
	}
	return out
}

// clearRefusedSendReviews empties what a refused-send review recorded about one
// subject.
//
// ONE SPELLING FOR BOTH ACTS. The eraser and the anonymizer must clear the same
// tables, or a subject anonymized rather than erased keeps their addresses on
// every review that named them — after an operator was told the record was
// anonymized. Two copies of this query is how that drift starts.
//
// SCRUBBED, NOT DELETED, and here the review parts company with the decision
// beside it. A decision records a message that WAS sent and why it was
// permitted, which the controller must be able to answer for under Art. 5(2).
// A review records one that was refused and never went: there is nothing to
// answer for, and what it holds is a snapshot of somebody's addresses. The row
// survives with its refusals emptied because the work may still be in front of
// a human, and a row vanishing under them leaves a queue pointing at nothing.
//
// BY ADDRESS AS WELL AS BY SUBJECT. A recipient the engine could not resolve to
// a person — two records on one address, or none — is refused with the address
// recorded and no subject id, which is precisely the row a subject-keyed sweep
// would walk past and leave holding an erased person's mailbox.
func clearRefusedSendReviews(ctx context.Context, tx pgx.Tx, personID ids.UUID, addresses []string) error {
	if _, err := tx.Exec(ctx, `
		UPDATE communication_review
		   SET refusals = '[]'::jsonb
		 WHERE EXISTS (
		         SELECT 1 FROM jsonb_array_elements(refusals) AS refusal
		          WHERE refusal->>'subject_id' = $1
		             OR lower(refusal->>'address') = ANY($2))`,
		personID.String(), lowerAll(addresses)); err != nil {
		return fmt.Errorf("privacy: clearing the subject from refused-send reviews: %w", err)
	}
	return nil
}
