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
//
// A VERDICT NAMES THE RULES IT WAS JUDGED UNDER, and may be re-judged when those
// move. It was written once and never revisited, which is right while the rules
// stand still and wrong the moment they change: a prompt corrected to read
// "Dienstag 14 Uhr würde bei uns passen" as a request could not reach the
// messages the old prompt had already judged, so a waiting customer stayed
// invisible for good. owed_verdict_ruleset holds the digest of the prompt that
// judged the row, and a row stamped with anything else is stale. That makes
// "the newer ruleset wins" a fact about the record rather than a claim — see
// SetOwedVerdict for why a different stamp alone is not enough to decide it.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
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

// owedVerdictField is the column a verdict is written to, and the name the
// audit trail and a decline stamp give the question it answers.
const owedVerdictField = "owed_verdict"

// PriorOutbound is OUR own last message in a candidate's thread, as context.
//
// It is what makes a reply readable AS a reply. "Dienstag 14 Uhr würde bei uns
// passen" is a bare statement on its own and an unanswered proposal beside the
// plan it answers — and the classifier judged the first reading, because the
// first reading was all it had ever been shown.
type PriorOutbound struct {
	Subject string
	Body    string // pre-truncated to priorBodyLimit
	At      time.Time
}

// OwedCandidate is one candidate as the classifier's prompt consumes it.
//
// It carries the recipient context deliberately. "Is something owed by me" is
// not answerable from a subject and a body: a message addressed to a desk
// address with the reader on cc reads exactly like one addressed to them, and
// the three examples that opened this work differ from real work mostly in who
// was on the envelope.
type OwedCandidate struct {
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
	// PriorOutbound is our own last message in this thread, when there is one.
	// Nil for a threadless message and for one that opens a conversation.
	PriorOutbound *PriorOutbound
}

// candidateColumns is what both reads select, including our own last message in
// the thread. Shared by both so a change to the shape reaches each of them,
// and so scanCandidates has one row layout to read.
//
// The prior message is keyed on its ID rather than on its subject: subject is
// nullable, and keying presence on it would silently drop the whole context
// block for a subjectless outbound — a failure that reads as the model simply
// judging worse, with nothing to point at.
const candidateColumns = `a.id, a.kind, coalesce(a.subject, ''), coalesce(left(a.body, $%[1]d), ''),
       coalesce(a.has_calendar_part, false),
       coalesce(array_agg(DISTINCT p.address)
                FILTER (WHERE p.role = 'to' AND p.address <> ''), '{}'),
       coalesce(array_agg(DISTINCT p.address)
                FILTER (WHERE p.role = 'cc' AND p.address <> ''), '{}'),
       prior.id, coalesce(prior.subject, ''), coalesce(prior.body, ''), prior.occurred_at`

// priorOutboundJoin finds OUR newest message before this one in the same
// conversation, or nothing.
//
// The conversation test is ourOutboundInThisThread, shared with the settlement
// pass, and why a thread key alone is not evidence of one is written there.
// What this adds is its own, and the two additions are for the same reason:
// this join ships the prior message's BODY to a model.
//
// The join is guarded on the CANDIDATE having a thread and a correspondent at
// all. The helper compares both by equality, so a NULL on either side matches
// nothing — but a guard that says so at the top reads as the intent rather than
// as a consequence, and it lets the planner drop the lateral outright.
//
// The audience and hold clauses are on the prior message too. It is our own
// text, but it goes to the same cloud tier as the message being judged, and a
// thread the confidentiality engine narrowed is exactly the mail that must not.
var priorOutboundJoin = `LEFT JOIN LATERAL (
  SELECT prior.id, prior.subject, left(prior.body, $%[4]d) AS body, prior.occurred_at
    FROM activity prior
   WHERE a.thread_key IS NOT NULL
     AND a.counterparty_email IS NOT NULL
     AND ` + ourOutboundInThisThread("prior", "a") + `
     -- On the prior row and not on the settlement pass's own uses of the same
     -- helper, because this one ships the body to a model. A thread the
     -- confidentiality engine narrowed is exactly the mail that must not go.
     AND prior.audience = 'workspace'
     AND prior.restricted_at IS NULL
     AND (prior.occurred_at, prior.id) < (a.occurred_at, a.id)
   ORDER BY prior.occurred_at DESC, prior.id DESC
   LIMIT 1) prior ON true`

// candidateGroupBy keeps the participant aggregation from multiplying the row.
const candidateGroupBy = `a.id, a.kind, a.subject, a.body, a.has_calendar_part, a.occurred_at,
          prior.id, prior.subject, prior.body, prior.occurred_at`

