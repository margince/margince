// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Which part of a conversation one read looks at.
//
// Separate from the pass that decides WHICH conversations are due, because the
// two answer different questions and the second one grew a judgement: the read
// used to take the newest six messages and nothing else, and a backfill was
// therefore marked read without being read. What that judgement is, and the
// order it asks its two questions in, is the whole of this file.

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
)

// threadMessages reads one window of a conversation, oldest first, so the
// prompt reads in the order the exchange happened.
//
// WHICH window is the whole of this function's judgement, and it used to have
// none: it always read the newest six. A backfill made the thread due again —
// the count changed — the same newest six were re-read, and the new count was
// recorded. The inserted older messages never reached the model and the thread
// then looked read, which is the bad direction: the state says handled, nothing
// errored, and nothing says content was skipped.
//
// Two ends, asked in this order:
//
//   - NEW MAIL FIRST. Messages newer than the last read are why the
//     conversation is interesting now, and a thread that walked backwards
//     through its history while a reply sat unread would be reading the least
//     useful end of it.
//   - THEN THE UNREAD OLDER RANGE. With nothing new, a message older than where
//     reading started is a backfill, and the window is the newest of THOSE — so
//     a long thread walks backwards one window per pass until its history is
//     covered, rather than never reaching it.
//
// Neither end left: the newest window, which is what a thread with no scan row
// gets and what every thread got before.
func threadMessages(ctx context.Context, tx pgx.Tx, thread *settledThread) ([]threadMessage, error) {
	// The upper bound of the window. Nothing to read older means the newest
	// end; a backfill below where reading started means walk back from there.
	var olderThan *time.Time
	if thread.ReadTo != nil && !thread.Newest.After(*thread.ReadTo) && thread.ReadFrom != nil {
		olderThan = thread.ReadFrom
	}
	messages, err := threadWindow(ctx, tx, thread.Key, olderThan)
	if err != nil {
		return nil, err
	}
	// Nothing older LEFT is not the same as nothing to read. A thread grows at
	// the same instant as its newest message — the count moves and the clock
	// does not — and one whose history is fully covered has an empty older
	// window every time. Both fall back to the newest window, which is where
	// the new content is.
	if len(messages) == 0 && olderThan != nil {
		messages, err = threadWindow(ctx, tx, thread.Key, nil)
		if err != nil {
			return nil, err
		}
	}
	return finishThreadWindow(ctx, thread, messages), nil
}

// threadWindow reads one window, oldest first. olderThan bounds it above; nil
// is the newest end.
func threadWindow(ctx context.Context, tx pgx.Tx, key string, olderThan *time.Time) ([]threadMessage, error) {
	// The remainder is measured in the same statement that cuts the body:
	// asked for afterwards it would be a second read of a row that may have
	// changed, and the number would then describe a different message from the
	// one in the prompt.
	rows, err := tx.Query(ctx, `
		SELECT id, coalesce(direction, ''), coalesce(subject, ''),
		       coalesce(left(body, $1), ''),
		       greatest(coalesce(char_length(body), 0) - $1, 0), occurred_at
		  FROM (SELECT id, direction, subject, body, occurred_at
		          FROM activity
		         WHERE thread_key = $2 AND kind = 'email' AND archived_at IS NULL
		           -- Null means no bound: the newest window, which is the
		           -- ordinary case and the one a thread starts from.
		           AND ($4::timestamptz IS NULL OR occurred_at < $4)
		         ORDER BY occurred_at DESC, id DESC
		         LIMIT $3) tail
		 ORDER BY occurred_at, id`, extractBodyLimit, key, extractThreadMessages, olderThan)
	if err != nil {
		return nil, fmt.Errorf("read the conversation: %w", err)
	}
	defer rows.Close()
	var out []threadMessage
	for rows.Next() {
		var message threadMessage
		if err := rows.Scan(&message.ID, &message.Direction, &message.Subject,
			&message.Body, &message.UnreadRunes, &message.At); err != nil {
			return nil, err
		}
		out = append(out, message)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// finishThreadWindow records where the window started and reports what the
// body limit cut, once the window is settled.
//
// After the fallback and not inside the read, because both are about the
// messages that actually reach the prompt: a cursor set from a window that was
// then discarded would move the reading backwards past text nobody saw.
func finishThreadWindow(ctx context.Context, thread *settledThread, out []threadMessage) []threadMessage {
	if len(out) > 0 {
		// The window's own lower end, so the mark can lower the cursor to it.
		// Rows come back oldest first, so it is the first.
		oldest := out[0].At
		thread.ReadFromNow = &oldest
	}
	unread, cut := 0, 0
	for _, message := range out {
		if message.UnreadRunes > 0 {
			unread += message.UnreadRunes
			cut++
		}
	}
	if cut > 0 {
		// Said once per conversation rather than once per message: what a
		// reader needs is how much of THIS exchange the reading was drawn
		// from, and a line per body would bury that under the long threads it
		// is most true of.
		slog.WarnContext(ctx, "part of this conversation was not read; the events extracted from it are drawn from the head of each mail and anything stated below the cut is absent rather than judged",
			"thread_key", thread.Key, "messages_cut", cut, "messages_read", len(out),
			"body_limit", extractBodyLimit, "unread_runes", unread)
	}
	return out
}
