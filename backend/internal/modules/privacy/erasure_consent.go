// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// The consent half of an Art. 17 erasure: the live capabilities over the
// subject's consent record, which are secrets other contacts hold rather than
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
// the schema: the contact row survives, so 0048's ON DELETE CASCADE never
// fires, and an erased subject would keep accruing contact_consent,
// consent_event, audit and outbox rows through the exact capabilities this
// erasure certifies destroyed. That a grant is refused elsewhere is one probe,
// not a reason to leave the credential standing. Deleted rather than revoked,
// like the address and phone rows beside it — a revoked row still holds the
// contact link.
//
// TWO statements rather than one loop over a table list, and the difference is
// not style. This tree's coverage gates read SQL string LITERALS —
// piicoverage_test.go proves Art. 17 reaches each PII table, and
// tableownership_test.go proves a package writes only what it owns. A table
// name arriving through a variable is invisible to both, so the tidier loop
// turns two proven writes into two unproven ones and the gates go quietly
// green. A third capability is added here as a third statement.
func deleteConsentCapabilities(
	ctx context.Context, tx pgx.Tx, contactID ids.ContactID, emails []string, reason string,
) error {
	if _, err := tx.Exec(ctx, `DELETE FROM preference_token WHERE contact_id = $1`, contactID); err != nil {
		return fmt.Errorf("privacy: destroying the subject's preference-center token: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM consent_doi_token WHERE contact_id = $1`, contactID); err != nil {
		return fmt.Errorf("privacy: destroying the subject's double-opt-in token: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM confirm_token WHERE contact_id = $1`, contactID); err != nil {
		return fmt.Errorf("privacy: destroying the subject's confirm-details link: %w", err)
	}
	// DELETED, not revoked, and that distinction matters here more than on the
	// three above: the row carries the ADDRESS the link was written to, which
	// outlives the contact_id an anonymize-in-place erasure nulls. Revoking
	// would leave that address standing in a table nothing else scrubs.
	//
	// It is also the longest-lived capability the subject holds — 24 months,
	// where the preference token is 30 days — so a missed one is a working
	// bearer credential for an erased contact for the better part of two years.
	if _, err := tx.Exec(ctx, `DELETE FROM withdrawal_credential WHERE contact_id = $1`, contactID); err != nil {
		return fmt.Errorf("privacy: destroying the subject's withdrawal link: %w", err)
	}

	// Not a capability but the subject's own words: what they proposed as a
	// correction, in their own name and address. Deleted rather than kept as
	// evidence, because an unaccepted proposal is data the workspace was asked
	// to hold and never agreed anything about.
	if _, err := tx.Exec(ctx, `DELETE FROM contact_confirm_submission WHERE contact_id = $1`, contactID); err != nil {
		return fmt.Errorf("privacy: destroying the subject's confirm-page submissions: %w", err)
	}

	// The authorization record, and the one place here where DELETE would be
	// the wrong verb.
	//
	// communication_decision says why each message to this contact was
	// permitted. That is the controller's own accountability record under
	// Art. 5(2), and destroying it would erase the evidence that the sending
	// was lawful — leaving the installation unable to answer for messages it
	// has already sent. What must go is the part that identifies the subject:
	// the address it went to, and the link back to the contact row.
	//
	// So the address is tombstoned and the subject link cut, in place. The
	// verdict, the category, the reason and the ruleset survive as an
	// unattributed statistic about a send that happened.
	if _, err := tx.Exec(ctx, `
		UPDATE communication_decision
		   SET recipient_address = 'erased+' || id || '@example.invalid',
		       subject_id = NULL, subject_kind = NULL
		 WHERE subject_id = $1`, contactID); err != nil {
		return fmt.Errorf("privacy: retiring the subject's authorization decisions: %w", err)
	}

	// data_subject_request is the SAME SHAPE and the sharpest case of it: the
	// erasure being performed here is usually the answer to one of these rows.
	//
	// Destroying the case would leave the controller unable to show it answered
	// the request at all — including this one, and including a request it
	// REFUSED, where the record is what an appeal or a supervisory authority
	// would ask to see. Art. 5(2) wants that kept.
	//
	// So the identifying half goes and the accountability half stays. subject_ref
	// is the free-text identity somebody typed or the link's own reference, and
	// resolution is prose a colleague wrote that can name the subject or quote
	// them. The kind, the status, the dates and the receipt reference survive as
	// an unattributed record that a request of this kind arrived and was
	// answered by its deadline.
	//
	// The receipt reference stays deliberately: it names no subject — it is a
	// minted code — and it is what somebody quotes when asking what happened to
	// their request, which they may still do after the erasure.
	//
	// TOMBSTONED, not nulled. dsr_resolution_shape requires a closed case to
	// carry a resolution, and a null would refuse the erasure on exactly the
	// rows that matter most: the fulfilled and rejected ones, which is every
	// case an erasure is likely to find. The stand-in keeps the shape true
	// while the prose goes.
	if err := retireRightsCases(ctx, tx, contactID.UUID, fulfillingCase(reason)); err != nil {
		return err
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
	if err := clearRefusedSendReviews(ctx, tx, contactID.UUID, emails); err != nil {
		return err
	}
	if err := tombstoneExceptionExplanations(ctx, tx, contactID.UUID, emails); err != nil {
		return err
	}

	// A basis and a suppression are the opposite case: both exist only to say
	// something about THIS contact, so neither has a life after them. Deleted,
	// like the address rows beside them.
	if _, err := tx.Exec(ctx, `DELETE FROM communication_basis WHERE contact_id = $1`, contactID); err != nil {
		return fmt.Errorf("privacy: destroying the subject's communication bases: %w", err)
	}
	// BY ADDRESS AS WELL AS BY CONTACT. A machine-written stop — the hard
	// bounce consent/bouncesuppress.go records — deliberately carries no
	// contact_id, because the engine matches the contact arm before the address
	// arm and a contact-scoped row would refuse every address that record has.
	// Keyed only on contact_id this delete walks straight past those rows, and
	// an erased subject's address survives in plaintext in a table nothing will
	// ever clean.
	if _, err := tx.Exec(ctx, `
		DELETE FROM communication_suppression
		 WHERE contact_id = $1 OR lower(address) = ANY($2)`,
		contactID, lowerAll(emails)); err != nil {
		return fmt.Errorf("privacy: destroying the subject's suppressions: %w", err)
	}
	return nil
}

// lowerAll folds the subject's addresses for comparison, because a refusal
// records the address as the caller typed it and a contact who writes their own
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
// a contact — two records on one address, or none — is refused with the address
// recorded and no subject id, which is precisely the row a subject-keyed sweep
// would walk past and leave holding an erased contact's mailbox.
func clearRefusedSendReviews(ctx context.Context, tx pgx.Tx, contactID ids.UUID, addresses []string) error {
	if _, err := tx.Exec(ctx, `
		UPDATE communication_review
		   SET refusals = '[]'::jsonb
		 WHERE EXISTS (
		         SELECT 1 FROM jsonb_array_elements(refusals) AS refusal
		          WHERE refusal->>'subject_id' = $1
		             OR lower(refusal->>'address') = ANY($2))`,
		contactID.String(), lowerAll(addresses)); err != nil {
		return fmt.Errorf("privacy: clearing the subject from refused-send reviews: %w", err)
	}
	return nil
}

// tombstoneExceptionExplanations scrubs what a director WROTE about a subject
// while keeping the fact that they decided.
//
// The explanation is a rep's own sentence about a named contact — "she asked for
// this on the call" — so it is personal data an erasure destroys. What must
// survive is the accountable half: that somebody overrode a refusal, who they
// were, when, and under which reason code. None of that names the subject.
//
// TOMBSTONED, NOT DELETED, and the row's own trigger enforces the difference:
// it admits this exact string and refuses every other edit, so an erasure can
// remove the words and nobody can improve the account of the decision.
//
// Reached through the REVIEW, because an instruction names no subject directly
// — it answers a review, and the review is what named the recipients.
func tombstoneExceptionExplanations(ctx context.Context, tx pgx.Tx, contactID ids.UUID, addresses []string) error {
	if _, err := tx.Exec(ctx, `
		UPDATE communication_instruction
		   SET explanation = '[erased]'
		 WHERE explanation <> '[erased]'
		   AND review_id IN (
		         SELECT r.id FROM communication_review r
		          WHERE EXISTS (
		                  SELECT 1 FROM jsonb_array_elements(r.refusals) AS refusal
		                   WHERE refusal->>'subject_id' = $1
		                      OR lower(refusal->>'address') = ANY($2)))`,
		contactID.String(), lowerAll(addresses)); err != nil {
		return fmt.Errorf("privacy: scrubbing what a director wrote about the subject: %w", err)
	}
	return nil
}

// retireRightsCases takes the subject out of their own rights cases and leaves
// the record that the cases happened.
//
// ONE WRITER for both acts. The erasure and the contact/anonymize sweep must
// clear the same tables — a gate enforces it — and a retirement spelled twice
// is two answers to what an anonymized subject keeps.
//
// TOMBSTONED, not nulled. dsr_resolution_shape requires a closed case to carry
// a resolution, so a null would abort the whole act on exactly the closed cases
// it is most likely to find. The stand-in keeps the shape true while the prose,
// which a colleague wrote and which can name or quote the subject, goes.
// BOTH KEYS, because only one of them is reliably set.
//
// A case opened through the subject's own confirm link carries contact_id. One
// an officer opens by hand carries only subject_ref — CreateDSR never sets the
// link, even when the reference IS a contact uuid — so keying on contact_id
// alone left every officer-created case holding its subject forever. Matching
// the reference as well reaches those, and reaches a case opened before a merge
// whose link was never repointed.
//
// ONE ROW IS EXCLUDED BY NAME, and every other one waits for its lock.
//
// The erasure this runs inside is USUALLY being performed to fulfil one of
// these cases. That transaction holds its row FOR UPDATE across this call, so
// an unqualified update blocks on a lock its own caller holds and dies on the
// statement timeout — which is what happened, caught by the compose lane.
//
// SKIP LOCKED was the first fix and it was too wide. It also skipped a case an
// unrelated officer happened to be editing at that moment, and nothing retries:
// that subject would survive in a case for no reason anybody could see, and the
// erasure would report success. Naming the one row that CANNOT be waited for
// keeps the rest blocking, which is the correct behaviour — a moment's wait for
// an officer's save, then the retirement.
//
// The excluded row is not left naming its subject. finalizeErasureFulfil
// (consent/dsr.go) clears it in the same statement that closes it, which is the
// only place that can: no other transaction may touch a row this one holds.
func retireRightsCases(ctx context.Context, tx pgx.Tx, contactID ids.UUID, fulfilling ids.UUID) error {
	if _, err := tx.Exec(ctx, `
		UPDATE data_subject_request
		   SET subject_ref = 'erased+' || id,
		       resolution = CASE WHEN resolution IS NULL THEN NULL ELSE 'erased' END,
		       -- THE LINK SURVIVES on an OPEN case, and only there. A second
		       -- erasure case for one subject must still be fulfillable after
		       -- the first retires it, and the link is what FulfilErasure
		       -- resolves through once the reference is tombstoned. On a closed
		       -- case nothing will resolve it again, so the link goes.
		       contact_id = CASE WHEN status IN ('open', 'in_progress')
		                         THEN contact_id ELSE NULL END
		 WHERE id IN (SELECT id FROM data_subject_request
		               WHERE (contact_id = $1 OR subject_ref = $1::text)
		                 AND id IS DISTINCT FROM $2
		               FOR UPDATE)`, contactID, fulfilling); err != nil {
		return fmt.Errorf("privacy: retiring the subject's rights cases: %w", err)
	}
	return nil
}

// fulfillingCase answers which rights case this erasure is being run to fulfil,
// read off the reason its caller passed.
//
// "dsr:<uuid>" is the spelling FulfilErasure uses (consent/dsr.go), and it is
// the ONE row retireRightsCases cannot wait for: that transaction holds it
// locked across this whole call. Every other reason — an officer's manual
// erasure, a retention sweep — names no case, and the zero uuid excludes
// nothing.
func fulfillingCase(reason string) ids.UUID {
	const prefix = "dsr:"
	if !strings.HasPrefix(reason, prefix) {
		return ids.UUID{}
	}
	id, err := ids.Parse(strings.TrimPrefix(reason, prefix))
	if err != nil {
		return ids.UUID{}
	}
	return id
}
