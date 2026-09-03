// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package weekly

// The weekly retrospective as a plain-text message.
//
// IT RENDERS THE SAME Review THE SCREEN DOES, and that is the whole reason
// this file takes a Review rather than a query of its own. A mail assembled
// from a second read would drift from the panel the moment either changed, and
// the two disagreeing about somebody's own week is worse than either being
// absent — the rep has no way to tell which one lied.
//
// Plain text, no link tracking and no images, in the installation's own
// language (platform/mailcopy). It follows the invitation and reset mail
// already in this tree: the operator relay carries product-originated
// transactional mail, and the body says what it has to say without needing a
// browser to render it.

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/mailcopy"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
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
	return words.WeeklySubject + review.LocalWeekStart.Format(mailDateLayout)
}

// mailDateLayout is how a week's start is written, in every language.
//
// ISO, and that is the point: `2 January 2006` puts an English month name in
// the middle of a German sentence, which is the half-translated message this
// catalog exists to stop, and a numeric order like 06/01 is read as 6 January
// by half the world. A reader needs two things from this date — to tell one
// week's message from the next, and to know which week — and 2026-06-01 gives
// both in any language. The deal lines in the same message already read this
// way.
const mailDateLayout = time.DateOnly

// MailBody renders one review as the message a rep reads on Monday.
//
// homeURL is the installation's own Home. Empty omits the closing line rather
// than mailing a link built on an empty origin — an unusable URL in a message
// whose only call to action it is.
func MailBody(review Review, homeURL string, words mailcopy.Copy) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s%s\n\n", words.WeeklyHeading, review.LocalWeekStart.Format(mailDateLayout))

	// The sentence first, when a pass wrote one. It is the only part of the
	// message that reads as a person talking, so it goes above the numbers it
	// is about rather than under them.
	//
	// FLATTENED like every other rendered string, and this is the one that most
	// needs it: a model wrote it, and a model is exactly the source somebody can
	// steer into emitting a newline followed by a line that looks like ours.
	if review.Narrative != "" {
		b.WriteString(mailcopy.OneLine(review.Narrative) + "\n\n")
	}

	c := review.Counts
	// The label column is padded to the WIDEST label in this language rather
	// than to a constant: German and Vietnamese labels are longer than the
	// English ones they were laid out for, and a fixed width turns the column
	// into a ragged edge in two of the three.
	rows := [][2]string{
		{words.WeeklyTasksDelivered, fmt.Sprintf(words.WeeklyOfDue, c.TasksDone, c.TasksDue)},
		{
			words.WeeklyDealsWon + " · " + words.WeeklyDealsLost + " · " + words.WeeklyMoved,
			strconv.Itoa(c.DealsWon) + " · " + strconv.Itoa(c.DealsLost) + " · " + strconv.Itoa(c.DealsMoved),
		},
		{words.WeeklyDecided, strconv.Itoa(c.ProposalsAccepted) + " " + words.WeeklyYes +
			" · " + strconv.Itoa(c.ProposalsRejected) + " " + words.WeeklyNo},
		{words.WeeklyQueue, strconv.Itoa(c.BriefItemsActed) + " " + words.WeeklyActed +
			" · " + strconv.Itoa(c.BriefItemsDismissed) + " " + words.WeeklyDismissed},
		{words.WeeklyCarried, strconv.Itoa(c.TasksCarriedOver)},
	}
	writeRows(&b, rows)

	writeDealLines(&b, review.Deals, words)

	// The message closes forward, not back.
	//
	// Everything above it reports a week that is over. Without a question the
	// mail is a receipt — a rep reads their numbers, agrees with them and does
	// nothing, while the panel the link opens is asking them to plan the next
	// week. Asked in the PANEL's own words (mailcopy pairs it to plan.title),
	// so the message invites exactly what the page it opens offers.
	//
	// It sits above the archive link because it is what the reader is being
	// asked to do; the archive is where they go if they want the week before.
	b.WriteString("\n" + words.WeeklyPlanAhead + "\n")
	// The WEEKLY view, not the bare origin: this message is about a week, and
	// the page it opened without the fragment was showing this morning.
	mailcopy.Link(&b, homeURL, mailcopy.BriefWeeklyFragment, words.WeeklyFullWeek)
	return b.String()
}