// scanCandidates reads the rows both candidate queries return.
func scanCandidates(rows pgx.Rows) ([]OwedCandidate, error) {
	defer rows.Close()
	var out []OwedCandidate
	for rows.Next() {
		var m OwedCandidate
		var priorID *ids.UUID
		var priorSubject, priorBody string
		var priorAt *time.Time
		if err := rows.Scan(&m.ID, &m.Kind, &m.Subject, &m.Body,
			&m.HasCalendarPart, &m.To, &m.Cc,
			&priorID, &priorSubject, &priorBody, &priorAt); err != nil {
			return nil, err
		}
		if priorID != nil && priorAt != nil {
			m.PriorOutbound = &PriorOutbound{Subject: priorSubject, Body: priorBody, At: *priorAt}
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// OwedBacklog reads the oldest inbound correspondence carrying no verdict.
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
//
// It never re-judges a verdict: that is OwedRestale's, in its own call under its
// own bound, because a single widened predicate would put stale rows into
// competition with fresh mail for this query's scan cap — see OwedRestale for
// what that costs. The ruleset it takes decides only which declines still
// stand: a message every rung declined under another prompt is unjudged mail
// like any other, and is offered to this one.
func (s *Store) OwedBacklog(ctx context.Context, ruleset string, asOf time.Time, limit, bodyLimit, priorBodyLimit int) ([]OwedCandidate, time.Time, error) {
	// System principal only, like RepliedRequests beside it and for the same
	// reason: this hands a customer's subject and body, and now our own earlier
	// message, to a model. OwedRestale composes no row-scope clause of its own,
	// so without this any principal holding activity:read that ever reached it
	// would read every judged message in the installation.
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalSystem {
		return nil, time.Time{}, apperrors.ErrPermissionDenied
	}
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return nil, time.Time{}, err
	}
	if ruleset == "" {
		return nil, time.Time{}, fmt.Errorf("activities: reading the unjudged backlog needs the prompt its declines are judged against: %w", errRulesetUnnamed)
	}
	var out []OwedCandidate
	var readAt time.Time
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		// The DATABASE's clock, which is the one owed_verdict_at is stamped
		// with. SetOwedVerdict compares the two, so taking this from the
		// caller's clock would compare two unsynchronised sources.
		if err := tx.QueryRow(ctx, `SELECT now()`).Scan(&readAt); err != nil {
			return err
		}
		args := []any{}
		arg := func(v any) int { args = append(args, v); return len(args) }
		body := arg(bodyLimit)
		prior := arg(priorBodyLimit)
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
		horizon, err := s.waitingHorizonFor(ctx, tx, asOf)
		if err != nil {
			return err
		}
		// The backlog is judged for the WORKSPACE, so it carries no reader
		// addresses: the verdict says what a message asks, which is a fact
		// about the message rather than about who is reading it. The waiting
		// queue applies the addressing test when it ranks.
		//
		// The AUDIENCE clause stays, and it is not about who may see the row —
		// it is about what leaves the building. This backlog hands subject and
		// body to a model, and the pass runs as a system principal, which
		// auth.ActivityContentClause lets read every audience. A thread the
		// confidentiality engine narrowed to its participants is exactly the
		// mail that must not be shipped to a cloud tier, and dropping this
		// clause would have done that silently — the statutory-hold check
		// beside it does not cover a confidentiality narrowing.
		waiting, err := waitingReplyExistsClause(ctx, arg, asOf, nil, nil, own, nil, horizon,
			`a.owed_verdict IS NULL AND `+owedVerdictDecline.offeredSQL("a.", fmt.Sprintf("$%d", arg(ruleset)))+
				` AND a.audience = 'workspace' AND a.restricted_at IS NULL`)
		if err != nil {
			return err
		}
		rows, err := tx.Query(ctx, fmt.Sprintf(`
			SELECT `+candidateColumns+`
			  FROM activity a
			  LEFT JOIN activity_participant p ON p.activity_id = a.id
			  `+priorOutboundJoin+`
			 WHERE %[2]s
			 GROUP BY `+candidateGroupBy+`
			 ORDER BY a.occurred_at
			 LIMIT $%[3]d`, body, waiting, arg(limit), prior), args...)
		if err != nil {
			return err
		}
		out, err = scanCandidates(rows)
		return err
	})
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("activities: reading the unjudged waiting backlog: %w", err)
	}
	return out, readAt, nil
}

