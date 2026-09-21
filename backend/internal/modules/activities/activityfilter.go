// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// The timeline query's own WHERE: what one list input narrows to, the gate that
// bounds which rows a caller reaches, and the keyset that continues a page.
//
// Split from companyscope.go, which is the account WALK — how an activity
// reaches a company through its links, its deal and its participants' employers.
// The walk is one arm this builder composes; keeping the two in one file made a
// reader of either scroll past the other.

import (
	"context"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// entityLinkFilter narrows the timeline to one record, in the SAME vocabulary
// the write uses. A second list here drifted from linkTargets and silently
// dropped two kinds: an activity could be linked to a lead or a project and
// then be unfindable by filtering on the very link that was just written.
func entityLinkFilter(in ListActivitiesInput, arg func(any) int) (where []string, err error) {
	column := linkColumn(*in.EntityType)
	if column == "" {
		return nil, &InvalidLinkTypeError{EntityType: *in.EntityType}
	}
	if *in.EntityType == string(datasource.RecordCompany) {
		// An account's timeline is wider than its direct links: mail is
		// filed against the CONTACT it was with, so a flat company_id
		// match hides every message the company actually exchanged.
		// CompanyLinkedActivityExists is the walk the company view's other
		// readers already use. EXISTS rather than a join, so an activity
		// reachable through two links stays one row and the keyset cursor
		// keeps ordering over a stable set.
		return []string{CompanyLinkedActivityExists(arg(*in.EntityID))}, nil
	}
	// EXISTS rather than a join, for the two reasons the company arm above
	// gives and one more: an activity linked twice to the same record stays one
	// row, and the keyset tuple — which names `created_at` and `id` alone,
	// since every other list it serves reads one table — is ambiguous the
	// moment activity_link, which carries both, is in the FROM list.
	return []string{sprintf(
		`EXISTS (SELECT 1 FROM activity_link al
		          WHERE al.activity_id = a.id AND al.entity_type = $%d AND al.%s = $%d)`,
		arg(*in.EntityType), column, arg(*in.EntityID))}, nil
}

// listActivitiesFilter builds the timeline query's join, WHERE terms and
// bind arguments from one list input, plus the per-row audience test the
// SELECT projects as content_state.
func listActivitiesFilter(ctx context.Context, in ListActivitiesInput) (
	where []string, content string, sorted *storekit.ListSort, args []any, err error,
) {
	where = []string{"1=1"}
	args = []any{}
	arg := func(v any) int { args = append(args, v); return len(args) }

	// The sort FIRST: it may bind parameters of its own and the cursor clause
	// below is rendered from it, so both count through one counter.
	sorted, err = timelineSort(ctx, in, arg)
	if err != nil {
		return nil, "", nil, nil, err
	}

	// The timeline is DISCOVER-gated: a row reachable through a record the
	// caller may read is listed, and whether its content comes with it is the
	// audience's call, answered per row (content_state). The free text of a
	// limited conversation is blanked by the scan, never selected into the
	// response.
	//
	// Composed HERE rather than in a helper of its own, and the reason is a
	// gate rather than taste: restrictedreaders_test.go reads the declaration
	// that builds the statement and the names it calls, so a gate one hop
	// further away is a gate that census cannot see — and a reader it cannot
	// see reads as one that excludes no held row.
	gate := auth.ActivityDiscoverClause
	if filtersOnContent(in) {
		// A filter over the subject, the body or the thread key is a READ of
		// them: a withheld row that matched would tell the caller what it says
		// through has_more and the page boundary. Such a list is content-gated,
		// so a limited row is simply not there.
		gate = auth.ActivityContentClause
	}
	scope, err := gate(ctx, "a", arg)
	if err != nil {
		return nil, "", nil, nil, err
	}
	if scope != "" {
		where = append(where, scope)
	}
	content, err = auth.ActivityAudienceArm(ctx, "a", arg)
	if err != nil {
		return nil, "", nil, nil, err
	}
	where = append(where, timelineNarrowings(in, arg)...)
	if in.EntityType != nil && in.EntityID != nil {
		entityWhere, entityErr := entityLinkFilter(in, arg)
		if entityErr != nil {
			return nil, "", nil, nil, entityErr
		}
		where = append(where, entityWhere...)
	}
	if in.WithinProjectID != nil {
		where = append(where, ActivityWithinProject(arg(*in.WithinProjectID)))
	}
	if where, err = appendWaitingReplyClause(ctx, in, arg, where); err != nil {
		return nil, "", nil, nil, err
	}
	if where, err = appendRequestReviewClause(ctx, in, arg, where); err != nil {
		return nil, "", nil, nil, err
	}
	where = append(where, activityRowClauses(in, arg)...)
	keyset, err := timelineKeyset(in, sorted, arg)
	if err != nil {
		return nil, "", nil, nil, err
	}
	if keyset != "" {
		where = append(where, keyset)
	}
	return where, content, sorted, args, nil
}

// timelineNarrowings are the filters that read the activity row itself: its
// lifecycle, its kind and transport, and the four queue shapes a member asks
// for their own work by.
//
// Together rather than inline, because each is one clause with no bearing on
// the next: the builder above composes the row scope, the record narrowing and
// the page, and reading it should not mean reading past nine of these first.
func timelineNarrowings(in ListActivitiesInput, arg func(any) int) []string {
	var where []string
	if !in.IncludeArchived {
		where = append(where, activityLive)
	}
	if in.Kind != nil {
		where = append(where, sprintf("a.kind = $%d", arg(*in.Kind)))
	}
	if in.ChannelProvider != nil {
		where = append(where, sprintf("a.channel_provider = $%d", arg(*in.ChannelProvider)))
	}
	if clause := ownQueueClause(in.OwnQueueOf, arg); clause != "" {
		where = append(where, clause)
	}
	if in.UnassignedQueue {
		where = append(where, unassignedQueueClause())
	}
	if clause := onMeetingOfClause(in.OnMeetingOf, arg); clause != "" {
		where = append(where, clause)
	}
	if in.MeetingHost != nil {
		where = append(where, sprintf("a.kind = 'meeting' AND a.host_user_id = $%d", arg(*in.MeetingHost)))
	}
	if in.UnhostedMeetings {
		where = append(where, "a.kind = 'meeting' AND a.host_user_id IS NULL")
	}
	if clause := openTaskAssigneeClause(in.AssigneeID, arg); clause != "" {
		where = append(where, clause)
	}
	return append(where, openTaskWindowClauses(in, arg)...)
}

// timelineKeyset continues a page, or answers empty for a first one.
//
// Through the sort's own keyset, which refuses a token minted under a different
// ordering rather than resuming on an axis this page is not ordered by — the
// guard the open-and-due read used to need a sentinel of its own for. The
// request queue keeps one, because its order is hand-built and mints no token
// to check.
func timelineKeyset(in ListActivitiesInput, sorted *storekit.ListSort, arg func(any) int) (string, error) {
	if in.Cursor == nil || *in.Cursor == "" {
		return "", nil
	}
	if in.RequestReviewAsOf != nil {
		return "", errRequestReviewWithCursor
	}
	return sorted.KeysetClause(*in.Cursor, arg)
}

// activityRowClauses narrows on columns of the activity row itself: the
// thread it belongs to, the words in it, when it happened, whether its
// outcome is still open.
//
// None of them consults the caller's scope, so none can fail — which row is
// in reach was already decided by the gate its caller picked, and these only
// shrink that set further.
func activityRowClauses(in ListActivitiesInput, arg func(any) int) []string {
	var where []string
	if in.ThreadKey != nil && *in.ThreadKey != "" {
		where = append(where, sprintf("a.thread_key = $%d", arg(*in.ThreadKey)))
	}
	if in.Query != nil && *in.Query != "" {
		// subject + body are the two human-readable columns a human would
		// recognize an item by. The wildcard is escaped, so a caller typing %
		// searches for a percent sign rather than matching everything.
		pos := arg("%" + storekit.EscapeLike(*in.Query) + "%")
		where = append(where, sprintf("(a.subject ILIKE $%d ESCAPE '\\' OR a.body ILIKE $%d ESCAPE '\\')", pos, pos))
	}
	if in.OccurredAfter != nil {
		where = append(where, sprintf("a.occurred_at >= $%d", arg(*in.OccurredAfter)))
	}
	if in.OccurredBefore != nil {
		where = append(where, sprintf("a.occurred_at < $%d", arg(*in.OccurredBefore)))
	}
	if in.AwaitingOutcome {
		// NULL counts as awaiting: a captured calendar event carries no status,
		// and excluding it would empty this question on a connected calendar.
		where = append(where, "(a.meeting_status IS NULL OR a.meeting_status = 'booked')")
	}
	return where
}

// filtersOnContent reports whether the list narrows on fields a withheld row
// does not disclose.
//
// waiting_reply belongs here for the reason waiting.go's own gate comment
// states: who wrote last and that nobody replied are both derived from
// thread membership, not from the safe discover markers, so a withheld
// message must drop out of the candidate set rather than surface as a row
// with the answer blanked.
func filtersOnContent(in ListActivitiesInput) bool {
	return (in.ThreadKey != nil && *in.ThreadKey != "") || (in.Query != nil && *in.Query != "") ||
		in.WaitingReplyAsOf != nil || in.RequestReviewAsOf != nil || in.ReadableOnly
}
