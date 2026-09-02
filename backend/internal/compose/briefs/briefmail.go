// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package briefs

// The morning brief's one attempt at reaching a rep by mail.
//
// The SAME shape the weekly retrospective takes (compose/weekly/weeklymail.go),
// deliberately: claim the attempt before dialling the relay, never release it,
// record why it failed. There is no delivery ledger and no receipt behind
// either, so a claim that could be cleared is a retry loop, and a retry loop on
// a synchronous SMTP call is how one morning becomes three messages.
//
// What differs is the stakes. A brief run is per rep per LOCAL DAY, so this
// lane runs five times a week where the weekly runs once, and a duplicate here
// is not one awkward Monday — it is a person learning the product mails them
// twice every morning.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// maxMailErrorRunes bounds the stored cause, matching the column's CHECK.
//
// A driver's error text is unbounded, and a row the product cannot render helps
// nobody — the point of storing the cause is that somebody asking "where is my
// brief" gets an answer from the row.
const maxMailErrorRunes = 500

// MailAttempt is one claimed send: the run to render and who to send it to.
type MailAttempt struct {
	Run BriefRun
	// Email is empty when the seat is no longer live. The claim is spent
	// either way, and the row then records that no address was found — the
	// honest account of a mail not sent, rather than one sent to a leaver.
	Email string
}

// ClaimMailAttempt takes this run's ONE attempt, or reports that it is spent.
//
// The claim is a conditional UPDATE exactly one transaction can win. Everything
// the caller does after it is allowed to fail and lose the message; nothing
// after it is allowed to produce a second one.
//
// The row scope and the claim are ONE statement. A brief is strictly personal,
// so the user predicate is what stops a run id alone from burning somebody
// else's attempt.
func (e *BriefEngine) ClaimMailAttempt(
	ctx context.Context, runID ids.UUID, now time.Time,
) (MailAttempt, bool, error) {
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return MailAttempt{}, false, err
	}
	userID, err := briefUser(ctx)
	if err != nil {
		return MailAttempt{}, false, err
	}
	var attempt MailAttempt
	var claimed bool
	err = database.WithWorkspaceTx(ctx, e.pool, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			UPDATE brief_run
			   SET mail_attempted_at = $3
			 WHERE id = $1 AND user_id = $2 AND mail_attempted_at IS NULL
			 RETURNING id`, runID, userID, now.UTC())
		var got ids.UUID
		switch err := row.Scan(&got); {
		case errors.Is(err, pgx.ErrNoRows):
			// Either the run is not this rep's, or the attempt is already
			// spent. Both mean the same thing to a caller: do not send.
			return nil
		case err != nil:
			return err
		}
		if _, err := storekit.Audit(ctx, tx, "update", "brief_run", runID,
			map[string]any{"mail_attempted_at": nil},
			map[string]any{"mail_attempted_at": now.UTC()}); err != nil {
			return err
		}
		// By ID and by USER both: the claim above already proved the run is
		// this rep's, and the read repeats the predicate rather than trusting
		// that proof across two statements.
		attempt.Run, err = scanRun(tx.QueryRow(ctx, runSelect+`
			WHERE id = $1 AND user_id = $2`, runID, userID))
		if err != nil {
			return err
		}
		attempt.Run.Items, err = readRunItems(ctx, tx, runID)
		if err != nil {
			return err
		}
		// LIVE MEMBERSHIP, both halves. Deactivating a seat leaves archived_at
		// NULL, so archived_at alone would go on mailing a departed colleague
		// their morning every day.
		switch err := tx.QueryRow(ctx,
			`SELECT email FROM app_user WHERE id = $1 AND `+identity.LiveMemberSQL(""),
			userID).Scan(&attempt.Email); {
		case errors.Is(err, pgx.ErrNoRows):
			attempt.Email = ""
		case err != nil:
			return fmt.Errorf("brief: reading the recipient: %w", err)
		}
		claimed = true
		return nil
	})
	if err != nil {
		return MailAttempt{}, false, err
	}
	return attempt, claimed, nil
}

// MailFailed records why the claimed attempt produced no message.
//
// It does NOT release the claim, and that is the point: the attempt is spent
// either way, and a caller that could clear the stamp would have rebuilt the
// retry loop this design refuses.
func (e *BriefEngine) MailFailed(ctx context.Context, runID ids.UUID, cause string) error {
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return err
	}
	userID, err := briefUser(ctx)
	if err != nil {
		return err
	}
	if n := len([]rune(cause)); n > maxMailErrorRunes {
		cause = string([]rune(cause)[:maxMailErrorRunes])
	}
	return database.WithWorkspaceTx(ctx, e.pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE brief_run SET mail_error = $3
			 WHERE id = $1 AND user_id = $2 AND mail_attempted_at IS NOT NULL`,
			runID, userID, cause)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperrors.ErrNotFound
		}
		_, err = storekit.Audit(ctx, tx, "update", "brief_run", runID,
			map[string]any{"mail_error": nil}, map[string]any{"mail_error": cause})
		return err
	})
}
