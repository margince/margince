// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// Whether an inbound message asks its recipient side for something.
//
// The waiting queue proves a customer wrote and nobody replied. It cannot tell
// an unanswered question from a report, a receipt or a monthly statement, and
// that difference is most of what a rep wanted to know — the three examples that
// opened this work were a colleague's invoice thread, a meeting invitation and a
// reporting mail, all correctly "unanswered" and none of them anybody's work.
//
// TWO DECISIONS ARE MADE HERE, and both were argued before any of this was
// written.
//
// The verdict is WORKSPACE-GLOBAL rather than per reader. "Does this message ask
// the recipient side for something" is a property of the message; who owes the
// answer is a different question the queue already answers, from the record the
// thread is filed under (WaitingReply.OwnerID). A per-reader verdict would be a
// second answer to the first question and would disagree with it.
//
// The write is AUDITED AND EVENTED, where capture_label beside it is neither.
// The label's exemption is a stated hard-floor rule about routing attention;
// it covers that column, not every column that ever routes attention. This is a
// model-derived claim about a customer's message, and "which records did the
// classifier touch" has to remain answerable from audit_log like every other
// derived write in this tree.
//
// What it may NOT do is hide anything. The verdict moves a row's band inside the
// queue and never removes one — the same floor capture_label sits under. If a
// verdict is ever allowed to suppress, it needs its own figure in
// /worklist/hidden first, like every other rule that hides.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The closed set of verdicts. Two, because the question is a yes or a no and a
// third value would be a confidence reading wearing a verdict's clothes — the
// classifier reports confidence separately and withholds the verdict below its
// floor, which is what "unjudged" already means.
const (
	// OwedVerdictAsksUs is a message whose sender is waiting on an answer.
	OwedVerdictAsksUs = "asks_us"
	// OwedVerdictInformsUs is a message that tells us something and asks
	// nothing: a report, a receipt, a notification, a statement.
	OwedVerdictInformsUs = "informs_us"
)

// UnjudgedMessage is one candidate as the classifier's prompt consumes it.
//
// It carries the recipient context deliberately. "Is something owed by me" is
// not answerable from a subject and a body: a message addressed to a desk
// address with the reader on cc reads exactly like one addressed to them, and
// the three examples that opened this work differ from real work mostly in who
// was on the envelope.
type UnjudgedMessage struct {
	ID      ids.UUID
	Kind    string
	Subject string
	Body    string // pre-truncated to bodyLimit
	// To and Cc are the addresses the message was sent to, so the model can
	// tell a direct ask from a copy. Bounded by the caller.
	To []string
	Cc []string
	// HasCalendarPart says the message carried a calendar payload. Evidence,
	// not a verdict: an invitation that asks a question is still work.
	HasCalendarPart bool
}

