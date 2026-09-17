// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What the morning digest is ALLOWED to say.
//
// The morning brief's message carries no record value at all — only counts, an
// annotation's own sentence and a link — and its author says why: a brief item
// holds a deal id and a rank, never a name, so listing them would mean either
// printing UUIDs or growing a second name resolver beside the one the API has.
// That decision sidestepped the question this file cannot sidestep. A notice
// carries its own SUBJECT, written when the notice was raised, and a subject
// routinely names the record it is about — "Aurelia Vance replied", "the Orion
// renewal moved to Negotiation". So this lane quotes record values, and inherits
// the whole obligation the brief's author avoided.
//
// THE STORED REFERENCE IS NOT A PERMISSION. A notice's target_type/target_id
// pair records what was true on the day it was written; ownership moves, grants
// lapse, a captured contact is withdrawn again. So every reference is re-scoped
// HERE, at render time, under the recipient's own freshly bound authority — the
// same rule the export's reference withholding follows. A target that still
// passes may be quoted. One that does not is COUNTED and never quoted, which
// leaves the reader an honest "and 3 more" rather than a line about a record
// they may no longer open.
//
// FAIL CLOSED on anything this package cannot answer about. A target type is a
// plain string a producer wrote; a type the row scope does not know is one there
// is no honest way to ask about, so it is counted too. Nothing here invents a
// second visibility rule to cover it — a parallel copy of the visibility
// question is precisely the outcome this file exists to avoid.
//
// A notice with NO target has nothing to re-scope: its subject is the product's
// own sentence to this one reader, so it is quotable.

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/mailcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// digestQuoteCap bounds the lines one message carries.
//
// A morning's queue is not capped at any length a reader chose, and a message
// that lists twenty notices is a page — which is what the link is for. Five is
// what a reader takes in before deciding to open the app, and it is the brief's
// own cap for the same reason.
const digestQuoteCap = 5

// digestLines is what one morning may say: the subjects the recipient may still
// be shown, and how many notices are waiting in total.
//
// TOTAL AND NOT "quotable", deliberately: the tail counts everything still
// waiting, the lines this reader's own scope withheld included.
//
// WHAT THAT DISCLOSES, stated plainly rather than waved at. Above the cap the
// count cannot be told from overflow. At or below it, it can — two lines and
// "and 1 more" out of three could only have withheld one — and that is
// accepted rather than hidden, because the notice is the READER'S OWN and
// their notification centre already counts it for them: the number says
// nothing the app does not. What the re-scope withholds is the SUBJECT, since
// a subject quotes the record it is about and ownership of that record moves
// after the line is written. So the message names what it may and counts what
// it may not, which is the one arrangement that neither quotes a record this
// seat cannot open nor under-reports their own morning.
type digestLines struct {
	quotable []string
	total    int
}

// readDigestLines reads this colleague's batched notices and decides which of
// them the message may quote.
//
// The read runs under seatCtx — the recipient's own authority — and so does the
// re-scope, in a transaction of its own. They are two statements against one
// unchanged fact: which rows this seat may open now.
func (w *notificationDigestWorker) readDigestLines(
	seatCtx context.Context, seat ids.UserID, day time.Time,
) (digestLines, error) {
	held, err := w.notices.DigestBody(seatCtx, seat, day)
	if err != nil {
		return digestLines{}, err
	}
	if len(held) == 0 {
		return digestLines{}, nil
	}
	var nameable map[notices.Target]bool
	if err := database.WithWorkspaceTx(seatCtx, w.pool, func(tx pgx.Tx) error {
		var txErr error
		nameable, txErr = nameableTargets(seatCtx, tx, held)
		return txErr
	}); err != nil {
		return digestLines{}, err
	}
	lines := digestLines{total: len(held)}
	for _, notice := range held {
		if len(lines.quotable) == digestQuoteCap {
			break
		}
		if notice.Target.Named() && !nameable[notice.Target] {
			continue
		}
		lines.quotable = append(lines.quotable, mailcopy.OneLine(notice.Subject))
	}
	return lines, nil
}

// nameableTargets answers which of the records these notices point at their
// recipient may still open.
//
// ONE probe per target type for the whole morning, never one per notice:
// auth.VisibleSubset answers a whole id set in one statement, and it asks BOTH
// halves of the question — the object grant on the type, then the row scope over
// the ids — so a seat whose role no longer reads contacts names none of them
// however the rows are owned.
//
// A type auth.RowScoped does not know never reaches VisibleSubset at all. It is
// absent from the answer, which the caller reads as "count it", and that is the
// fail-closed direction: absent means withheld here exactly as it does inside
// VisibleSubset, where a row that does not exist is absent too.
func nameableTargets(
	ctx context.Context, tx pgx.Tx, held []notices.Notice,
) (map[notices.Target]bool, error) {
	byType := map[string][]ids.UUID{}
	for _, notice := range held {
		if notice.Target.Named() && auth.RowScoped(notice.Target.Type) {
			byType[notice.Target.Type] = append(byType[notice.Target.Type], notice.Target.ID)
		}
	}
	nameable := make(map[notices.Target]bool, len(held))
	for targetType, targetIDs := range byType {
		visible, err := auth.VisibleSubset(ctx, tx, targetType, targetIDs)
		if err != nil {
			return nil, fmt.Errorf("compose: reading which %s rows the recipient may open: %w", targetType, err)
		}
		for id, may := range visible {
			if may {
				nameable[notices.Target{Type: targetType, ID: id}] = true
			}
		}
	}
	return nameable, nil
}

// digestSubject is what a colleague sees in their list: what the message is,
// and how much is in it.
//
// The COUNT and not the first subject, unlike the immediate notice's mail. That
// one is about a single decision and names it; this is a batch, and putting one
// of five records in the subject line would make the message look like it is
// about that record.
func digestSubject(lines digestLines, words mailcopy.Copy) string {
	return fmt.Sprintf("%s (%d)", words.DigestSubject, lines.total)
}

// digestMessage renders the body: why it arrived, what is in it, and the way in.
//
// EVERY interpolated value goes through OneLine. mailer.Send refuses line breaks
// in the recipient and the subject, which are the header fields; the body is the
// sender's to keep honest, and a notice's subject is written by producers that
// bound its length and not its structure.
func digestMessage(lines digestLines, base string, words mailcopy.Copy) string {
	var b strings.Builder
	b.WriteString(words.DigestIntro + "\n\n")
	for _, line := range lines.quotable {
		b.WriteString("  · " + line + "\n")
	}
	if rest := lines.total - len(lines.quotable); rest > 0 {
		fmt.Fprintf(&b, "  "+words.DigestAndMore+"\n", rest)
	}
	mailcopy.Link(&b, base, mailcopy.WorklistFragment, words.DigestOpen)
	return b.String()
}
