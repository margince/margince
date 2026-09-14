// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// Writing one outcome review.
//
// The note and the structured answers are ONE write. A review that saved its
// prose and lost its answers, or the reverse, would be a record nobody could
// act on: the timeline would show a review with nothing in it, or the
// completion count would claim a review nobody can read.
//
// So this runs inside a transaction the caller already holds — compose opens it
// so the deal's own checks and this write commit together — and it reuses the
// ordinary activity writer rather than inserting a note by hand. That writer
// carries the author, the audience rules, the retention treatment and the
// timeline projection; a hand-built insert would have none of them and nothing
// would say so until somebody's erasure request missed the review.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// WriteReviewInput is one submission, as the caller that already holds the deal
// hands it over. The deal's identity and its current closing are settled by
// that caller: this module owns the review, not the deal.
type WriteReviewInput struct {
	DealID              ids.DealID
	ClosingOccurrenceID ids.UUID
	// Outcome is what the CLOSING recorded, read from history rather than from
	// the deal's stage label today.
	Outcome string
	// SubmissionID is the client's own id for this submission, unique per
	// author. A retry with the same id finds the review already written.
	SubmissionID ids.UUID
	Answers      map[string]string
	// Body is optional prose for the note itself. The answers are the review;
	// this is what somebody wanted to say beside them.
	Body   *string
	Source string
}

// WriteOutcomeReviewTx writes the note and the frozen review together.
//
// It returns the review as stored. A second call with the same submission id
// from the same author returns that first review unchanged rather than writing
// another: a retried request is one review, and the client cannot tell whether
// its first attempt reached us.
func (s *Store) WriteOutcomeReviewTx(ctx context.Context, tx pgx.Tx, in WriteReviewInput) (crmcontracts.OutcomeReview, error) {
	if err := ensureReviewAuthority(ctx); err != nil {
		return crmcontracts.OutcomeReview{}, err
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return crmcontracts.OutcomeReview{}, err
	}

	// A retry is answered before anything is written, so the second attempt
	// does not create a second note and then fail on the unique index.
	if existing, found, err := reviewBySubmission(ctx, tx, by, in.SubmissionID); err != nil {
		return crmcontracts.OutcomeReview{}, err
	} else if found {
		return existing, nil
	}

	template, live, err := ReviewTemplateFor(ctx, tx, in.Outcome)
	if err != nil {
		return crmcontracts.OutcomeReview{}, err
	}
	if !live {
		return crmcontracts.OutcomeReview{}, errReviewTemplateRetired
	}
	if err := validateAnswers(template.Questions, in.Answers); err != nil {
		return crmcontracts.OutcomeReview{}, err
	}

	// The note first: it is what the review IS, and the frozen row points at
	// it. Written through the ordinary activity writer so it carries the
	// author, the audience and the retention treatment every other note has.
	note, _, err := s.LogActivityTx(ctx, tx, LogActivityInput{
		// A note, not a kind of its own. The review IS a note somebody wrote;
		// a new activity kind would need a place in every timeline filter and
		// every count, to say something the frozen row beside it already says.
		Kind:    "note",
		Subject: reviewSubject(template.Label),
		Body:    in.Body,
		Links:   []ActivityLinkInput{{EntityType: "deal", EntityID: in.DealID.UUID}},
		Source:  in.Source,
	})
	if err != nil {
		return crmcontracts.OutcomeReview{}, fmt.Errorf("write the review's note: %w", err)
	}

	questions, err := json.Marshal(template.Questions)
	if err != nil {
		return crmcontracts.OutcomeReview{}, fmt.Errorf("freeze the review's questions: %w", err)
	}
	answers, err := json.Marshal(in.Answers)
	if err != nil {
		return crmcontracts.OutcomeReview{}, fmt.Errorf("encode the review's answers: %w", err)
	}

	var id ids.UUID
	if err := tx.QueryRow(ctx,
		`INSERT INTO activity_review_response
		     (activity_id, deal_id, closing_occurrence_id, outcome,
		      template_key, template_version, questions, answers, submission_id, source, captured_by)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		 RETURNING id`,
		ids.UUID(note.Id), in.DealID, in.ClosingOccurrenceID, in.Outcome,
		template.Key, template.Version, questions, answers, in.SubmissionID, in.Source, by).
		Scan(&id); err != nil {
		if storekit.IsForeignKeyViolation(err) {
			// The composite key refused the pair: the occurrence is not a
			// closing of THIS deal. A caller reaching this has named somebody
			// else's closing, which is a 404 about the occurrence rather than
			// a fault in the review.
			return crmcontracts.OutcomeReview{}, apperrors.ErrNotFound
		}
		if storekit.IsUniqueViolation(err) {
			// Two submissions racing for one activity or one submission id.
			// The retry above answers the ordinary case; this is the narrow
			// window where both arrived at once.
			return crmcontracts.OutcomeReview{}, apperrors.ErrConflict
		}
		return crmcontracts.OutcomeReview{}, fmt.Errorf("store the review: %w", err)
	}

	// No audit row of its own, and no event.
	//
	// The note IS the review, and LogActivityTx above already wrote both: an
	// audit row naming the activity and the activity.created event that ships
	// with it. A second audit row here would record one write twice — the
	// timeline would show the review created once and the audit trail twice —
	// and a second event would announce to subscribers that two things
	// happened when one did.
	//
	// What the frozen row adds is not a separate write to announce. It is the
	// structure of a note that already exists, and a reader who wants to know
	// who wrote this review and when asks the activity, which is where the
	// product keeps that for every other note.
	return readOutcomeReview(ctx, tx, id)
}

// reviewSubject titles the note so a timeline reader can tell what it is
// without opening it.
func reviewSubject(label string) *string {
	subject := label
	return &subject
}

