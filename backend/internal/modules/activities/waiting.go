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
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
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
}

// waitingScanCap bounds the work one read does. Beyond this the answer is
// "there are more", never a silent truncation reported as a total.
const waitingScanCap = 200

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

// liveRecord renders one of the predicates above under a caller's alias.
func liveRecord(predicate, alias string) string {
	return fmt.Sprintf(predicate, alias)
}

// waitingRepliesSQL finds, per thread, the newest inbound with no later
// outbound in the same thread — where the thread is a SALES conversation the
// workspace is answerable for, recent enough to still be one.
//
// Every eligibility rule is applied before ORDER BY and LIMIT, so the cap falls
// on qualified rows only. A rule applied after the cap reads as a working
// filter and fails as a silent one: the scan fills with rows the rule would
// have removed, the customer behind them never arrives, and the page reports an
// empty queue with nothing to say it was truncated.
//
// NOT EXISTS rather than a window function or a join: it expresses the question
// directly — "nobody wrote back after this" — and it stops at the first later
// outbound rather than materializing every thread's history to sort it.
//
// The outbound side deliberately ignores the audience arm. A reply this reader
// may not READ still answered the customer, and skipping it would report a
// message as unanswered because the answer was somebody else's to see — the
// worst failure available here, since it sends a rep to write a second reply.
//
// A thread is matched within ONE medium: same kind, same channel provider. A
// mail thread key comes from headers the sender controls, and channel keys
// share the flat namespace with them, so comparing keys alone lets a crafted
// References value silence an unrelated conversation. The capture side's own
// reply detector matches the same way.
//
// The anti-joins are bounded by the read instant too, so the answer is a
// snapshot: a message dated in the future — mail carries the sender's own Date
// header — cannot suppress a thread that is genuinely waiting now.
//
// Equal timestamps are broken by id, because second-precision mail makes ties
// ordinary and both halves of "newest inbound, no later outbound" would
// otherwise be wrong at once.
//
// A message with NO thread_key is excluded rather than matched loosely. SQL
// equality would never join two NULLs, and IS NOT DISTINCT FROM joins them ALL
// — so an unthreaded message would be silenced by any other unthreaded
// outbound in the workspace, and one unthreaded reply would hide every
// unthreaded question at once. Excluding them under-reports, which is the
// direction that costs a row rather than a customer.
const waitingRepliesSQL = `
	SELECT a.id, COALESCE(a.subject, ''),
	       COALESCE((array_agg(sender.address ORDER BY sender.address)
	                 FILTER (WHERE sender.address IS NOT NULL))[1], ''),
	       a.occurred_at,
	       -- One row per message however many records it is filed under. There
	       -- is no max(uuid) in Postgres, so the pick is the first by text
	       -- order: arbitrary but STABLE, which is what a card needs — the same
	       -- message must not point at the person on one read and the company
	       -- on the next.
	       COALESCE((array_agg(wl.person_id ORDER BY wl.person_id::text)
	                 FILTER (WHERE wl.person_id IS NOT NULL))[1],
	                '00000000-0000-0000-0000-000000000000'::uuid),
	       COALESCE((array_agg(wl.organization_id ORDER BY wl.organization_id::text)
	                 FILTER (WHERE wl.organization_id IS NOT NULL))[1],
	                '00000000-0000-0000-0000-000000000000'::uuid),
	       COALESCE((array_agg(wl.deal_id ORDER BY wl.deal_id::text)
	                 FILTER (WHERE wl.deal_id IS NOT NULL))[1],
	                '00000000-0000-0000-0000-000000000000'::uuid),
	       -- Whether an open deal this reader can SEE is on this thread, which
	       -- is what lets a long wait with money on it stay in the day.
	       --
	       -- Off the visibility-gated join, like every record id above it. Read
	       -- off an ungated one it would answer "a deal exists" rather than "you
	       -- can see a deal", and a reader would learn the first by watching a
	       -- row they can see decline to go stale.
	       bool_or(openDeal.id IS NOT NULL)
	  FROM activity a
	  LEFT JOIN activity_link wl ON wl.activity_id = a.id AND (%[3]s)
	  -- Who wrote. The sender participant is where capture records the address,
	  -- and it is the only evidence at this level that tells a person apart
	  -- from a notification service.
	  LEFT JOIN activity_participant sender
	         ON sender.activity_id = a.id AND sender.role = 'from'
	  LEFT JOIN deal openDeal ON openDeal.id = wl.deal_id
	                         AND %[8]s
	 WHERE a.kind IN ('email', 'message')
	   AND a.direction = 'inbound'
	   AND a.archived_at IS NULL
	   AND a.occurred_at <= $%[1]d
	   AND %[2]s
	   -- Entity narrowing goes HERE, before waitingScanCap's LIMIT below: a
	   -- record's own wait can sit outside the oldest waitingScanCap threads
	   -- workspace-wide, and narrowing after the cap would report nothing
	   -- waiting on the very record this asks about. "TRUE" for the
	   -- workspace-wide Worklist read.
	   AND (%[11]s)
	   AND a.thread_key IS NOT NULL
	   -- Old enough and it is history, not work — UNLESS an open deal is on it.
	   --
	   -- The horizon and the caller's staleness rule have to agree about money,
	   -- or the looser of the two is decoration. The caller keeps a long wait
	   -- that still has a deal behind it, on the ground that there the silence
	   -- IS the problem; a horizon that removed those rows first would make that
	   -- branch unreachable and the rep would never see the one case where a
	   -- half-year of quiet costs something.
	   --
	   -- Before the cap, like every other exclusion here.
	   AND (a.occurred_at >= $%[1]d - make_interval(days => %[5]d)
	     OR EXISTS (
	          SELECT 1 FROM activity_link funded
	          JOIN deal fd ON fd.id = funded.deal_id AND %[9]s
	           WHERE funded.activity_id = a.id))
	   -- A SALES link, or it is not this queue's business.
	   --
	   -- The rule that was missing: this read used to answer "somebody wrote and
	   -- nobody replied", which is true of a rep's dentist. Unanswered is a fact
	   -- about a mailbox; waiting is a fact about a customer, and only a link to
	   -- a record the workspace sells to tells the two apart.
	   --
	   -- Its own EXISTS rather than a predicate on the wl join above, because
	   -- that join is filtered by what the reader may SEE. Qualifying through it
	   -- would make eligibility depend on the reader, so the same message would
	   -- be work for one colleague and personal mail for another.
	   AND EXISTS (
	         SELECT 1 FROM activity_link sales
	          WHERE sales.activity_id = a.id
	            AND (sales.person_id IS NOT NULL
	              OR sales.organization_id IS NOT NULL
	              OR EXISTS (SELECT 1 FROM deal d
	                          WHERE d.id = sales.deal_id AND %[6]s)
	              OR EXISTS (SELECT 1 FROM lead ld
	                          WHERE ld.id = sales.lead_id AND %[7]s)))
	   -- The obvious machines, excluded BEFORE the cap. Filtering them after
	   -- LIMIT lets two hundred notification threads fill the scan and push a
	   -- real customer past it, and the page then says nobody is waiting —
	   -- which is the one answer this source must never get wrong.
	   --
	   -- Deliberately coarse: it removes what nothing could mistake for a
	   -- person, and the caller's own rule (capture's address list, which
	   -- knows the operator's allowlist) still runs over what survives.
	   AND NOT EXISTS (
	         SELECT 1 FROM activity_participant machine
	          WHERE machine.activity_id = a.id
	            AND machine.role = 'from'
	            AND (machine.address ILIKE '%%noreply%%'
	              OR machine.address ILIKE '%%no-reply%%'
	              OR machine.address ILIKE '%%do-not-reply%%'
	              OR machine.address ILIKE '%%donotreply%%'
	              OR machine.address ILIKE '%%notification%%'
	              OR machine.address ILIKE '%%mailer-daemon%%'))
	   AND NOT EXISTS (
	         SELECT 1 FROM activity later
	          WHERE later.thread_key = a.thread_key
	            AND later.kind = a.kind
	            AND later.channel_provider IS NOT DISTINCT FROM a.channel_provider
	            AND later.direction = 'outbound'
	            AND later.archived_at IS NULL
	            AND later.occurred_at <= $%[1]d
	            AND (later.occurred_at, later.id) > (a.occurred_at, a.id))
	   AND NOT EXISTS (
	         SELECT 1 FROM activity newer
	          WHERE newer.thread_key = a.thread_key
	            AND newer.kind = a.kind
	            AND newer.channel_provider IS NOT DISTINCT FROM a.channel_provider
	            AND newer.direction = 'inbound'
	            AND newer.archived_at IS NULL
	            AND newer.occurred_at <= $%[1]d
	            AND (newer.occurred_at, newer.id) > (a.occurred_at, a.id))
	   -- Judged NOT a sales conversation, by anybody. A property of the THREAD,
	   -- so it holds for every reader AND for every later reply: one rep
	   -- recognizing the procurement newsletter settles what the conversation
	   -- is, and the next issue of it must not arrive as fresh work.
	   --
	   -- Matched on the same triple the reply anti-joins below use. Keying the
	   -- judgement on one activity id instead let the next inbound revive the
	   -- thread, because that message is a different row.
	   --
	   -- Before the cap, like every rule above it.
	   AND NOT EXISTS (
	         SELECT 1 FROM activity_sales_state judged
	          WHERE judged.thread_key = a.thread_key
	            AND judged.kind = a.kind
	            AND judged.channel_provider = coalesce(a.channel_provider, ''))
	   -- Set aside by THIS reader, and only this reader.
	   --
	   -- Judged against the row's CURRENT state rather than against what it was
	   -- at asOf: there is no set_at comparison here, so a judgement made after
	   -- the instant this page was read at still hides its row. In production
	   -- asOf is now() at the top of the same assembly, so the window is
	   -- milliseconds wide and hiding a message somebody just set aside is the
	   -- answer a reader wants. A caller replaying a HISTORICAL instant would
	   -- get today's judgements over that day's messages, and nothing does.
	   --
	   -- A snooze lifts on its own moment, so the row comes back when it is due
	   -- rather than waiting for somebody to remember it.
	   --
	   -- not_mine carries no moment and does not lift at all. Ending it when the
	   -- linked record changes hands would be the kinder rule, and it is not
	   -- implemented: a message reaches its owner through a person, an
	   -- organization, a deal or a lead, so the re-arm is a consumer over four
	   -- ownership events rather than a clause here. Until that exists the
	   -- judgement stands until its reader withdraws it, and the contract says
	   -- so rather than promising the re-arm.
	   AND NOT EXISTS (
	         SELECT 1 FROM activity_reader_state mine
	          WHERE mine.activity_id = a.id
	            AND mine.reader_id = $%[10]d
	            AND (mine.state = 'not_mine'
	              OR (mine.state = 'snoozed' AND mine.snoozed_until > $%[1]d)))
	 GROUP BY a.id, a.subject, a.occurred_at
	 -- NEWEST first, which is the opposite of how the rows are then shown.
	 --
	 -- The cap has to spend its budget on the rows most likely to matter, and
	 -- those are the recent ones: a wait inside a day is urgent, and one past a
	 -- fortnight without an open deal is demoted by the caller the moment it
	 -- arrives. Taking the OLDEST two hundred spent the entire scan on rows
	 -- headed for the bottom of the page and cut the urgent ones before anybody
	 -- saw them — the queue would report nobody waiting on the day it was most
	 -- wrong.
	 --
	 -- The caller sorts oldest-first for display, so what a reader sees is
	 -- unchanged. This decides only WHICH waits survive the bound.
	 ORDER BY a.occurred_at DESC
	 LIMIT %[4]d`

