// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package weekly

// Freezing what a week taught, once.
//
// WRITE-ONCE, and the state column is what makes that true rather than hoped
// for. A second pass finds a stamped review and returns without touching it, so
// a dispatcher that ticks more than once inside a week — which it does, so a
// worker that was down still backfills — cannot replace a learning a rep has
// already read.
//
// The three states are not two. `not_run` is a week nobody looked at;
// `insufficient_evidence` is a week that was read and had too little to say.
// Collapsing them tells a rep "nothing to learn" about a week no model ever
// saw, which is a claim the product has not earned.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/weekly/learnings"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The three states a review's learnings can be in, mirroring the CHECK on
// weekly_review.learnings_state.
const (
	LearningsNotRun       = "not_run"
	LearningsInsufficient = "insufficient_evidence"
	LearningsSynthesized  = "synthesized"
)

// RecordLearnings stamps what a week taught, or that there was too little.
//
// An EMPTY list is a real answer and stamps insufficient_evidence: the pass ran
// and found nothing it could ground. The caller reaching here below the floor
// passes nothing too, for the same stamp — a week that could not be learned
// from and one that yielded nothing read the same to a rep, and both are
// honestly distinct from a week nobody looked at.
//
// wrote=false means the week already had its learnings. That is the constraint
// doing its job rather than a failure.
func (e *Engine) RecordLearnings(
	ctx context.Context, reviewID ids.UUID, items []learnings.Learning, now time.Time,
) (bool, error) {
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return false, err
	}
	userID, err := reviewUser(ctx)
	if err != nil {
		return false, err
	}
	if len(items) > learnings.MaxLearnings {
		return false, fmt.Errorf("weekly: %d learnings, over the %d one week holds",
			len(items), learnings.MaxLearnings)
	}

	wrote := false
	err = database.WithWorkspaceTx(ctx, e.pool, func(tx pgx.Tx) error {
		// The owner check IS the row scope: a review belongs to the rep whose
		// week it was, and the id alone must not reach anybody else's. FOR
		// UPDATE so two passes racing produce one set of learnings.
		var state string
		row := tx.QueryRow(ctx, `
			SELECT learnings_state FROM weekly_review
			 WHERE id = $1 AND user_id = $2
			 FOR UPDATE`, reviewID, userID)
		switch err := row.Scan(&state); {
		case errors.Is(err, pgx.ErrNoRows):
			return apperrors.ErrNotFound
		case err != nil:
			return err
		}
		if state != LearningsNotRun {
			// Already learned. Leave what the rep read where it is.
			return nil
		}

		stamp := now.UTC()
		next := LearningsInsufficient
		if len(items) > 0 {
			next = LearningsSynthesized
		}
		if _, err := tx.Exec(ctx, `
			UPDATE weekly_review SET learnings_state = $2, learned_at = $3
			 WHERE id = $1`, reviewID, next, stamp); err != nil {
			return err
		}
		for i, item := range items {
			if err := insertLearning(ctx, tx, reviewID, i, item); err != nil {
				return err
			}
		}
		wrote = true
		// ONE audit row for the whole pass rather than one per learning. The
		// subject is the review, and four learnings writing four entries would
		// bury the one that says the week was learned.
		//
		// AuditEvent and not Audit: these rows did not exist a moment ago, so
		// there is no before-image, and the ordinary door refuses an update
		// carrying none — correctly. "create" rather than a verb of its own,
		// because the action vocabulary is a migration-controlled CHECK and
		// extending it for one call site would be a schema change bought for a
		// word.
		_, err := storekit.AuditEvent(ctx, tx, "create", "weekly_review", reviewID,
			map[string]any{"learnings_state": next, "learnings": len(items)})
		return err
	})
	if err != nil {
		return false, err
	}
	return wrote, nil
}

// insertLearning writes one learning and what it rests on.
func insertLearning(
	ctx context.Context, tx pgx.Tx, reviewID ids.UUID, position int, item learnings.Learning,
) error {
	var id ids.UUID
	if err := tx.QueryRow(ctx, `
		INSERT INTO weekly_review_learning (weekly_review_id, kind, text, position)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT ON CONSTRAINT uq_weekly_review_learning_slot DO NOTHING
		RETURNING id`, reviewID, item.Kind, item.Text, position).Scan(&id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// The slot was taken by a pass that got here first. Its citations
			// are already written; adding ours would attach them to a claim
			// nobody made.
			return nil
		}
		return fmt.Errorf("weekly: writing a learning: %w", err)
	}
	for _, c := range item.Citations {
		if _, err := tx.Exec(ctx, `
			INSERT INTO weekly_review_learning_citation
			    (learning_id, subject_type, subject_id, label)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT ON CONSTRAINT uq_weekly_review_learning_citation DO NOTHING`,
			id, c.SubjectType, c.SubjectID, c.Label); err != nil {
			return fmt.Errorf("weekly: writing a citation: %w", err)
		}
	}
	return nil
}

// readLearnings reads one review's frozen learnings, in the order the pass put
// them in.
func readLearnings(ctx context.Context, tx pgx.Tx, reviewID ids.UUID) ([]learnings.Learning, error) {
	rows, err := tx.Query(ctx, `
		SELECT l.id, l.kind, l.text
		  FROM weekly_review_learning l
		 WHERE l.weekly_review_id = $1
		 ORDER BY l.position`, reviewID)
	if err != nil {
		return nil, fmt.Errorf("weekly: reading the week's learnings: %w", err)
	}
	defer rows.Close()

	var out []learnings.Learning
	// Collected first, because the citation reads below open their own queries
	// and this one's rows must be drained before another can run on the same
	// connection.
	learningIDs := make([]ids.UUID, 0, learnings.MaxLearnings)
	for rows.Next() {
		var id ids.UUID
		var item learnings.Learning
		if err := rows.Scan(&id, &item.Kind, &item.Text); err != nil {
			return nil, fmt.Errorf("weekly: reading a learning: %w", err)
		}
		out = append(out, item)
		learningIDs = append(learningIDs, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("weekly: reading the week's learnings: %w", err)
	}
	for i, id := range learningIDs {
		if out[i].Citations, err = readCitations(ctx, tx, id); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// readCitations reads what one learning rests on.
func readCitations(ctx context.Context, tx pgx.Tx, learningID ids.UUID) ([]learnings.Citation, error) {
	rows, err := tx.Query(ctx, `
		SELECT subject_type, subject_id, label
		  FROM weekly_review_learning_citation
		 WHERE learning_id = $1
		 ORDER BY subject_type, subject_id`, learningID)
	if err != nil {
		return nil, fmt.Errorf("weekly: reading a learning's citations: %w", err)
	}
	defer rows.Close()

	var out []learnings.Citation
	for rows.Next() {
		var c learnings.Citation
		if err := rows.Scan(&c.SubjectType, &c.SubjectID, &c.Label); err != nil {
			return nil, fmt.Errorf("weekly: reading a citation: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("weekly: reading a learning's citations: %w", err)
	}
	return out, nil
}