// reviewBySubmission answers the review this author already wrote under this
// submission id, if any.
func reviewBySubmission(ctx context.Context, tx pgx.Tx, by string, submission ids.UUID) (crmcontracts.OutcomeReview, bool, error) {
	var id ids.UUID
	err := tx.QueryRow(ctx,
		`SELECT id FROM activity_review_response WHERE captured_by = $1 AND submission_id = $2`,
		by, submission).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return crmcontracts.OutcomeReview{}, false, nil
	}
	if err != nil {
		return crmcontracts.OutcomeReview{}, false, fmt.Errorf("look for an earlier submission: %w", err)
	}
	out, err := readOutcomeReview(ctx, tx, id)
	return out, err == nil, err
}

// ReadDealOutcomeReviews answers every review written about one deal, newest
// first — including reviews of EARLIER closings.
//
// An old review stays readable rather than being hidden or reattached. A deal
// closed, reopened and closed again has more than one thing to have an opinion
// about, and each review names the closing it is about so a caller can tell
// which one is current.
func ReadDealOutcomeReviews(ctx context.Context, tx pgx.Tx, dealID ids.DealID) ([]crmcontracts.OutcomeReview, error) {
	// The deal's own row scope, checked HERE rather than left to the caller.
	//
	// A review says what happened to one deal, so reading the reviews is
	// reading something about that deal. compose checks the same thing before
	// calling, and this is deliberately the second check: a reader that trusts
	// its caller to have bounded it is a reader that becomes unbounded the
	// first time somebody calls it from somewhere else, and nothing would fail
	// at that moment to say so.
	if err := auth.EnsureVisible(ctx, tx, "deal", dealID.UUID); err != nil {
		return nil, err
	}

	// And the ACTIVITY's audience on top of it, which is the gate that decides
	// who reads what was SAID rather than who may know the deal exists.
	//
	// Without this, an author who limits their review's note to a few
	// colleagues has limited only the note: the structured answers are the
	// same words, and anybody who can read the deal would still be handed them
	// through this endpoint. The frozen row is content, so it takes the
	// content gate.
	args := []any{dealID}
	arg := func(v any) int { args = append(args, v); return len(args) }
	scope, err := auth.ActivityContentClause(ctx, "a", arg)
	if err != nil {
		return nil, err
	}
	where := "r.deal_id = $1"
	if scope != "" {
		where += " AND " + scope
	}
	rows, err := tx.Query(ctx,
		reviewSelect+` r JOIN activity a ON a.id = r.activity_id
		 WHERE `+where+` ORDER BY r.created_at DESC, r.id DESC`, args...)
	if err != nil {
		return nil, fmt.Errorf("read the deal's reviews: %w", err)
	}
	defer rows.Close()
	out := []crmcontracts.OutcomeReview{}
	for rows.Next() {
		review, err := scanOutcomeReview(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, review)
	}
	return out, rows.Err()
}

const reviewSelect = `SELECT r.id, r.activity_id, r.deal_id, r.closing_occurrence_id, r.outcome,
	r.template_key, r.template_version, r.questions, r.answers, r.revision, r.created_at, r.updated_at
	FROM activity_review_response`

// readOutcomeReview answers one review, bounded by the deal it is about.
//
// The row hands back a deal_id, so reading it is learning that a particular
// deal exists and was closed. Both callers reach it holding a deal they have
// already checked, and this checks again anyway: a read bounded only by its
// callers is unbounded the moment a third one appears, and nothing fails at
// that moment to say so.
func readOutcomeReview(ctx context.Context, tx pgx.Tx, id ids.UUID) (crmcontracts.OutcomeReview, error) {
	out, err := scanOutcomeReview(tx.QueryRow(ctx, reviewSelect+` r WHERE r.id = $1`, id))
	if err != nil {
		return crmcontracts.OutcomeReview{}, err
	}
	if err := auth.EnsureVisible(ctx, tx, "deal", ids.UUID(out.DealId)); err != nil {
		return crmcontracts.OutcomeReview{}, err
	}
	// The note's audience, for the same reason the list applies it: the
	// answers are the content of a note somebody may have limited.
	if err := auth.EnsureActivityVisible(ctx, tx, ids.UUID(out.ActivityId)); err != nil {
		return crmcontracts.OutcomeReview{}, err
	}
	return out, nil
}

func scanOutcomeReview(row pgx.Row) (crmcontracts.OutcomeReview, error) {
	var out crmcontracts.OutcomeReview
	var id, activityID, dealID, occurrenceID ids.UUID
	var outcome string
	var questions, answers []byte
	if err := row.Scan(&id, &activityID, &dealID, &occurrenceID, &outcome,
		&out.TemplateKey, &out.TemplateVersion, &questions, &answers,
		&out.Revision, &out.CreatedAt, &out.UpdatedAt); err != nil {
		return crmcontracts.OutcomeReview{}, err
	}
	var decoded []ReviewQuestion
	if err := json.Unmarshal(questions, &decoded); err != nil {
		return crmcontracts.OutcomeReview{}, fmt.Errorf("the review's frozen questions are not readable: %w", err)
	}
	if err := json.Unmarshal(answers, &out.Answers); err != nil {
		return crmcontracts.OutcomeReview{}, fmt.Errorf("the review's answers are not readable: %w", err)
	}
	out.Id = openapi_types.UUID(id)
	out.ActivityId = openapi_types.UUID(activityID)
	out.DealId = openapi_types.UUID(dealID)
	out.ClosingOccurrenceId = openapi_types.UUID(occurrenceID)
	out.Outcome = crmcontracts.OutcomeReviewOutcome(outcome)
	out.Questions = wireQuestions(decoded)
	return out, nil
}
