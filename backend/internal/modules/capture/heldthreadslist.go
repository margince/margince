// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// What one seat's mailbox is holding right now, in one list.
//
// An owner could already open or hold a thread from a record's timeline, one
// thread at a time. What they could not do is ask "what am I holding" — and
// during a classifier outage that is the only question worth asking, because
// every new thread lands pending and stays there until the model answers.
//
// One seat's own threads and nobody else's. What a person's mailbox is holding
// is the most private thing this module knows: the list names threads a
// classifier judged legal, personnel or personal, so a colleague's view of it
// would disclose exactly what holding them exists to prevent. There is no id
// here that reaches another seat's list and no admin view of one.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// HeldThread is one thread this seat is holding, and why.
type HeldThread struct {
	ThreadKey string
	// Status is the ledger's own word: pending while no verdict has landed,
	// held or unsure once one has, held_by_owner where the owner said so.
	Status string
	// Kind is what the classifier concluded the thread is ABOUT — legal,
	// personnel, financial_corporate and the rest — empty while pending.
	Kind string
	// Subject and OccurredAt come from the message that opened the thread, so
	// a reader can tell one held thread from another.
	Subject    string
	OccurredAt *time.Time
	// HasActivity separates "no message to read a subject from" — erased while
	// the verdict stood, which the ledger deliberately survives — from a
	// message somebody sent with a blank subject line. Collapsing the two would
	// tell a reader their evidence was destroyed when it is sitting there
	// unnamed.
	HasActivity bool
	// Attempts is how many times a verdict has been asked for. It is the
	// outage signal: a pending row whose attempts stop climbing is a model
	// that stopped answering, not a thread that is merely slow.
	Attempts int
}

// Pending reports that no verdict has landed yet, which is the row an owner
// acts on during an outage.
func (t HeldThread) Pending() bool { return t.Status == "pending" }

// HeldThreadsFor lists what the calling seat's mailbox is holding.
//
// PENDING FIRST, then by recency. The order is the feature: during an outage
// the pending rows are the ones nobody has decided, and burying them under
// decided ones sorted by date is what makes an owner scroll to find the work.
//
// `cleared` and `shared_by_owner` are absent because they are not held — the
// list answers "what is not visible to my colleagues", and a thread the
// classifier opened is visible. Every other status withholds, including
// `unsure`: a thread the model could not judge is held exactly like one it
// judged legal, and an owner who is not shown it cannot release it.
func HeldThreadsFor(ctx context.Context, db *database.DB) ([]HeldThread, error) {
	if err := auth.RequireHuman(ctx); err != nil {
		return nil, err
	}
	actor, ok := principal.Actor(ctx)
	if !ok || actor.UserID == ids.Nil {
		return nil, apperrors.ErrPermissionDenied
	}
	// The arguments the clause and the query share, numbered together: the
	// content clause mints its own placeholders and they must not collide with
	// the seat's.
	args := []any{actor.UserID}
	arg := func(v any) int {
		args = append(args, v)
		return len(args)
	}
	// The subject is activity CONTENT, so it is read through the same clause
	// every other reader of that table composes. This is the caller's own
	// mailbox, so the clause admits them — but writing the read unconditionally
	// would leave a projection of activity text that no audience rule governs,
	// which is the shape audiencereaders_test refuses. A held thread the caller
	// may not read the content of still appears, with no subject.
	content, err := auth.ActivityContentClause(ctx, "a", arg)
	if err != nil {
		return nil, err
	}
	var out []HeldThread
	err = db.Tx(ctx, func(tx pgx.Tx) error {
		// The activity join is a LEFT one and the ledger is the driving table:
		// a verdict outlives the message it was raised about (first_activity_id
		// is ON DELETE SET NULL, so an erasure inside the window nulls it
		// rather than dropping the row), and an inner join would silently drop
		// exactly the threads whose evidence is gone while the hold stands.
		//
		// The content clause rides the JOIN rather than the WHERE for the same
		// reason: in the WHERE it would turn the outer join into an inner one
		// and drop every row whose activity is absent or withheld.
		rows, err := tx.Query(ctx, `
			SELECT v.thread_key,
			       v.status,
			       coalesce(v.kind, '')    AS kind,
			       a.subject,
			       a.occurred_at,
			       v.attempts
			  FROM capture_thread_verdict v
			  LEFT JOIN activity a
			    ON a.id = v.first_activity_id
			   AND a.archived_at IS NULL
			   AND `+content+`
			 WHERE v.user_id = $1
			   AND v.status NOT IN ('cleared', 'shared_by_owner')
			 ORDER BY (v.status = 'pending') DESC,
			          a.occurred_at DESC NULLS LAST,
			          v.created_at DESC`, args...)
		if err != nil {
			return fmt.Errorf("capture: listing a seat's held threads: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var t HeldThread
			var subject *string
			if err := rows.Scan(&t.ThreadKey, &t.Status, &t.Kind,
				&subject, &t.OccurredAt, &t.Attempts); err != nil {
				return fmt.Errorf("capture: listing a seat's held threads: %w", err)
			}
			// NULL is "no message to read one from"; an empty string is a
			// message somebody sent with a blank subject line. The two read
			// differently on screen, so they stay different here.
			t.HasActivity = subject != nil
			if subject != nil {
				t.Subject = *subject
			}
			out = append(out, t)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
