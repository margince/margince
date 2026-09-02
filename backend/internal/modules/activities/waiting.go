// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// Who is waiting for a reply.
//
// The deal page already answers this for ONE deal, by walking that deal's
// timeline newest-first and stopping at the first outbound. This is the same
// question asked of the whole workspace at once, and it cannot be the same walk:
// a per-deal scan cannot find the person with no deal, and it cannot be run
// once per record on a page that must render in one read.
//
// So it is a query, and the two spellings are held together by a test that
// feeds both the same timeline and requires the same answer.
//
// WHY THIS IS ITS OWN READ rather than a filter over the at-risk deals: a fresh
// inbound makes a deal LESS quiet, so the deal drops out of the quiet-deal
// candidate set exactly when somebody starts waiting on it. Deriving "waiting"
// from "quiet" would therefore lose the newest and most urgent cases, which are
// the ones a rep most needs.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// WaitingReply is one inbound message nobody has answered.
type WaitingReply struct {
	// ActivityID is the message itself — what a draft would reply to.
	ActivityID ids.UUID
	Subject    string
	// Sender is the address the message came from, so a caller can tell a
	// person waiting from a machine sending. Empty when no sender was recorded.
	Sender string
	// OccurredAt is when they wrote, which is what the wait is measured from.
	OccurredAt time.Time
	// The record the thread is filed under, when it names one.
	PersonID       ids.UUID
	OrganizationID ids.UUID
	DealID         ids.UUID
	// HasOpenDeal reports whether an open deal is on this thread. It is what
	// lets a caller keep an old wait that still has money behind it, and drop
	// one that does not.
	//
	// Read through the SAME visibility-gated links as the record ids above, so
	// it means "an open deal this reader can see" rather than "an open deal
	// exists". The looser reading would let somebody learn a deal is there by
	// watching a row they can see decline to go stale.
	HasOpenDeal bool
	// OwnerID is who owes this reply, resolved from the record the thread is
	// filed under. Zero when no record on it names an owner.
	//
	// PRECEDENCE, first owner found: deal, lead, person, organization. It is the
	// order of how specific the claim is — a thread on a deal is that deal
	// owner's to answer whatever else it touches, and a person outranks their
	// company because the company owner is answerable for the account rather
	// than for every conversation inside it.
	//
	// A separate question from the record ids above, which the caller picks a
	// DISPLAY record from by link priority. The two are allowed to differ: those
	// answer "what is this about", this one answers "who owes the reply".
	//
	// Resolved through the SAME visibility-gated links, so an owner appears only
	// where the reader may see the record naming them. Read off an ungated join
	// it would publish who owns a record the reader cannot open.
	OwnerID ids.UUID
}

// WaitingScanCap bounds the work one read does. Beyond this the answer is
// "there are more", never a silent truncation reported as a total.
//
// Exported because the caller has to compare against it. The seam filters what
// this read returns — machine senders out, duplicate threads folded — so only a
// comparison against the ROWS THIS RETURNED can tell a truncated scan from a
// complete one, and the number doing the comparing has to be this one rather
// than a copy that can drift from it.
const WaitingScanCap = 200

// waitingHorizonDays is how far back a wait can reach and still be work.
//
// Past this, an unanswered message is history rather than an obligation: the
// conversation it belonged to has ended one way or another, and nobody is
// sitting at the other end of it. The horizon is coarse on purpose — the bands
// that separate an urgent wait from a stale one are the caller's, and they
// judge what survives this.
//
// A thread with an open deal on it is exempt. That is the one case where a long
// silence still costs money, and the caller says the same thing in its own
// staleness rule; a horizon that outranked it would leave that rule with
// nothing to act on.
//
// Applied BEFORE the cap for the same reason the machine rule is, and the
// reason is worth restating because it is the whole shape of this query: a
// filter after LIMIT lets two hundred rows nobody wants fill the scan and push
// a real customer past it, and the page then says nobody is waiting.
const waitingHorizonDays = 90

// What "still live" means, per record type, as one spelling each.
//
// Both predicates take a table alias, because every reader needs them under a
// different one. They exist as constants because this file needed each rule
// twice and lasttouch.go states the same two in its own dispatch: four copies
// of "a deal is open" in one package is four places to edit when archiving
// changes, and nothing fails when the fourth is missed.
//
// The lead list is the WORKING part of the lifecycle. A promoted or
// disqualified lead is finished business, and a wait on one is history rather
// than work.
const (
	openDealPredicate    = `%[1]s.status = 'open' AND %[1]s.archived_at IS NULL`
	workingLeadPredicate = `%[1]s.status IN ('new', 'contacted', 'engaged') AND %[1]s.archived_at IS NULL`
)

// neverRelaxed is the relaxation predicate an ordinary caller passes: a literal
// FALSE, so the clause it fronts decides the row exactly as it did before the
// hole existed.
//
// A named constant rather than `"FALSE"` typed at each call site, because a
// caller that reached for `"false"` or `"0"` would compile and would still be
// wrong — and what a reader of that call site needs is the WORD for what it
// means, not the value.
const neverRelaxed = "FALSE"

