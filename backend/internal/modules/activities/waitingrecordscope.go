// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// Narrowing the waiting-reply walk to ONE record.
//
// The workspace-wide read is the Worklist's; this is what a record page asks,
// and the two share a statement rather than a resemblance. The narrowing goes
// inside that statement, before the scan cap's LIMIT: a record's own wait can
// sit outside the oldest WaitingScanCap threads workspace-wide, and narrowing
// after the cap would report nothing waiting on the very record being asked
// about.

import (
	"context"
	"fmt"
	"time"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

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
func waitingReplyExistsClause(ctx context.Context, arg func(any) int, asOf time.Time, entityType *string, entityID *ids.UUID, ownDomains []string, alsoBeforeTheCap string) (string, error) {
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
	// A caller's own narrowing joins the entity clause INSIDE the statement,
	// which is the only place it can go.
	//
	// The template caps its scan at the newest WaitingScanCap threads, so a
	// predicate applied to what comes back selects from those rather than from
	// the queue: the classifier's backlog, filtering for unjudged rows outside
	// this, would see only the newest 200 and never reach an older message once
	// those were judged. Nothing would fail — the backlog would report itself
	// empty with work still in it. Same reason the entity clause is here and
	// not outside, spelled in the template beside the hole.
	if alsoBeforeTheCap != "" {
		entityClause = "(" + entityClause + ") AND (" + alsoBeforeTheCap + ")"
	}
	// The SAME reader, horizon and live-record rules WaitingReplies applies
	// for the Worklist: a thread the Worklist would not name as waiting must
	// not be named by a record page either, and a per-record set-aside must
	// still be this reader's own.
	reader := arg(readerOrNobody(ctx))
	return "a.id IN (SELECT id FROM (" +
		fmt.Sprintf(waitingRepliesSQL, instant, content, linkVisible, WaitingScanCap,
			waitingHorizonDays,
			liveRecord(openDealPredicate, "d"),
			liveRecord(workingLeadPredicate, "ld"),
			liveRecord(openDealPredicate, "openDeal"),
			liveRecord(openDealPredicate, "fd"),
			reader,
			entityClause,
			neverRelaxed, neverRelaxed,
			neverRelaxed, ownDomainSenderSQL("a", arg(ownDomains))) +
		") waiting_thread)", nil
}

// appendWaitingReplyClause is listActivitiesFilter's `waiting_reply=true`
// term, split out so that already-long function does not have to grow to
// hold it. A no-op when the caller did not ask for the filter.
func appendWaitingReplyClause(ctx context.Context, in ListActivitiesInput, arg func(any) int, where []string) ([]string, error) {
	if in.WaitingReplyAsOf == nil {
		return where, nil
	}
	clause, err := waitingReplyExistsClause(ctx, arg, *in.WaitingReplyAsOf, in.EntityType, in.EntityID, in.ownDomains, "")
	if err != nil {
		return nil, err
	}
	return append(where, clause), nil
}
