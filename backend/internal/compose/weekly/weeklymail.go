// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package weekly

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/mailcopy"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/textcut"
)

// mailDealCap bounds the deal lines a message carries.
//
// The mail is a nudge toward the screen, not a replacement for it: a rep who
// closed thirty deals wants the panel, and a mail that lists all thirty is one
// nobody reads to the end. The overflow is COUNTED rather than dropped
// silently — a message that quietly stops at ten reads as a complete week that
// happened to have ten.
const mailDealCap = 10

// MailSubject names the week the message is about.
//
// The week is in the subject on purpose: these arrive weekly into a mailbox
// that already has last week's, and two identical subjects are two messages a
// reader cannot tell apart in a list.
func MailSubject(review Review, words mailcopy.Copy) string {
	return words.WeeklySubject + words.DateRange(review.LocalWeekStart, review.LocalWeekStart.AddDate(0, 0, 6))
}

// MailBody renders one review as the message a rep reads on Monday.
//
// homeURL is the installation's own Home. Empty omits the closing line rather
// than mailing a link built on an empty origin — an unusable URL in a message
// whose only call to action it is.
func MailBody(review Review, homeURL string, words mailcopy.Copy) string {
	return MailMessage(review, homeURL, words).Text
}

// mailDealLine is one deal, as the week recorded it.
//
// The LABEL is what was frozen when the review was written, never a lookup: a
// deal renamed or deleted since still reads here as it did that week, which is
// the same promise the panel makes.
func mailDealLine(line DealLine, words mailcopy.Copy) string {
	parts := []string{mailcopy.OneLine(line.Label), outcomeWord(line.Outcome, words)}
	if line.ToStageLabel != "" {
		// A stage name is stored with no single-line validation, so it is the
		// same species of input as the label beside it.
		parts = append(parts, mailcopy.OneLine(line.ToStageLabel))
	}
	// Money is a pair or it is absent — the same rule the wire follows. A bare
	// amount is a number nobody can read.
	if line.AmountMinor != nil && line.Currency != "" {
		parts = append(parts, words.Money(*line.AmountMinor, line.Currency))
	}
	parts = append(parts, words.Date(line.OccurredAt))
	return strings.Join(parts, " — ")
}

// MailAttempt is what one claim won: the review to render and the address to
// render it to, or nothing at all.
type MailAttempt struct {
	Review Review
	// Email is the rep's own address. A claim is never issued without one — a
	// review whose seat has no address is left unclaimed rather than burned.
	Email string
}

// ClaimMailAttempt spends the existing weekly identity before contacting the relay.
// An uncertain send keeps its claim, preventing a retry from producing another copy.
func (e *Engine) ClaimMailAttempt(ctx context.Context, reviewID ids.UUID, now time.Time) (MailAttempt, bool, error) {
	var attempt MailAttempt
	var claimed bool
	err := database.WithWorkspaceTx(ctx, e.pool, func(tx pgx.Tx) error {
		var err error
		attempt, claimed, err = e.ClaimMailAttemptTx(ctx, tx, reviewID, now)
		return err
	})
	return attempt, claimed, err
}

// ClaimMailAttemptTx lets a combined edition claim its existing rows atomically.
func (e *Engine) ClaimMailAttemptTx(ctx context.Context, tx pgx.Tx, reviewID ids.UUID, now time.Time) (MailAttempt, bool, error) {
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return MailAttempt{}, false, err
	}
	userID, err := reviewUser(ctx)
	if err != nil {
		return MailAttempt{}, false, err
	}

	var attempt MailAttempt
	var claimed bool
	claim := func() error {
		// The row scope and the claim in ONE statement. A review belongs to
		// the rep whose week it was, so the user predicate is what stops an id
		// alone from burning somebody else's attempt.
		row := tx.QueryRow(ctx, `
			UPDATE weekly_review
			   SET mail_attempted_at = $3
			 WHERE id = $1 AND user_id = $2 AND mail_attempted_at IS NULL
			 RETURNING id`, reviewID, userID, now.UTC())
		var got ids.UUID
		switch err := row.Scan(&got); {
		case errors.Is(err, pgx.ErrNoRows):
			// Either the review is not this rep's, or the attempt is already
			// spent. Both mean the same thing to a caller: do not send.
			return nil
		case err != nil:
			return err
		}
		if _, err := storekit.Audit(ctx, tx, "update", "weekly_review", reviewID,
			map[string]any{"mail_attempted_at": nil},
			map[string]any{"mail_attempted_at": now.UTC()}); err != nil {
			return err
		}
		attempt.Review, err = readReviewTx(ctx, tx, reviewID, userID)
		if err != nil {
			return err
		}
		// LIVE MEMBERSHIP, both halves. Deactivating a seat leaves archived_at
		// NULL, so archived_at alone would go on mailing a departed colleague
		// their week every Monday.
		//
		// A seat that is no longer live reads as no address, which leaves the
		// claim spent and the row saying why — the honest record of a mail that
		// was not sent, rather than one sent to somebody who left.
		switch err := tx.QueryRow(ctx,
			`SELECT email FROM app_user WHERE id = $1 AND `+identity.LiveMemberSQL(""),
			userID).Scan(&attempt.Email); {
		case errors.Is(err, pgx.ErrNoRows):
			attempt.Email = ""
		case err != nil:
			return fmt.Errorf("weekly: reading the recipient: %w", err)
		}
		claimed = true
		return nil
	}
	if err := claim(); err != nil {
		return MailAttempt{}, false, err
	}
	return attempt, claimed, nil
}