// UnjudgedInbound reads the oldest inbound correspondence carrying no verdict.
//
// The candidates are the messages the WAITING QUEUE would show, narrowed to the
// unjudged ones — never a bare index scan over unjudged mail. Survivorship in
// that queue is relational and time-dependent: it turns on links to live
// records, on anti-joins for a later reply, on the colleague rule and on the
// horizon. A partial index cannot encode any of that, so a backlog built from
// one would spend model calls on messages nobody will ever see.
//
// The index behind the column accelerates "unjudged", which is the one part of
// the question that IS a property of the row.
func (s *Store) UnjudgedInbound(ctx context.Context, asOf time.Time, limit, bodyLimit int) ([]UnjudgedMessage, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return nil, err
	}
	var out []UnjudgedMessage
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		args := []any{}
		arg := func(v any) int { args = append(args, v); return len(args) }
		body := arg(bodyLimit)
		own, err := s.ownDomainList(ctx, tx)
		if err != nil {
			return err
		}
		// The unjudged predicate goes INSIDE the waiting statement, before its
		// scan cap. Outside it, the filter would select from the newest 200
		// waits rather than from the backlog — and once those were judged, an
		// older unjudged message would be unreachable for good while the
		// backlog reported itself empty.
		//
		// The rest of the partial index's predicate rides along, so the index
		// and this query ask the same question and the planner can use it.
		waiting, err := waitingReplyExistsClause(ctx, arg, asOf, nil, nil, own,
			`a.owed_verdict IS NULL AND a.audience = 'workspace' AND a.restricted_at IS NULL`)
		if err != nil {
			return err
		}
		rows, err := tx.Query(ctx, fmt.Sprintf(`
			SELECT a.id, a.kind, coalesce(a.subject, ''), coalesce(left(a.body, $%[1]d), ''),
			       coalesce(a.has_calendar_part, false),
			       coalesce(array_agg(DISTINCT p.address)
			                FILTER (WHERE p.role = 'to' AND p.address <> ''), '{}'),
			       coalesce(array_agg(DISTINCT p.address)
			                FILTER (WHERE p.role = 'cc' AND p.address <> ''), '{}')
			  FROM activity a
			  LEFT JOIN activity_participant p ON p.activity_id = a.id
			 WHERE %[2]s
			 GROUP BY a.id, a.kind, a.subject, a.body, a.has_calendar_part, a.occurred_at
			 ORDER BY a.occurred_at
			 LIMIT $%[3]d`, body, waiting, arg(limit)), args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var m UnjudgedMessage
			if err := rows.Scan(&m.ID, &m.Kind, &m.Subject, &m.Body,
				&m.HasCalendarPart, &m.To, &m.Cc); err != nil {
				return err
			}
			out = append(out, m)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("activities: reading the unjudged waiting backlog: %w", err)
	}
	return out, nil
}

// SetOwedVerdict writes one verdict, reporting whether it applied.
//
// THE CAS IS `owed_verdict IS NULL`: a concurrent pass that judged the row first
// wins, and this write reports applied=false. An earlier verdict stands rather
// than being overwritten, because two model calls on one message are two
// opinions and the tree has no rule for preferring the later one.
//
// The audience, hold and archive clauses are re-tested at WRITE time, not merely
// in the read that selected the row. The classifier reads a batch, spends a model call
// per message and writes the answers back; a human or a privacy verdict can
// narrow the row inside that window, and a write landing after the narrowing
// would stamp a judgement on a message the queue's readers may no longer open.
func (s *Store) SetOwedVerdict(ctx context.Context, id ids.UUID, verdict string) (applied bool, err error) {
	if verdict != OwedVerdictAsksUs && verdict != OwedVerdictInformsUs {
		return false, fmt.Errorf("activities: %q is not a verdict this column accepts", verdict)
	}
	if err := auth.Require(ctx, "activity", principal.ActionUpdate); err != nil {
		return false, err
	}
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		// The CAS is `owed_verdict IS NULL`, read back through RowsAffected: no
		// row means the message was judged already, narrowed since it was read,
		// or put under a statutory hold. None of those is an error — the caller
		// reports the write as unapplied and moves on.
		tag, err := tx.Exec(ctx, `
			UPDATE activity SET owed_verdict = $2, owed_verdict_at = now()
			WHERE id = $1 AND owed_verdict IS NULL
			  AND archived_at IS NULL
			  AND audience = 'workspace' AND restricted_at IS NULL`, id, verdict)
		if err != nil {
			return fmt.Errorf("activities: setting the owed verdict: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return nil
		}
		applied = true
		// Audited in the SAME transaction as the column write, so a verdict that
		// landed is one audit_log can account for. Without it the question this
		// tree answers for every other derived write — which records did this
		// pass touch — would have no answer for the one pass that reads customer
		// correspondence and acts on what it read.
		//
		// No outbox row beside it. The write shape asks for an event where one
		// has a consumer outside the transaction, and this has none: the verdict
		// is read by the queue's own next read, from the column. An event type
		// with no subscriber is a catalog entry and a schema to keep current for
		// nobody, and the tree already spells this shape — an audited derived
		// write with no event — in transcriptread.go and in capture's settings
		// writers.
		if _, err := storekit.AuditEvent(ctx, tx, "update", "activity", id,
			map[string]any{"owed_verdict": verdict}); err != nil {
			return err
		}
		return nil
	})
	return applied, err
}
