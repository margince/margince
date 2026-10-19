// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"
	"strings"

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

// Digest totals include only the notices whose content remains visible.
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
	seatCtx context.Context, seat ids.UserID, window notices.Window,
) (digestLines, error) {
	held, err := w.notices.DigestBody(seatCtx, seat, window)
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
	lines := digestLines{}
	for _, notice := range held {
		if notice.Target.Named() && !nameable[notice.Target] {
			continue
		}
		lines.total++
		if len(lines.quotable) < digestQuoteCap {
			line := mailcopy.OneLine(notice.Subject)
			if notice.Body != "" {
				line += " — " + mailcopy.OneLine(notice.Body)
			}
			lines.quotable = append(lines.quotable, line)
		}
	}
	return lines, nil
}

// Batch visibility checks cover both object grants and row scope.
// Unknown target types remain withheld.
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
			return nil, fmt.Errorf("compose: reading recipient visibility: %w", err)
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
