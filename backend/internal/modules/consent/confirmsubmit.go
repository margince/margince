// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// What a data subject sends back through their confirm link, landed in ONE
// transaction with the spending of the link itself.
//
// One transaction because the three things it writes only make sense together.
// A consent grant recorded without spending the token would be replayable, which
// is the property that makes the grant defensible in the first place. A spent
// token without the answer it carried loses the answer and gives the person no
// second chance. And corrections accepted while the consent write failed would
// leave the page having half-worked, with nothing to tell the subject which
// half.

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// confirmSubmissionKind separates the two things the page can send back.
const (
	submissionCorrection = "correction"
	submissionErasure    = "erasure_request"
)

// correctionsField is the wire path a refusal about a proposed value points at.
// A field slot holds a wire path rather than prose, so the client can highlight
// the input the reader has to fix.
const correctionsField = "corrections"

// maxProposedValueRunes bounds one corrected field. A name or an address is
// short; anything longer is a paste accident or an attempt to use the record as
// storage, and both are refused rather than stored.
const maxProposedValueRunes = 500

// ConfirmSubmission is what the subject sent: their corrections, whether they
// asked to be removed, and their answer to the marketing question.
type ConfirmSubmission struct {
	// Corrections are proposals, keyed by the field the page showed. Empty is
	// the ordinary case — most people confirm without changing anything.
	Corrections map[string]string
	// RequestErasure records that they asked to be removed. It files a request
	// for a human rather than erasing: this caller holds a bearer token and no
	// principal, so a leaked link must not be able to destroy a record.
	RequestErasure bool
	// MarketingChoice is "granted", "withdrawn", or empty for no answer. Empty
	// is a real answer to record nothing — a page view grants nothing, and a
	// person who corrects their address without answering has not consented.
	MarketingChoice string
	// MarketingWording is the exact sentence shown beside the choice, stored
	// verbatim on the proof row (Art. 7(1) demonstrability).
	MarketingWording string
}

// SubmitConfirmation spends the link and records everything it carried.
//
// The token is spent FIRST, inside the transaction, and every later step is
// authorized by having spent it. That ordering is the security property: a
// replayed submit finds the token already consumed and refuses before writing
// anything, and the MailboxProof the consent write relies on cannot be
// fabricated by a caller because it is only reachable here.
func (s *Store) SubmitConfirmation(ctx context.Context, token string, in ConfirmSubmission) error {
	if err := validateConfirmSubmission(in); err != nil {
		return err
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		// The SUBJECT first, before the token row, and the order is the whole
		// point. Art. 17 erasure takes the person and then deletes this
		// subject's confirm_token rows; a transaction taking the token first
		// and the person second closes a cycle, and nothing in this tree
		// retries a deadlock — when the eraser is the one that loses, an
		// erasure fulfilment fails. Same ordering IssueDoubleOptIn takes, for
		// the same reason.
		//
		// Naming the subject costs one extra read and buys the ordering: it
		// takes no row lock, so it can say who this link belongs to without
		// ordering anything.
		personID, err := s.subjectOfConfirmTokenTx(ctx, tx, token)
		if err != nil {
			return err
		}
		if err := auth.LockSubjectLive(ctx, tx, "person", personID.UUID); err != nil {
			return err
		}
		// Spent under the subject lock, which is what makes the link
		// single-use: a concurrent submit blocks on the person row first, then
		// finds the token consumed.
		ref, err := s.spendConfirmTokenTx(ctx, tx, token)
		if err != nil {
			return err
		}
		for field, value := range in.Corrections {
			if err := stageSubmission(ctx, tx, ref, submissionCorrection, &field, &value); err != nil {
				return err
			}
		}
		if in.RequestErasure {
			if err := stageSubmission(ctx, tx, ref, submissionErasure, nil, nil); err != nil {
				return err
			}
		}
		if in.MarketingChoice == "" {
			return nil
		}
		return s.recordMarketingAnswerTx(ctx, tx, ref, in)
	})
}