// OwedRestale reads judged messages whose verdict was reached under other rules.
//
// IT DELIBERATELY DOES NOT ASK THE WAITING QUEUE, and that is the whole design.
// A wrong verdict can exclude itself from the queue that would correct it, two
// ways, both live: past the waiting horizon a row survives only when
// requestCandidateSQL holds, which needs `asks_us`, an accepted task or a
// scheduling label — none of which a wrongly-`informs_us` row has; and the
// queue drops a row carrying a request task, which the same pass mints from the
// old verdict moments earlier. Either one makes the sweep unable to reach the
// rows it exists for. That is the identical self-feeding exclusion that put
// "Dienstag 14 Uhr würde bei uns passen" behind "Nobody here is owed an answer"
// in the first place, and rebuilding it one level up would be a poor joke.
//
// So this selects on the row's OWN columns — exactly idx_activity_owed_ruleset's
// predicate, so the index serves it — and leaves eligibility to the readers
// downstream, each of which re-applies its own rules to the corrected verdict.
// Re-judging a message the queue would not show today costs one cheap-ladder
// call and resurrects nothing by itself.
//
// The audience and hold clauses carry the same meaning they do in OwedBacklog:
// what may leave the building, not who may read it.
func (s *Store) OwedRestale(ctx context.Context, ruleset string, limit, bodyLimit, priorBodyLimit int) ([]OwedCandidate, time.Time, error) {
	// System principal only, like RepliedRequests beside it and for the same
	// reason: this hands a customer's subject and body, and now our own earlier
	// message, to a model. OwedRestale composes no row-scope clause of its own,
	// so without this any principal holding activity:read that ever reached it
	// would read every judged message in the installation.
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalSystem {
		return nil, time.Time{}, apperrors.ErrPermissionDenied
	}
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return nil, time.Time{}, err
	}
	if ruleset == "" {
		return nil, time.Time{}, fmt.Errorf("activities: reading stale verdicts needs the prompt they are stale against: %w", errRulesetUnnamed)
	}
	var out []OwedCandidate
	var readAt time.Time
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		// The database's clock, for SetOwedVerdict's comparison. See OwedBacklog.
		if err := tx.QueryRow(ctx, `SELECT now()`).Scan(&readAt); err != nil {
			return err
		}
		args := []any{}
		arg := func(v any) int { args = append(args, v); return len(args) }
		body := arg(bodyLimit)
		prior := arg(priorBodyLimit)
		rows, err := tx.Query(ctx, fmt.Sprintf(`
			SELECT `+candidateColumns+`
			  FROM activity a
			  LEFT JOIN activity_participant p ON p.activity_id = a.id
			  `+priorOutboundJoin+`
			 WHERE a.direction = 'inbound'
			   AND a.kind IN ('email', 'message')
			   AND a.owed_verdict IS NOT NULL
			   AND `+rulesetStaleSQL("a.owed_verdict_ruleset", "$%[2]d")+`
			   AND a.archived_at IS NULL
			   AND a.audience = 'workspace'
			   AND a.restricted_at IS NULL
			   -- Declined by every rung under THESE rules (MarkOwedVerdictDeclined):
			   -- its verdict stands. A decline under older rules is re-offered.
			   AND `+owedVerdictDecline.offeredSQL("a.", "$%[2]d")+`
			   -- A human saying "this is not sales work" is the one rule from the
			   -- waiting query this read keeps, and it is kept because it is a
			   -- DECISION rather than a derivation. The clauses left behind are
			   -- derived from the verdict and would exclude the rows this exists
			   -- to correct; this one is set by a rep, about their dentist or
			   -- their lawyer, and nothing about re-judging can feed back into
			   -- it. Dropping it would ship a thread somebody dismissed back to
			   -- the model every time the prompt moved.
			   AND NOT EXISTS (SELECT 1 FROM activity_sales_state judged
			         WHERE judged.thread_key = a.thread_key
			           AND judged.kind = a.kind
			           AND judged.channel_provider = coalesce(a.channel_provider, ''))
			 GROUP BY `+candidateGroupBy+`
			 ORDER BY a.occurred_at DESC
			 LIMIT $%[3]d`, body, arg(ruleset), arg(limit), prior), args...)
		if err != nil {
			return err
		}
		out, err = scanCandidates(rows)
		return err
	})
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("activities: reading verdicts judged under other rules: %w", err)
	}
	return out, readAt, nil
}

// rulesetStaleSQL is "this row was judged under other rules", shared by the
// read that finds such rows and the write that replaces them — the two have to
// agree on what stale means, or the sweep re-reads rows the write then refuses.
// A declined question's prompt is compared the same way (declineStamp), so a
// verdict and a decline go stale on one definition.
//
// TWO ARMS rather than IS DISTINCT FROM, which is not a btree search operator:
// the NULL arm is index-served, and it is also the legacy population, judged
// before the column existed. The shape CHECK guarantees an unjudged row carries
// NULL here too, so callers pair this with their own test for a verdict.
func rulesetStaleSQL(col, param string) string {
	return "(" + col + " IS NULL OR " + col + " <> " + param + ")"
}