// writeRows lays the tallies out as a label column and a value column, sized to
// the labels it was actually given.
func writeRows(b *strings.Builder, rows [][2]string) {
	widest := 0
	for _, row := range rows {
		// RUNES, not bytes: `Übernommen` is ten characters and eleven bytes,
		// and padding by length would short-change every row with an umlaut or
		// a Vietnamese diacritic in it.
		if n := utf8.RuneCountInString(row[0]); n > widest {
			widest = n
		}
	}
	for _, row := range rows {
		pad := strings.Repeat(" ", widest-utf8.RuneCountInString(row[0]))
		b.WriteString(row[0] + ":" + pad + "  " + row[1] + "\n")
	}
}

// writeDealLines writes the week's deals, capped, saying so when it caps.
func writeDealLines(b *strings.Builder, deals []DealLine, words mailcopy.Copy) {
	if len(deals) == 0 {
		return
	}
	b.WriteString("\n" + words.WeeklyWhatMoved + "\n")
	shown := deals
	if len(shown) > mailDealCap {
		shown = shown[:mailDealCap]
	}
	for _, line := range shown {
		b.WriteString("  · " + mailDealLine(line, words) + "\n")
	}
	if rest := len(deals) - len(shown); rest > 0 {
		fmt.Fprintf(b, "  "+words.WeeklyAndMore+"\n", rest)
	}
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
		parts = append(parts, values.MajorUnits(*line.AmountMinor, line.Currency)+" "+line.Currency)
	}
	parts = append(parts, line.OccurredAt.Format(time.DateOnly))
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

// ClaimMailAttempt takes the one attempt this week's mail is allowed, and
// hands back what to send. Claimed reports whether this caller won it.
//
// THE CLAIM IS WRITTEN BEFORE THE RELAY IS DIALLED, and that ordering is the
// design rather than an optimisation.
//
// The transport is a synchronous SMTP call with no retry identity and no
// receipt (platform/mailer). There is nothing to reconcile against afterwards,
// so the only thing that can bound duplicates is a claim taken first: the
// UPDATE below is conditional on mail_attempted_at being NULL, exactly one
// transaction can win it, and every retry after it reads zero rows and does
// nothing.
//
// WHAT THIS COSTS, so the next reader does not "fix" it: any failure after the
// claim loses the mail. A crash before the relay is contacted, a refused
// envelope, a connection dropped mid-body — all of them leave a claimed row
// and no message. That is deliberate. A weekly retrospective delivered twice
// is a person told their own week twice on the one morning the mail exists to
// make calm, and this installation cannot tell a failed send from a delivered
// one, so it cannot retry without risking exactly that.
//
// Do NOT turn this into a retry loop. If confirmed delivery is ever wanted,
// the answer is a delivery ledger with a real receipt from the transport, not
// a second attempt over a transport that reports nothing.
//
// The failure is RECORDED rather than only logged: MailFailed writes the cause
// beside the stamp, so a missing weekly is answerable from the row.
//
// Held by: TestTheWeeklyMailIsAttemptedOnce
// (backend/internal/compose/weeklymail_integration_test.go)
func (e *Engine) ClaimMailAttempt(ctx context.Context, reviewID ids.UUID, now time.Time) (MailAttempt, bool, error) {
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return MailAttempt{}, false, err
	}
	userID, err := reviewUser(ctx)
	if err != nil {
		return MailAttempt{}, false, err
	}
	var attempt MailAttempt
	var claimed bool
	err = database.WithWorkspaceTx(ctx, e.pool, func(tx pgx.Tx) error {
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
	if n := len([]rune(cause)); n > maxMailErrorRunes {
		cause = string([]rune(cause)[:maxMailErrorRunes])
	}
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
