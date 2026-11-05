// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package companyscan

// The recent exchanges' own words, read as the viewer.

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

const (
	// scanMessages bounds how many exchanges the model reads. Past twenty the
	// prompt is a mailbox, not a conversation, and the findings that matter
	// are in the newest ones anyway.
	scanMessages = 20
	// scanBodyChars bounds one body. A commitment, a question or a risk is
	// stated in a message's opening; past this is quoted history and a
	// signature, which the model would otherwise cite as if it were new.
	scanBodyChars = 1200
	// scanWindowDays is how far back an exchange still needs a contact. A
	// question from a year ago that nobody answered has been answered by time.
	scanWindowDays = 180
)

// scopeAll is the predicate a reader with no row scope gets.
const scopeAll = "TRUE"

// readWords reads the newest exchanges on the account, oldest first, with
// their own words. A canceled or no-show meeting is no exchange, and would
// push real correspondence out of the window.
//
// Gated on ActivityContentClause, not the discover clause: the bodies go into
// a prompt, and a reader allowed to know a message exists is not thereby
// allowed to have it read for them. The activity grant is REFUSED rather than
// softened, for the reason the role proposals give — an account whose
// messages this reader may not read is not a quiet account, and a scan that
// read as one would be advising from a fact about their permissions.
//
// Reachability is activities.CompanyLinkedActivityExists, the walk every reader
// of the account's timeline uses, so a message linked through one of the
// account's deals counts as the account's own correspondence.
func readWords(
	ctx context.Context, tx pgx.Tx, companyID ids.CompanyID, now time.Time,
) ([]MessageIn, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return nil, err
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	companyPos := arg(companyID)
	sincePos := arg(now.AddDate(0, 0, -scanWindowDays))
	nowPos := arg(now)
	charsPos := arg(scanBodyChars)
	capPos := arg(scanMessages)
	scope, err := auth.ActivityContentClause(ctx, "a", arg)
	if err != nil {
		return nil, err
	}
	if scope == "" {
		scope = scopeAll
	}
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT a.id, a.kind, coalesce(a.direction, ''), coalesce(a.subject, ''), a.occurred_at,
		       coalesce(left(a.body, $%[3]d), ''),
		       greatest(coalesce(char_length(a.body), 0) - $%[3]d, 0),
		       coalesce(a.thread_key || '/' || a.kind || '/' || coalesce(a.channel_provider, ''), '')
		  FROM activity a
		 WHERE %[6]s AND a.archived_at IS NULL
		   AND %[2]s AND (%[5]s)
		   AND %[1]s
		 ORDER BY a.occurred_at DESC, a.id DESC
		 LIMIT $%[4]d`,
		activities.CompanyLinkedActivityExists(companyPos), occurredday.FromSQL("a", fmt.Sprintf("$%d", sincePos)), charsPos, capPos, scope,
		relstrength.InteractionCountsAtSQL("a", fmt.Sprintf("$%d::timestamptz", nowPos))), args...)
	if err != nil {
		return nil, fmt.Errorf("companyscan: reading the account's exchanges: %w", err)
	}
	defer rows.Close()
	var out []MessageIn
	var threads []string
	for rows.Next() {
		var message MessageIn
		var thread string
		if err := rows.Scan(&message.ID, &message.Kind, &message.Direction, &message.Subject,
			&message.At, &message.Text, &message.UnreadChars, &thread); err != nil {
			return nil, err
		}
		threads = append(threads, thread)
		message.At = message.At.UTC()
		// The quote gate reads Text too, so a quoted reply can be neither shown
		// nor cited as this message's words.
		message.Text = textlang.SenderText(message.Text)
		out = append(out, message)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Newest first is the read's order, because the cap keeps the newest. So
	// whenever a thread is in the read, its newest message is too. Oldest first
	// is the model's, because a conversation is read forwards.
	slices.Reverse(out)
	slices.Reverse(threads)
	labelThreads(out, threads)
	if err := markSettled(ctx, tx, companyID, out, now); err != nil {
		return nil, err
	}
	return out, nil
}

// labelThreads gives the messages of one thread one opaque label.
func labelThreads(messages []MessageIn, threads []string) {
	labels := map[string]string{}
	for i, thread := range threads {
		if thread == "" {
			continue
		}
		if _, seen := labels[thread]; !seen {
			labels[thread] = fmt.Sprintf("t%d", len(labels)+1)
		}
		messages[i].Thread = labels[thread]
	}
}

// markSettled asks the activities module which messages their thread settled.
func markSettled(ctx context.Context, tx pgx.Tx, companyID ids.CompanyID, messages []MessageIn, now time.Time) error {
	cited := make([]ids.UUID, 0, len(messages))
	for _, message := range messages {
		cited = append(cited, message.ID)
	}
	settled, err := settledFor(ctx, tx, companyID, cited, now)
	if err != nil {
		return fmt.Errorf("companyscan: reading which exchanges their threads settled: %w", err)
	}
	for i := range messages {
		messages[i].Settled = settled[messages[i].ID]
	}
	return nil
}