// liveRecord renders one of the predicates above under a caller's alias.
func liveRecord(predicate, alias string) string {
	return fmt.Sprintf(predicate, alias)
}

// Two of the holes are RELAXATIONS, and both default to off.
//
// %[12]s and %[13]s widen the not_sales judgement and the sales-link
// requirement respectively, each by OR-ing a caller-supplied predicate in front
// of the clause rather than by removing it. Every ordinary caller passes
// `neverRelaxed`, so the statement they run is the statement that was always
// here — the clause is unreachable and Postgres plans it away.
//
// They exist for the hidden-backlog guardrail (hiddenbacklog.go), which asks
// what each hiding rule is keeping off the queue. That question can only be
// answered by the query that owns the OTHER rules: a second statement restating
// the anti-joins, the machine-sender exclusion and the live-record predicates
// would be a second answer to "is this person waiting", and the two would
// disagree the first time either was edited. Widening one clause of the real
// query is the version that cannot drift.
//
// waitingRepliesSQL is Sprintf'd directly at ALL call sites — WaitingReplies
// below (entityClause scopeUnbounded, the workspace-wide Worklist read), the
// entity-scoped list filter (waitingReplyExistsClause) and the guardrail —
// rather than through a wrapper. Thirteen positional holes is already the shape the
// constant settled on for its own eligibility rules; a wrapper over that
// many arguments would just be the same Sprintf call once removed, with a
// second place to keep its parameter order in sync with the %[N] indices
// below. What must not fork between the call sites is the SQL TEXT — the
// anti-joins, the tie break, the future-dated guard, the horizon, the
// live-record predicates — and sharing the one constant holds that; a test
// feeding both callers the same timeline and requiring the same answer holds
// the rest.

// WaitingReplies answers who is waiting on this reader for a reply.
//
// One row per thread — the newest inbound in it — because a customer who wrote
// three times is waiting once, and three rows would read as three obligations.
// Oldest first: the longest wait is the one most likely to have been forgotten.
func (s *Store) WaitingReplies(ctx context.Context, asOf time.Time) ([]WaitingReply, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return nil, err
	}
	var waiting []WaitingReply
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		args := []any{}
		arg := func(v any) int { args = append(args, v); return len(args) }
		instant := arg(asOf)
		// The CONTENT gate, not the discover one. Everything this read answers
		// — who wrote last, that nobody replied, how long they have waited — is
		// derived from thread membership, and inheritedscope.go states the rule
		// plainly: a reader that shows anything derived from a thread composes
		// ActivityContentClause. Discover admits the safe markers only, and a
		// caller that picks it for content is the defect restrictedreaders_test
		// exists to catch.
		//
		// So a message this reader may not read produces no row at all. The
		// earlier cut kept the row and withheld only its subject, which still
		// published the wait, the timing and the linked record — and let a
		// reader watch a row vanish to learn that a reply they may not see had
		// arrived.
		content, err := auth.ActivityContentClause(ctx, "a", arg)
		if err != nil {
			return err
		}
		// The links come back only where the reader may see what they point at.
		// One visible person must not expose a colleague's deal, which is the
		// disclosure the timeline's own link read guards against.
		//
		// Aliased `wl`, not `l`: the discover gate composed above renders its
		// OWN correlated subquery over activity_link using `l`, and a second
		// `l` in this query's FROM shadows it — the gate's subquery then reads
		// our joined row instead of the activity's own links, and admits or
		// refuses on the wrong evidence.
		linkVisible, err := auth.LinkTargetVisibleClause(ctx, "wl", arg)
		if err != nil {
			return err
		}
		if linkVisible == "" {
			linkVisible = scopeUnbounded
		}
		// WHOSE set-asides apply. The reader comes from the principal rather
		// than from a parameter, so one person's snooze cannot be asked for on
		// another's behalf. A caller with no person behind it — a system pass
		// reading the same query — matches no reader_state row and therefore
		// has nothing hidden from it, which is the honest answer: a background
		// job has set nothing aside.
		reader := arg(readerOrNobody(ctx))
		rows, err := tx.Query(ctx,
			fmt.Sprintf(waitingRepliesSQL, instant, content, linkVisible, WaitingScanCap,
				waitingHorizonDays,
				liveRecord(openDealPredicate, "d"),
				liveRecord(workingLeadPredicate, "ld"),
				liveRecord(openDealPredicate, "openDeal"),
				liveRecord(openDealPredicate, "fd"),
				reader,
				scopeUnbounded,
				neverRelaxed, neverRelaxed), args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		waiting = []WaitingReply{}
		for rows.Next() {
			var row WaitingReply
			if err := rows.Scan(&row.ActivityID, &row.Subject, &row.Sender, &row.OccurredAt,
				&row.PersonID, &row.OrganizationID, &row.DealID,
				&row.HasOpenDeal, &row.OwnerID); err != nil {
				return err
			}
			waiting = append(waiting, row)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("activities: reading who is waiting for a reply: %w", err)
	}
	return waiting, nil
}
