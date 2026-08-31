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
	// OwnerID is who answers for the linked record, by the precedence the query
	// declares. Zero means nobody does — which routes the wait to the unassigned
	// queue rather than to whoever happens to be reading.
	OwnerID ids.UUID
	// HasOpenDeal reports whether an open deal is on this thread. It is what
	// lets a caller keep an old wait that still has money behind it, and drop
	// one that does not.
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
// Applied BEFORE the cap for the same reason the machine rule is, and the
// reason is worth restating because it is the whole shape of this query: a
// filter after LIMIT lets two hundred rows nobody wants fill the scan and push
// a real customer past it, and the page then says nobody is waiting.
const waitingHorizonDays = 90

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
	       -- Who answers for this wait, by ONE declared precedence:
	       -- deal, then lead, then person, then organization.
	       --
	       -- Nearest-to-the-money first. A thread filed under both a deal and
	       -- the person on it belongs to whoever owns the deal, because that is
	       -- who the reply changes an outcome for. Without a stated order the
	       -- answer would follow whichever link happened to sort first, and one
	       -- message would change hands between reads.
	       --
	       -- Absent is a real answer, not a missing one: nobody owns it, and the
	       -- caller routes it to the unassigned queue rather than to everyone.
	       COALESCE(
	         (array_agg(dealOwner.owner_id ORDER BY dealOwner.id::text)
	          FILTER (WHERE dealOwner.owner_id IS NOT NULL))[1],
	         (array_agg(leadOwner.owner_id ORDER BY leadOwner.id::text)
	          FILTER (WHERE leadOwner.owner_id IS NOT NULL))[1],
	         (array_agg(personOwner.owner_id ORDER BY personOwner.id::text)
	          FILTER (WHERE personOwner.owner_id IS NOT NULL))[1],
	         (array_agg(orgOwner.owner_id ORDER BY orgOwner.id::text)
	          FILTER (WHERE orgOwner.owner_id IS NOT NULL))[1],
	         '00000000-0000-0000-0000-000000000000'::uuid),
	       -- Whether an OPEN deal is on this thread, which is what lets the
	       -- caller keep an old wait that still has money on it.
	       bool_or(dealOwner.id IS NOT NULL)
	  FROM activity a
	  LEFT JOIN activity_link wl ON wl.activity_id = a.id AND (%[3]s)
	  -- Who wrote. The sender participant is where capture records the address,
	  -- and it is the only evidence at this level that tells a person apart
	  -- from a notification service.
	  LEFT JOIN activity_participant sender
	         ON sender.activity_id = a.id AND sender.role = 'from'
	  -- The owner joins walk the links WITHOUT the visibility clause the wl
	  -- join carries. Who answers for a record is a fact about the record; if it
	  -- were read through what this reader may see, a message would look
	  -- unowned to one colleague and owned to another, and "mine" would mean a
	  -- different set of rows per reader for the same underlying truth.
	  --
	  -- Only the id and the owner leave these joins. No name, no amount, no
	  -- subject — nothing a reader could not otherwise reach — so this widens
	  -- what the query KNOWS without widening what it discloses.
	  LEFT JOIN activity_link ownerLink ON ownerLink.activity_id = a.id
	  LEFT JOIN deal dealOwner ON dealOwner.id = ownerLink.deal_id
	                          AND dealOwner.status = 'open'
	                          AND dealOwner.archived_at IS NULL
	  LEFT JOIN lead leadOwner ON leadOwner.id = ownerLink.lead_id
	                          AND leadOwner.status IN ('new', 'contacted', 'engaged')
	                          AND leadOwner.archived_at IS NULL
	  LEFT JOIN person personOwner ON personOwner.id = ownerLink.person_id
	                              AND personOwner.archived_at IS NULL
	  LEFT JOIN organization orgOwner ON orgOwner.id = ownerLink.organization_id
	                                 AND orgOwner.archived_at IS NULL
	 WHERE a.kind IN ('email', 'message')
	   AND a.direction = 'inbound'
	   AND a.archived_at IS NULL
	   AND a.occurred_at <= $%[1]d
	   AND %[2]s
	   AND a.thread_key IS NOT NULL
	   -- Old enough and it is history, not work. Before the cap, like every
	   -- other exclusion here.
	   AND a.occurred_at >= $%[1]d - make_interval(days => %[5]d)
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
	                          WHERE d.id = sales.deal_id
	                            AND d.status = 'open'
	                            AND d.archived_at IS NULL)
	              OR EXISTS (SELECT 1 FROM lead ld
	                          WHERE ld.id = sales.lead_id
	                            AND ld.status IN ('new', 'contacted', 'engaged')
	                            AND ld.archived_at IS NULL)))
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
	 GROUP BY a.id, a.subject, a.occurred_at
	 ORDER BY a.occurred_at ASC
	 LIMIT %[4]d`

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
		rows, err := tx.Query(ctx,
			fmt.Sprintf(waitingRepliesSQL, instant, content, linkVisible, waitingScanCap,
				waitingHorizonDays), args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		waiting = []WaitingReply{}
		for rows.Next() {
			var row WaitingReply
			if err := rows.Scan(&row.ActivityID, &row.Subject, &row.Sender, &row.OccurredAt,
				&row.PersonID, &row.OrganizationID, &row.DealID,
				&row.OwnerID, &row.HasOpenDeal); err != nil {
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
