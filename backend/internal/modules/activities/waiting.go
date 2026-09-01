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
	// OccurredAt is when they wrote, which is what the wait is measured from.
	OccurredAt time.Time
	// The record the thread is filed under, when it names one.
	PersonID       ids.UUID
	OrganizationID ids.UUID
	DealID         ids.UUID
}

// waitingScanCap bounds the work one read does. Beyond this the answer is
// "there are more", never a silent truncation reported as a total.
const waitingScanCap = 200

// waitingRepliesSQL finds, per thread, the newest inbound with no later
// outbound in the same thread.
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
	SELECT a.id, COALESCE(a.subject, ''), a.occurred_at,
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
	                '00000000-0000-0000-0000-000000000000'::uuid)
	  FROM activity a
	  LEFT JOIN activity_link wl ON wl.activity_id = a.id AND (%[3]s)
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
	   AND (%[5]s)
	   AND a.thread_key IS NOT NULL
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

// waitingRepliesQueryText fills waitingRepliesSQL's five holes. The one
// spelling backs both callers: WaitingReplies (entityClause scopeUnbounded,
// the workspace-wide Worklist read) and the entity-scoped list filter
// (waitingReplyExistsClause), so the thread walk — the anti-joins, the tie
// break, the future-dated guard — is never answered twice.
func waitingRepliesQueryText(instant int, content, linkVisible, entityClause string, scanCap int) string {
	return fmt.Sprintf(waitingRepliesSQL, instant, content, linkVisible, scanCap, entityClause)
}

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
			waitingRepliesQueryText(instant, content, linkVisible, scopeUnbounded, waitingScanCap), args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		waiting = []WaitingReply{}
		for rows.Next() {
			var row WaitingReply
			if err := rows.Scan(&row.ActivityID, &row.Subject, &row.OccurredAt,
				&row.PersonID, &row.OrganizationID, &row.DealID); err != nil {
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
	return "a.id IN (SELECT id FROM (" +
		waitingRepliesQueryText(instant, content, linkVisible, entityClause, waitingScanCap) +
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