// waitingRepliesSQL is Sprintf'd directly at BOTH call sites — WaitingReplies
// below (entityClause scopeUnbounded, the workspace-wide Worklist read) and
// the entity-scoped list filter (waitingReplyExistsClause) — rather than
// through a wrapper. Eleven positional holes is already the shape the
// constant settled on for its own eligibility rules; a wrapper over that
// many arguments would just be the same Sprintf call once removed, with a
// second place to keep its parameter order in sync with the %[N] indices
// below. What must not fork between the two call sites is the SQL TEXT — the
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
			fmt.Sprintf(waitingRepliesSQL, instant, content, linkVisible, waitingScanCap,
				waitingHorizonDays,
				liveRecord(openDealPredicate, "d"),
				liveRecord(workingLeadPredicate, "ld"),
				liveRecord(openDealPredicate, "openDeal"),
				liveRecord(openDealPredicate, "fd"),
				reader,
				scopeUnbounded), args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		waiting = []WaitingReply{}
		for rows.Next() {
			var row WaitingReply
			if err := rows.Scan(&row.ActivityID, &row.Subject, &row.Sender, &row.OccurredAt,
				&row.PersonID, &row.OrganizationID, &row.DealID,
				&row.HasOpenDeal); err != nil {
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

// waitingReplyEntityClause narrows the thread walk to one record, in the SAME
// vocabulary linktarget.go and listActivitiesFilter's own entity_type/id
// filter use — a record type added to linkColumn or the organization arm
// reaches this walk too, rather than a second copy silently missing it.
func waitingReplyEntityClause(entityType string, entityID ids.UUID, arg func(any) int) (string, error) {
	if entityType == string(datasource.RecordOrganization) {
		// An account's timeline is wider than its direct links (mail is filed
		// against the person it was with), so this reuses the SAME three-arm
		// walk the timeline list and the company view both read through —
		// see OrgLinkedActivityExists.
		return OrgLinkedActivityExists(arg(entityID)), nil
	}
	column := linkColumn(entityType)
	if column == "" {
		return "", &InvalidLinkTypeError{EntityType: entityType}
	}
	typePos := arg(entityType)
	idPos := arg(entityID)
	return sprintf("EXISTS (SELECT 1 FROM activity_link el WHERE el.activity_id = a.id AND el.entity_type = $%d AND el.%s = $%d)",
		typePos, column, idPos), nil
}

// waitingReplyExistsClause builds the timeline list's `waiting_reply=true`
// filter: the SAME thread walk WaitingReplies runs for the Worklist,
// embedded as a subquery so the outer list's own entity/kind/cursor terms
// compose with it rather than duplicating what "unanswered" means.
//
// The subquery is uncorrelated — it computes its own candidate set rather
// than reading the outer FROM — so its own `a` alias shadowing the outer
// query's is harmless.
func waitingReplyExistsClause(ctx context.Context, arg func(any) int, asOf time.Time, entityType *string, entityID *ids.UUID) (string, error) {
	instant := arg(asOf)
	content, err := auth.ActivityContentClause(ctx, "a", arg)
	if err != nil {
		return "", err
	}
	linkVisible, err := auth.LinkTargetVisibleClause(ctx, "wl", arg)
	if err != nil {
		return "", err
	}
	if linkVisible == "" {
		linkVisible = scopeUnbounded
	}
	entityClause := scopeUnbounded
	if entityType != nil && entityID != nil {
		entityClause, err = waitingReplyEntityClause(*entityType, *entityID, arg)
		if err != nil {
			return "", err
		}
	}
	// The SAME reader, horizon and live-record rules WaitingReplies applies
	// for the Worklist: a thread the Worklist would not name as waiting must
	// not be named by a record page either, and a per-record set-aside must
	// still be this reader's own.
	reader := arg(readerOrNobody(ctx))
	return "a.id IN (SELECT id FROM (" +
		fmt.Sprintf(waitingRepliesSQL, instant, content, linkVisible, waitingScanCap,
			waitingHorizonDays,
			liveRecord(openDealPredicate, "d"),
			liveRecord(workingLeadPredicate, "ld"),
			liveRecord(openDealPredicate, "openDeal"),
			liveRecord(openDealPredicate, "fd"),
			reader,
			entityClause) +
		") waiting_thread)", nil
}

// appendWaitingReplyClause is listActivitiesFilter's `waiting_reply=true`
// term, split out so that already-long function does not have to grow to
// hold it. A no-op when the caller did not ask for the filter.
func appendWaitingReplyClause(ctx context.Context, in ListActivitiesInput, arg func(any) int, where []string) ([]string, error) {
	if in.WaitingReplyAsOf == nil {
		return where, nil
	}
	clause, err := waitingReplyExistsClause(ctx, arg, *in.WaitingReplyAsOf, in.EntityType, in.EntityID)
	if err != nil {
		return nil, err
	}
	return append(where, clause), nil
}
