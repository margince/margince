// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

// Pass two: what the conversations that matter actually SAID.
//
// The arc knows which stretches of the relationship bear on today. This goes
// back for their bodies, bounded — a handful of threads, a few messages each,
// each message trimmed — so the preparation can name what the other side asked
// for rather than that they wrote three times in July.
//
// This is the difference between a brief that says "there was a long thread
// about requirements" and one that says "they asked for issue tracking, quote
// tracking and multi-channel capture, and twice asked how to start". Nothing
// downstream can recover the second from subject lines; a plan built without
// this reads as though it had been written for a different account.
//
// Bounded three ways on purpose. Six threads, four messages each, twelve
// hundred characters per message: enough for the model to quote a real ask,
// far short of handing it a mailbox.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const (
	excerptThreads       = 6
	excerptRowsPerThread = 4
	excerptChars         = 1200
)

// ExcerptIn is one message, with enough of its body to be quotable.
type ExcerptIn struct {
	ActivityID string
	Subject    string
	Direction  string
	At         time.Time
	Text       string
}

const excerptQuery = `
	SELECT a.id, COALESCE(a.subject, ''), COALESCE(a.direction, ''), a.occurred_at,
	       left(COALESCE(a.body, ''), %[3]d)
	FROM activity a
	WHERE a.id = ANY($%[2]d)
	  AND a.archived_at IS NULL
	  AND %[1]s
	ORDER BY a.occurred_at`

// readExcerpts reads the bodies of the chosen activities.
//
// Gated on the CONTENT clause rather than the audience arm used in pass one:
// this returns what a conversation said, which is the strongest thing the brief
// hands anyone, and the weaker clause is documented as covering safe markers
// alone. A row that does not pass simply does not come back — pass one already
// counted it, and the omission already says so.
func (s *Service) readExcerpts(
	ctx context.Context, tx pgx.Tx, wanted []string,
) ([]ExcerptIn, error) {
	if len(wanted) == 0 {
		return nil, nil
	}
	parsed := make([]ids.UUID, 0, len(wanted))
	for _, raw := range wanted {
		id, err := ids.Parse(raw)
		if err != nil {
			// The ids come from this package's own read, so a bad one is a bug
			// here rather than bad input — but a silent skip would make the
			// symptom "the plan is vague" three layers away.
			return nil, fmt.Errorf("excerpt id %q: %w", raw, err)
		}
		parsed = append(parsed, id)
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	scope, err := auth.ActivityContentClause(ctx, "a", arg)
	if err != nil {
		return nil, err
	}
	if scope == "" {
		scope = scopeAll
	}
	rows, err := tx.Query(ctx, fmt.Sprintf(excerptQuery, scope, arg(parsed), excerptChars), args...)
	if err != nil {
		return nil, fmt.Errorf("read the conversation excerpts: %w", err)
	}
	defer rows.Close()

	var excerpts []ExcerptIn
	for rows.Next() {
		var row ExcerptIn
		var id ids.UUID
		if err := rows.Scan(&id, &row.Subject, &row.Direction, &row.At, &row.Text); err != nil {
			return nil, fmt.Errorf("read the conversation excerpts: %w", err)
		}
		row.ActivityID = id.String()
		excerpts = append(excerpts, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read the conversation excerpts: %w", err)
	}
	return excerpts, nil
}

// excerptTargets picks which activities are worth a body read: the first and
// last of each ranked moment's threads, which between them carry how a
// conversation opened and where it landed.
func excerptTargets(moments []ArcMoment) []string {
	var wanted []string
	seen := map[string]bool{}
	threadsTaken := 0
	for _, moment := range moments {
		for _, current := range moment.Threads {
			if current.Readable == 0 {
				continue
			}
			if threadsTaken == excerptThreads {
				return wanted
			}
			threadsTaken++
			for _, id := range pickRows(current) {
				if !seen[id] {
					seen[id] = true
					wanted = append(wanted, id)
				}
			}
		}
	}
	return wanted
}

// pickRows takes the ends of a thread and, when there is room, the messages
// just inside them. IDs arrive newest first.
func pickRows(current thread) []string {
	if len(current.IDs) <= excerptRowsPerThread {
		return current.IDs
	}
	last := len(current.IDs) - 1
	return []string{
		current.IDs[0],    // the newest: where it landed
		current.IDs[1],    // and what preceded it
		current.IDs[last], // the oldest: how it opened
		current.IDs[last-1],
	}
}