// MailFailed records why the claimed attempt produced no message.
//
// It does NOT release the claim, and that is the point: the attempt is spent
// either way, and a caller that could clear the stamp would have rebuilt the
// retry loop this design refuses. What it adds is the reason, so somebody
// asking "where is my weekly" gets an answer from the row.
func (e *Engine) MailFailed(ctx context.Context, reviewID ids.UUID, cause string) error {
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return err
	}
	userID, err := reviewUser(ctx)
	if err != nil {
		return err
	}
	cause = textcut.Runes(cause, maxMailErrorRunes)
	return database.WithWorkspaceTx(ctx, e.pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE weekly_review SET mail_error = $3
			 WHERE id = $1 AND user_id = $2 AND mail_attempted_at IS NOT NULL`,
			reviewID, userID, cause)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperrors.ErrNotFound
		}
		_, err = storekit.Audit(ctx, tx, "update", "weekly_review", reviewID,
			map[string]any{"mail_error": nil}, map[string]any{"mail_error": cause})
		return err
	})
}

// maxMailErrorRunes matches the column's CHECK. A driver error's text is
// unbounded, and a row nothing can render helps nobody — so the cause is cut
// here rather than learned from a constraint violation at 06:00 on a Monday.
const maxMailErrorRunes = 500

// outcomeWord is what a deal's outcome is called in the reader's language.
//
// The stored value is the vocabulary the API publishes, so it is translated at
// the edge rather than stored translated: a review written under one base
// language and read after it changed would otherwise be half in each. An
// outcome this build cannot spell is written through as it was stored — a
// reader seeing `won` in a German message has learned something, where a blank
// tells them nothing.
func outcomeWord(outcome string, words mailcopy.Copy) string {
	switch outcome {
	case "won":
		return words.WeeklyOutcomeWon
	case "lost":
		return words.WeeklyOutcomeLost
	case "moved":
		return words.WeeklyOutcomeMoved
	}
	return outcome
}

// MailPending reads the existing personal claim without spending it.
func (e *Engine) MailPending(ctx context.Context, reviewID ids.UUID) (bool, error) {
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return false, err
	}
	user, err := reviewUser(ctx)
	if err != nil {
		return false, err
	}
	var pending bool
	err = database.WithWorkspaceTx(ctx, e.pool, func(tx pgx.Tx) error {
		args := []any{reviewID, user}
		holders := strings.Split(storekit.Placeholders(args), ",")
		return tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM weekly_review WHERE id = "+holders[0]+" AND user_id = "+holders[1]+" AND mail_attempted_at IS NULL)", args...).Scan(&pending)
	})
	return pending, err
}

func mailHighlights(review Review, words mailcopy.Copy) string {
	figures := figuresOf(review, words)
	var lines []string
	if review.Counts.DealsWon > 0 && measuredFigure(figures.set.Deals) && figures.completeBookings() {
		line := words.WeeklyWonOne
		if review.Counts.DealsWon != 1 {
			line = fmt.Sprintf(words.WeeklyWonMany, review.Counts.DealsWon)
		}
		lines = append(lines, line)
	}
	if done := review.Counts.TasksCompleted; done != nil && *done > 0 && measuredFigure(figures.set.Tasks) {
		line := words.WeeklyDoneOne
		if *done != 1 {
			line = fmt.Sprintf(words.WeeklyDoneMany, *done)
		}
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		return words.WeeklyOverview
	}
	return strings.Join(lines, " ")
}
