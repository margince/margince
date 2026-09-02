// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package weekly

// The week's sentence: what a model wrote about the counts beside it.
//
// Its own file because it is the one part of a review a model touches, and
// the one a reader must be able to tell from the facts: everything else here
// is measured, this is written.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/weekly/narrative"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Narrate writes the week's sentence onto an existing review.
//
// A SECOND WRITE onto a row the deterministic pass already committed, never
// part of assembling it. The counts and the deal lines are the review; this is
// what a colleague would say about them, and it must be able to fail without
// costing the rep any of it.
//
// Idempotent by replacement: a later pass over the same week is a correction,
// not an addition. The stamp moves with it.
//
// Empty prose stores as NULL with the stamp still written — the CHECK admits
// that deliberately, so a pass that ran and found the week unremarkable is
// distinguishable from one that never ran.
func (e *Engine) Narrate(ctx context.Context, reviewID ids.UUID, sentence string, now time.Time) error {
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return err
	}
	userID, err := reviewUser(ctx)
	if err != nil {
		return err
	}
	// Bounded HERE as well as in the parser, because this is the writer: a
	// caller that reaches Narrate without going through narrative.Parse would
	// otherwise learn the ceiling from a driver error at 06:00 on a Monday.
	// Runes, because the column counts characters — a German sentence full of
	// umlauts is fewer characters than bytes.
	if n := len([]rune(sentence)); n > narrative.MaxNarrativeRunes {
		return httperr.Validation("narrative", "too_long",
			fmt.Sprintf("the sentence is %d characters, over the %d the column holds",
				n, narrative.MaxNarrativeRunes))
	}
	return database.WithWorkspaceTx(ctx, e.pool, func(tx pgx.Tx) error {
		var before *string
		var beforeStamp *time.Time
		// The owner check is the row scope: a review belongs to the rep whose
		// week it was, and the id alone must not reach anybody else's.
		row := tx.QueryRow(ctx, `
			SELECT narrative, narrated_at FROM weekly_review
			 WHERE id = $1 AND user_id = $2
			 FOR UPDATE`, reviewID, userID)
		switch err := row.Scan(&before, &beforeStamp); {
		case errors.Is(err, pgx.ErrNoRows):
			return apperrors.ErrNotFound
		case err != nil:
			return err
		}

		stamp := now.UTC()
		var stored *string
		if sentence != "" {
			stored = &sentence
		}
		if _, err := tx.Exec(ctx,
			`UPDATE weekly_review SET narrative = $2, narrated_at = $3 WHERE id = $1`,
			reviewID, stored, stamp); err != nil {
			return err
		}
		_, err := storekit.Audit(ctx, tx, "update", "weekly_review", reviewID,
			map[string]any{"narrative": before, "narrated_at": beforeStamp},
			map[string]any{"narrative": stored, "narrated_at": stamp})
		return err
	})
}

// insertColumns is a statement's column list, which knows its own placeholders.
//
// The pairing is the point: a caller adds a column and its argument in one
// call, and the $N run is derived from how many there are rather than typed.
// The defect this prevents is a column added to the list and not to the values,
// which Postgres reports as a type error several columns away from the cause.
