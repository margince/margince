// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package briefs

// Which activities a brief may rest on.
//
// A brief names activities in the evidence it cites and in the moment it dates
// a returning deal by, and an activity's audience decides whether that id and
// that moment are its reader's to see. The deal's row scope answers neither — a
// deal is workspace-readable, while an activity under it can be limited to its
// participants, and the product's own rule for a limited meeting is that the
// timestamp IS the disclosure.
//
// So the gathering of those ids and moments, the rule that lets an activity
// bring a dismissed deal back (the deal returns dated by it), and the serving of
// a stored item all compose briefActivityClause. A snooze lifted by a reply does
// not: it tells the rep only that the deal moved, not what moved it or when
// (briefsnoozelift.go says why).

import (
	"context"
	"errors"
	"fmt"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// briefActivityClause renders the predicate an activity row under alias must
// pass before a brief may count it, cite it, or date a line by it.
//
// BOTH halves, for the reason dealstatus.readableActivities gives: the object
// grant says whether this seat reads activities at all, and the content clause
// says which — its audience arm is what a discover-only clause would miss. A
// seat refused the grant gets FALSE rather than an error: its brief still ranks
// its deals, it simply rests on none of their activity.
func briefActivityClause(ctx context.Context, alias string, arg func(any) int) (string, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			return "FALSE", nil
		}
		return "", err
	}
	return auth.ActivityContentClause(ctx, alias, arg)
}

// servedActivityRefsSQL renders the two columns of a stored brief item (alias
// bi) that name activities, as the item is served to its reader.
//
// The run recorded both under the reader's access at the time, and an audience
// narrowed since then binds on the next read — the same way the deal's scope is
// re-applied rather than inherited from the snapshot. An activity archived since
// (an Art. 17 erasure archives its rows) is gone from both, as the run's own
// gathering never admitted one.
//
//   - evidence is the ids the run cited, less every activity the reader can no
//     longer read. The deal's own id stays: it is the evidence for winnability,
//     revenue and timing, and the item is served at all only once the deal
//     passed its scope. WITH ORDINALITY keeps the order the ranking cited.
//   - returnedWith is the moment a returning deal is dated by, served only while
//     a readable activity on that deal still occurred at it. The column keeps
//     the moment and not the activity, so this is the question it can answer:
//     is the date still one this reader could have read off the deal.
//
// The activity clause is rendered once, in the `served_activity` CTE that with
// names, and the caller puts with ahead of its SELECT. Spelled per column it was
// planned three times, and a connection keeps the planner's memory.
func servedActivityRefsSQL(ctx context.Context, arg func(any) int, items string) (with, evidence, returnedWith string, err error) {
	readable, err := briefActivityClause(ctx, "ev_a", arg)
	if err != nil {
		return "", "", "", err
	}
	with = fmt.Sprintf(`WITH served_activity AS MATERIALIZED (
		SELECT ev_a.id FROM activity ev_a
		 WHERE ev_a.archived_at IS NULL AND ev_a.id IN (
		        SELECT ref.id FROM brief_item ri, unnest(ri.evidence_ids) AS ref(id) WHERE %[1]s
		        UNION
		        SELECT rl.activity_id FROM brief_item ri
		          JOIN activity_link rl ON rl.deal_id = ri.deal_id
		          JOIN activity ra ON ra.id = rl.activity_id AND ra.occurred_at = ri.returned_with_activity_at
		         WHERE %[1]s)
		   AND %[2]s)`, items, readable)
	evidence = `ARRAY(
		SELECT ev.id FROM unnest(bi.evidence_ids) WITH ORDINALITY AS ev(id, n)
		 WHERE ev.id = bi.deal_id OR ev.id IN (SELECT id FROM served_activity)
		 ORDER BY ev.n)`
	returnedWith = `(CASE WHEN EXISTS (
		SELECT 1 FROM activity ev_a
		JOIN activity_link ev_l ON ev_l.activity_id = ev_a.id AND ev_l.deal_id = bi.deal_id
		 WHERE ev_a.occurred_at = bi.returned_with_activity_at AND ev_a.id IN (SELECT id FROM served_activity))
		THEN bi.returned_with_activity_at END)`
	return with, evidence, returnedWith, nil
}

// A cached finding depends on its original evidence. A reader who lost any
// cited activity may still see the deal, but cannot receive that derived text.
func servedFindingSQL(evidence string) string {
	return fmt.Sprintf(`CASE WHEN cardinality(%s) = cardinality(bi.evidence_ids)
		THEN coalesce(bi.finding, '') ELSE '' END`, evidence)
}
