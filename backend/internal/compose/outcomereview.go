// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Writing a review of how a deal's closing went.
//
// Two modules own the halves. Deals owns which closing the deal is on and who
// may read that deal; activities owns the note, the frozen questions and the
// answers. Neither may import the other (ADR-0054 §3), so the join happens
// here, in ONE transaction the deal check and the review write share.
//
// That shared transaction is the whole reason this file exists rather than two
// HTTP calls. A review written against a closing that was checked in a separate
// transaction could be filed against an outcome the deal had already left by
// the time the write landed, and nothing downstream would ever say so.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// OutcomeReviews joins the deal's closing to the review written about it.
type OutcomeReviews struct {
	pool       *pgxpool.Pool
	activities *activities.Store
	deals      *deals.Store
}

// NewOutcomeReviews wires the seam over the shared pool.
func NewOutcomeReviews(pool *pgxpool.Pool) *OutcomeReviews {
	return &OutcomeReviews{
		pool:       pool,
		activities: activities.NewStore(InstallationDB(pool)),
		deals:      deals.NewStore(InstallationDB(pool), DealsInstallation()),
	}
}

// Write files one review against the deal's CURRENT closing.
//
// The occurrence the caller names is checked against the one the deal is on
// now, inside the transaction that then writes the review. A deal reopened and
// re-closed while somebody had the form open answers 409 rather than filing
// their account of one outcome against a different one.
func (o *OutcomeReviews) Write(ctx context.Context, dealID ids.DealID, req crmcontracts.CreateOutcomeReviewRequest) (crmcontracts.OutcomeReview, error) {
	// Both ids are REQUIRED and neither is a pointer, so an omitted key decodes
	// to the zero UUID with no error at all. Unguarded, the zero occurrence
	// would be compared against the deal's real one and answer "this deal has
	// closed again since the review was started" — a confident, wrong
	// explanation for a field the caller simply left out.
	if err := httperr.RequireBodyID("closing_occurrence_id", ids.UUID(req.ClosingOccurrenceId)); err != nil {
		return crmcontracts.OutcomeReview{}, err
	}
	if err := httperr.RequireBodyID("submission_id", ids.UUID(req.SubmissionId)); err != nil {
		return crmcontracts.OutcomeReview{}, err
	}

	var out crmcontracts.OutcomeReview
	err := database.WithWorkspaceTx(ctx, o.pool, func(tx pgx.Tx) error {
		occurrence, err := o.deals.HoldClosingOccurrenceTx(ctx, tx, dealID)
		if err != nil {
			return err
		}
		if occurrence.ID != ids.UUID(req.ClosingOccurrenceId) {
			return &deals.StaleClosingError{Current: occurrence.ID}
		}
		out, err = o.activities.WriteOutcomeReviewTx(ctx, tx, activities.WriteReviewInput{
			DealID:              dealID,
			ClosingOccurrenceID: occurrence.ID,
			// From the CLOSING, never from the request. A caller naming their
			// own outcome could file a win review against a loss.
			Outcome:      occurrence.Outcome,
			SubmissionID: ids.UUID(req.SubmissionId),
			Answers:      answersOf(req.Answers),
			Body:         req.Body,
			Source:       "ui",
		})
		return err
	})
	if err != nil {
		return crmcontracts.OutcomeReview{}, err
	}
	return out, nil
}

// List answers every review written about one deal, including reviews of
// earlier closings.
func (o *OutcomeReviews) List(ctx context.Context, dealID ids.DealID) ([]crmcontracts.OutcomeReview, error) {
	var out []crmcontracts.OutcomeReview
	err := database.WithWorkspaceTx(ctx, o.pool, func(tx pgx.Tx) error {
		// Read whatever the deal carries, whether or not it is closed RIGHT
		// NOW. A deal that was closed, reviewed and then reopened still has
		// that review, and it is still the record of what somebody thought
		// when the deal closed — hiding it because the deal happens to be open
		// again would make a reopen look like an erasure.
		//
		// The reader carries the deal's row scope and the note's audience
		// itself, so there is nothing to check first here.
		var err error
		out, err = activities.ReadDealOutcomeReviews(ctx, tx, dealID)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("read the deal's outcome reviews: %w", err)
	}
	return out, nil
}

// answersOf narrows the wire's free-form object to the string answers the
// review stores. A non-string value is dropped rather than coerced: the
// contract says string, and a number silently stringified would read back as
// something the author never typed.
func answersOf(answers map[string]string) map[string]string {
	if answers == nil {
		return map[string]string{}
	}
	return answers
}
