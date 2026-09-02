// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// The planning half of an Art. 17 erasure: what a rep wrote down about the
// subject on their own weekly plan. Its own file beside erasure_consent.go and
// erasure_approvals.go, because the package splits an erasure by the kind of
// thing being destroyed and a commitment is a kind of its own — neither a
// message, nor a capability, nor a field on the record.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// redactWorkingRecords clears the subject's name where it survives inside
// somebody else's working record: an automation run addressed to them, and a
// commitment a colleague wrote about them on their own weekly plan.
//
// The two are one step because they answer one question — what does an erasure
// owe a record that is not the subject's and not about them, but names them in
// passing — and because ErasePerson reads as a sequence of such questions
// rather than a list of tables.
func redactWorkingRecords(ctx context.Context, tx pgx.Tx, subject ids.PersonID, emails []string) error {
	if err := redactWorkflowRuns(ctx, tx, emails); err != nil {
		return err
	}
	return redactCommitmentsNaming(ctx, tx, subject)
}

// erasedCommitment is what a commitment naming an erased subject says instead.
// A tombstone rather than an empty label, because the table refuses a blank one
// (weekly_plan_commitment_label_present) and because the rep's week is still
// entitled to say that a commitment was there.
const erasedCommitment = "(erased: the person this named exercised erasure)"

// redactCommitmentsNaming clears what a rep wrote about the subject on their
// weekly plan, in the single erasure transaction.
//
// Reachable because weekly_plan_commitment.linked_record_type accepts 'person'
// alongside deal, lead, organization and project: a rep can commit to an action
// about a contact and type a free-text label, a help request and a manager
// response about them. Nothing else in this cascade can see those rows — the
// table carries no person FK for a schema cascade to walk, its link is
// deliberately unconstrained so a deleted record does not erase the promise,
// and it is keyed to the app_user who wrote it rather than to the subject.
//
// REDACTED, not deleted, and the row is what draws the line. A commitment is
// the REP's work record: they undertook something that week, their lead may
// have answered, and their weekly review has already frozen a count of how many
// they kept. The erasure request belongs to the contact, so it reaches the
// contact's data — the label, the help request, the answer, and the link that
// names them — and stops at the employee's record of having worked. Deleting
// the row would discharge a third party's request by destroying a colleague's
// history, which is over-erasure rather than compliance.
//
// Two of the table's CHECK constraints decide the SHAPE of this statement, and
// neither is optional. weekly_plan_commitment_link_whole says a link is both
// its type and its id or neither, so the pair is cleared together. And
// weekly_plan_commitment_response_whole says an answer is a text, a name and a
// moment or none of the three — so clearing the lead's words alone leaves a
// quotation attributed to nobody, which the constraint refuses and which would
// roll the whole erasure back.
//
// Clearing all three is the same ruling the schema's own departed-lead trigger
// makes (weekly_plan_commitment_forget_manager): what survives is THAT the
// commitment was answered, never a quotation with nobody behind it. The lead's
// user id goes with the text even though a colleague is not the subject,
// because the constraint binds the three together and a name beside a blanked
// answer says only that somebody replied about a person nobody may now name.
//
// One statement over one table, with the name written out. Not a loop over the
// linked-record types and not a helper shared with a sibling eraser: this
// tree's coverage gates read SQL string LITERALS, so a table name arriving
// through a variable is invisible to piicoverage_test.go and
// tableownership_test.go both, and the tidier form turns a proven write into an
// unproven one while the gates stay green.
func redactCommitmentsNaming(ctx context.Context, tx pgx.Tx, personID ids.PersonID) error {
	if _, err := tx.Exec(ctx, `
		UPDATE weekly_plan_commitment
		   SET label = $2,
		       help_requested = '',
		       manager_response = '',
		       manager_user_id = NULL,
		       responded_at = NULL,
		       linked_record_type = NULL,
		       linked_record_id = NULL
		 WHERE linked_record_type = 'person' AND linked_record_id = $1`,
		personID, erasedCommitment); err != nil {
		return fmt.Errorf("privacy: redacting the weekly commitments naming the subject: %w", err)
	}
	return nil
}