// recordMarketingAnswerTx writes the subject's marketing answer through the
// ordinary consent engine — same proof row, same audit, same consent.changed
// event as any other decision — with the mailbox proof the spent token earned.
func (s *Store) recordMarketingAnswerTx(ctx context.Context, tx pgx.Tx, ref ConfirmRef, in ConfirmSubmission) error {
	purposeID, err := purposeByKeyTx(ctx, tx, PurposeMarketingEmail)
	if err != nil {
		return err
	}
	source := "confirm_details"
	input := RecordInput{
		PersonID:   ref.PersonID,
		PurposeID:  purposeID,
		NewState:   in.MarketingChoice,
		Source:     &source,
		PolicyText: &in.MarketingWording,
		// Earned, not asserted: this is set only after spendConfirmTokenTx
		// consumed the link that proves the mailbox.
		MailboxProof: MailboxProvenByConfirmLink,
		// Earned, not asserted: this is set only after spendConfirmTokenTx
		// consumed the link that proves the mailbox.
	}
	sub, state, err := admitRecord(ctx, input)
	if err != nil {
		return err
	}
	_, err = s.recordAdmittedTx(ctx, tx, input, sub, state)
	return err
}

// stageSubmission files one proposal for a human to accept. Nothing here
// touches the person record: the subject holds a bearer token and sits outside
// every row-scope probe, so what they send is evidence of what they asked for
// and never a write to the CRM.
func stageSubmission(ctx context.Context, tx pgx.Tx, ref ConfirmRef, kind string, field, value *string) error {
	var submissionID ids.UUID
	if err := tx.QueryRow(ctx, `
		INSERT INTO person_confirm_submission (person_id, token_id, kind, field, proposed_value)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id`,
		ref.PersonID, ref.TokenID, kind, field, value).Scan(&submissionID); err != nil {
		return err
	}
	// Audited against the PERSON, because that is the record a later reader
	// asks about — "what did this contact send us, and when". The proposed
	// value is deliberately absent from the audit payload: it is the subject's
	// own data, it already sits on the submission row, and a second copy in the
	// audit trail would outlive the erasure that clears the first.
	// AuditEvent, not Audit: this records something that HAPPENED rather than a
	// field that moved. Nothing about the person changed — the field a
	// correction proposes still holds its old value, deliberately — so there is
	// no prior state for a before-image to name.
	_, err := storekit.AuditEvent(ctx, tx, "update", "person", ref.PersonID.UUID, map[string]any{
		"confirm_submission": kind,
		"submission_id":      submissionID,
	})
	return err
}

// validateConfirmSubmission refuses what the store cannot stand behind, before
// the token is spent — so a malformed submit leaves the link usable rather than
// burning somebody's one chance to answer.
func validateConfirmSubmission(in ConfirmSubmission) error {
	switch in.MarketingChoice {
	case "", string(StateGranted), string(StateWithdrawn):
	default:
		return &ValidationError{
			Field:  "marketing_choice",
			Reason: "a marketing answer is granted, withdrawn, or absent",
		}
	}
	if in.MarketingChoice == string(StateGranted) && strings.TrimSpace(in.MarketingWording) == "" {
		return &ValidationError{
			// Art. 7(1) asks the controller to demonstrate what the subject
			// agreed TO, so a grant with no wording is a grant nobody can
			// stand behind.
			Field:  "marketing_wording",
			Reason: "a grant records the exact wording shown to the subject",
		}
	}
	for field, value := range in.Corrections {
		if strings.TrimSpace(field) == "" {
			return &ValidationError{Field: correctionsField, Reason: "a correction names the field it corrects"}
		}
		if !confirmCorrectableFields[field] {
			return &ValidationError{
				Field:  correctionsField,
				Reason: "that field is not one the confirm page offers",
			}
		}
		if len([]rune(value)) > maxProposedValueRunes {
			return &ValidationError{
				Field:  correctionsField,
				Reason: "a corrected value is at most 500 characters",
			}
		}
	}
	return nil
}
