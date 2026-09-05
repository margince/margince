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
	// Kind tells an email from a channel message. The query spans both, and
	// only an email has an email's shape: a caller drawing the canonical email
	// row for a chat would put a mail icon on a message that never travelled
	// on one.
	Kind    string
	Subject string
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
	// OwedVerdict is the classifier's judgement of whether this message asks
	// its recipient side for something: OwedVerdictAsksUs, OwedVerdictInformsUs
	// or empty for unjudged.
	//
	// Empty is not a third verdict. An unjudged message ranks exactly as it did
	// before the column existed, because a classifier that has not run, has run
	// out of budget or answered below its confidence floor must not change what
	// a rep sees.
	OwedVerdict string
	// Engaged reports that this workspace wrote on this thread BEFORE the
	// message arrived — the evidence that a conversation is one we are already
	// in, rather than one that merely reached a mailbox.
	//
	// Reported, never used to hide. Thread identity comes from the reply
	// headers, and a client that strips them gives every message its own
	// thread: an exclusion keyed on this would drop a live customer silently,
	// which is the one failure this queue must not have. A caller demotes an
	// unengaged wait instead, so being wrong costs a scroll.
	Engaged bool
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
// rather than through a wrapper. A long positional argument list is already the
// shape the constant settled on for its own eligibility rules; a wrapper over
// that many arguments would just be the same Sprintf call once removed, with a
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
		// The same gate for the reply that may LIFT a snooze. A reply this
		// reader cannot see must not put the row back on their day: the row
		// reappearing is itself the disclosure that it arrived.
		backContent, err := auth.ActivityContentClause(ctx, "back", arg)
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
		// One snapshot for this read. Passed as a list rather than tested in Go
		// so the clause runs before the scan cap — see OwnDomains.
		ownDomains, err := s.ownDomainList(ctx, tx)
		if err != nil {
			return err
		}
		// The horizon this installation's own answering implies, measured in
		// the same transaction as the scan it bounds — so the cutoff and the
		// rows it judges come from one snapshot.
		horizon, err := s.waitingHorizonFor(ctx, tx, asOf)
		if err != nil {
			return err
		}
		rows, err := tx.Query(ctx,
			fmt.Sprintf(waitingRepliesSQL, instant, content, linkVisible, WaitingScanCap,
				horizon,
				liveRecord(openDealPredicate, "d"),
				liveRecord(workingLeadPredicate, "ld"),
				liveRecord(openDealPredicate, "openDeal"),
				liveRecord(openDealPredicate, "fd"),
				reader,
				scopeUnbounded,
				neverRelaxed, neverRelaxed,
				neverRelaxed, ownDomainSenderSQL("a", arg(ownDomains)),
				messageSnoozeLiftedSQL(fmt.Sprintf("$%d", instant), backContent)), args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		waiting = []WaitingReply{}
		for rows.Next() {
			var row WaitingReply
			if err := rows.Scan(&row.ActivityID, &row.Kind, &row.Subject, &row.Sender, &row.OccurredAt,
				&row.PersonID, &row.OrganizationID, &row.DealID,
				&row.HasOpenDeal, &row.OwedVerdict, &row.Engaged, &row.OwnerID); err != nil {
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

// OwnDomains reports the email domains this installation's own people write
// from — the set a message's sender is tested against to tell a colleague from
// a customer.
//
// A seam rather than a query here because the domains are capture's to define:
// it owns workspace_email_domain and the rule for which entries count as
// vouched-for, and a module may not read a sibling's tables. What this returns
// is DATA the queue tests against in SQL, not a Go predicate, because the test
// has to run before the scan cap — a predicate applied to the rows that came
// back would let two hundred colleague threads fill the scan and push a real
// customer past it, which is the failure every other rule in waitingsql.go is
// ordered to avoid.
//
// The domains are read inside the CALLER's transaction, so the strict read and
// every relaxed read beside it see one snapshot. A seam that opened its own
// would let the set change between two counts that are meant to differ by
// exactly one rule.
type OwnDomains interface {
	Domains(ctx context.Context, tx pgx.Tx) ([]string, error)
}

// WithOwnDomains wires the colleague-domain reader the waiting queue needs.
//
// Unbound, the queue admits every sender: a deployment that cannot say which
// domains are its own must not guess, and the honest failure is a queue with
// colleagues in it rather than one that silently drops a customer whose domain
// resembles ours.
func (s *Store) WithOwnDomains(own OwnDomains) *Store {
	s.ownDomains = own
	return s
}

// WithOwnDomains on Handlers wires the same reader the store needs, so the
// timeline's `waiting_reply=true` filter answers with the queue's rule rather
// than a looser one.
func (h Handlers) WithOwnDomains(own OwnDomains) Handlers {
	h.store = h.store.WithOwnDomains(own)
	return h
}

// ownDomainList reads the colleague domains for one query, or none when no
// reader is wired.
func (s *Store) ownDomainList(ctx context.Context, tx pgx.Tx) ([]string, error) {
	if s.ownDomains == nil {
		return nil, nil
	}
	domains, err := s.ownDomains.Domains(ctx, tx)
	if err != nil {
		return nil, fmt.Errorf("activities: reading which domains are our own: %w", err)
	}
	return domains, nil
}

// ownDomainSenderSQL is the ONE spelling of "this message came from one of our
// own domains", rendered under a caller's activity alias with the placeholder
// holding the domain list.
//
// Held by: TestTheOwnDomainSenderPredicateHasOneSpelling
// (backend/gates/owndomainpredicate_test.go)
//
// A shared fragment rather than the same predicate typed twice, because the
// waiting queue and the response reading judge the same messages: a rule that
// drifted between them would make /worklist/response disagree with the queue
// about who is waiting, and nothing would fail.
//
// Two things it does NOT do, each learned from a way it can suppress a real
// customer silently:
//
//   - It reads the domain after the LAST at-sign, not the first. A quoted local
//     part may legally contain one, so a sender can put one of our domains
//     inside their own local part; splitting at the first would read that as
//     the domain and hide their message.
//   - It compares a suffix with right(), never LIKE. A domain is text an
//     operator typed, and underscore is a LIKE wildcard, so an entry such as
//     "our_.test" would match "ourx.test" and hide that customer's mail.
//
// A blank entry matches nothing on its own: equality fails against any real
// domain, and the suffix comparison then asks whether a domain ends in a bare
// dot, which none does. Stated here rather than guarded, because a guard no
// test can fail is a claim nobody is holding.
func ownDomainSenderSQL(alias string, domainArg int) string {
	return fmt.Sprintf(`EXISTS (
	         SELECT 1 FROM activity_participant ours
	          CROSS JOIN LATERAL (SELECT lower(split_part(ours.address, '@',
	                  length(ours.address) - length(replace(ours.address, '@', '')) + 1))
	              ) AS sender(domain)
	          WHERE ours.activity_id = %[1]s.id
	            AND ours.role = 'from'
	            AND sender.domain <> ''
	            AND EXISTS (
	                  SELECT 1 FROM unnest($%[2]d::text[]) AS own(domain)
	                   WHERE (sender.domain = own.domain
	                       OR right(sender.domain, length(own.domain) + 1)
	                          = '.' || own.domain)))`, alias, domainArg)
}