// SetOwedVerdict writes one verdict, reporting whether it applied.
//
// THE CAS IS `owed_verdict IS NULL`: a concurrent pass that judged the row first
// wins, and this write reports applied=false. An earlier verdict stands rather
// than being overwritten, because two model calls on one message under the SAME
// rules are two opinions and the tree has no rule for preferring the later one.
//
// readAt MUST come from the DATABASE's clock, which OwedBacklog and OwedRestale
// return for exactly this purpose. owed_verdict_at is stamped with now(), so a
// readAt taken from the caller's own clock compares two unsynchronised clocks
// and the guard below simply stops firing — silently, because the write still
// lands and only the race it was meant to refuse gets through.
//
// ACROSS rules it does have one, and readAt is what makes it decidable. A
// different ruleset proves the two judgements differ; it says nothing about
// which is later, and the classifier reads a batch, spends a model call, then
// writes — so during a rolling deploy an old binary can hold a pre-deploy
// digest across that gap, land its UPDATE after a new binary judged the row,
// and pass a bare `ruleset <> $current` test while overwriting a strictly newer
// answer with a strictly older one. Comparing owed_verdict_at against the
// moment the batch was READ closes it: a row judged since then is left alone,
// whichever rules judged it. FOR UPDATE serialises the pair without deciding
// it, which is why the timestamp and not the lock is the answer.
//
// The audience, hold and archive clauses are re-tested at WRITE time, not merely
// in the read that selected the row. The classifier reads a batch, spends a model call
// per message and writes the answers back; a human or a privacy verdict can
// narrow the row inside that window, and a write landing after the narrowing
// would stamp a judgement on a message the queue's readers may no longer open.
func (s *Store) SetOwedVerdict(ctx context.Context, id ids.UUID, verdict, ruleset string, readAt time.Time) (applied bool, err error) {
	if verdict != OwedVerdictAsksUs && verdict != OwedVerdictInformsUs {
		return false, fmt.Errorf("activities: %q is not a verdict this column accepts", verdict)
	}
	if ruleset == "" {
		return false, fmt.Errorf("activities: a verdict must name the prompt that judged it: %w", errRulesetUnnamed)
	}
	if err := auth.Require(ctx, "activity", principal.ActionUpdate); err != nil {
		return false, err
	}
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		// The prior verdict, read under the row lock so the image and the write
		// cannot disagree. ErrNoRows means the row is gone or narrowed, which is
		// the same unapplied answer the CAS gives.
		var priorVerdict, priorRuleset *string
		switch err := tx.QueryRow(ctx, `
			SELECT owed_verdict, owed_verdict_ruleset FROM activity
			WHERE id = $1 AND archived_at IS NULL
			  AND audience = 'workspace' AND restricted_at IS NULL
			FOR UPDATE`, id).Scan(&priorVerdict, &priorRuleset); {
		case errors.Is(err, pgx.ErrNoRows):
			return nil
		case err != nil:
			return fmt.Errorf("activities: reading the verdict this write replaces: %w", err)
		}
		// Read back through RowsAffected: no row means the message was judged
		// under these same rules already, or judged by a newer pass since this
		// batch was read. Neither is an error — the caller reports the write as
		// unapplied and moves on.
		tag, err := tx.Exec(ctx, `
			UPDATE activity
			   SET owed_verdict = $2, owed_verdict_at = now(), owed_verdict_ruleset = $3
			WHERE id = $1
			  AND `+rulesetStaleSQL("owed_verdict_ruleset", "$3")+`
			  AND (owed_verdict_at IS NULL OR owed_verdict_at < $4)
			  AND archived_at IS NULL
			  AND audience = 'workspace' AND restricted_at IS NULL`, id, verdict, ruleset, readAt)
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
		// WITH a before-image, because this write can now replace. A first
		// judgement images two explicit nulls rather than nothing: "there was no
		// verdict here" is the fact, and storekit refuses an absent image on an
		// update precisely so that fact is recorded rather than assumed.
		//
		// No outbox row beside it. The write shape asks for an event where one
		// has a consumer outside the transaction, and this has none: the verdict
		// is read by the queue's own next read, from the column. An event type
		// with no subscriber is a catalog entry and a schema to keep current for
		// nobody, and the tree already spells this shape — an audited derived
		// write with no event — in transcriptread.go and in capture's settings
		// writers.
		if _, err := storekit.Audit(ctx, tx, "update", "activity", id,
			map[string]any{owedVerdictField: priorVerdict, "owed_verdict_ruleset": priorRuleset},
			map[string]any{owedVerdictField: verdict, "owed_verdict_ruleset": ruleset}); err != nil {
			return err
		}
		return nil
	})
	return applied, err
}
