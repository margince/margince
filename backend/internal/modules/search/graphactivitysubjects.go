// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

// What a meeting is ABOUT: the records an event names, the three ways it names
// them, and the precedence that decides which one a prep is built around.

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// activitySubject is one record an event names, with the key that decides
// which of them the prep is built around.
type activitySubject struct {
	entityType string
	id         ids.UUID
	title      string
	// tier, named and role together are the precedence, most significant
	// first; id breaks the remaining ties so the choice is deterministic.
	tier  int
	named int
	role  int
}

// activityLinkArm is one record type an activity_link row can point at: the
// entity and the link column that reaches it.
type activityLinkArm struct {
	entity string
	column string
}

// title is the expression that renders this record type for a human, read off
// the module's one entity table rather than restated here — so a record named
// in a prep reads exactly as it reads in a search result and on an anchor
// profile. An entity with no branch has no title, which is a broken read the
// arm gate (TestEverySubjectLinkArmIsRanked) refuses before it can ship.
func (a activityLinkArm) title() string {
	for _, branch := range searchBranches {
		if branch.entity == a.entity {
			return branch.title
		}
	}
	return ""
}

// activityLinkArms is EVERY arm of activity_link, in no particular order —
// subjectTier decides the dereference precedence and relatedSectionOrder
// decides which of them the hop-2 walk reports.
//
// All five are here, and the completeness is load-bearing: the first draft of
// the dereference borrowed the hop-2 walk's shorter list and so dropped the
// lead arm (core 0038), which is exactly the discovery call a prep is most
// often for.
//
// Held by: TestEverySubjectLinkArmIsRanked
// (backend/internal/modules/search/graphactivity_test.go), which reads the
// DDL's own enum rather than a sibling list in Go.
var activityLinkArms = []activityLinkArm{
	{entity: string(datasource.EntityPerson), column: "person_id"},
	{entity: string(datasource.EntityOrganization), column: "organization_id"},
	{entity: string(datasource.EntityDeal), column: "deal_id"},
	{entity: string(datasource.EntityProject), column: "project_id"},
	{entity: string(datasource.EntityLead), column: "lead_id"},
}

// subjectTier orders record types by how much of the meeting they account for:
// the work first (a deal, then a project), then the account, then a single
// contact. A prep built around the deal answers what is at stake in the room;
// the same prep built around one attendee answers a smaller question.
//
// A lead comes last, below the person it may one day become: an event naming
// both has a promoted record to prepare against, and that is the one with a
// neighborhood. An event naming ONLY a lead still prepares against it — an
// honest "this is all we hold" beats naming nothing at all.
var subjectTier = map[string]int{
	string(datasource.EntityDeal):         0,
	string(datasource.EntityProject):      1,
	string(datasource.EntityOrganization): 2,
	string(datasource.EntityPerson):       3,
	string(datasource.EntityLead):         4,
}

// How the event came to name the record, weakest evidence last. A link is
// something capture ASSERTED about the record; a participant is something it
// MATCHED from an address; an employer is something this module INFERRED from
// the attendee's current job. All three can reach the same record, and the fold
// keeps it at its strongest.
//
// The employer hop is what lets a company be reached through the person who was
// in the room, which is the model the activity_link refusal for meetings and
// calls rests on: without it, forbidding the direct link would remove the only
// path an organization had into a prep rather than a redundant one.
const (
	namedByLink        = 0
	namedByParticipant = 1
	namedByEmployer    = 2
)

// participantRoleRank puts the party who convened the meeting — or sent the
// message — ahead of the ones who were invited to it.
var participantRoleRank = map[string]int{
	"organizer": 0, "from": 1, "to": 2, "cc": 3, "attendee": 4,
}

// unrankedRole is where a role the map does not name sorts: last among the
// participants, never ahead of a named one. The role vocabulary is a CHECK
// constraint (0157_activity_participant.up.sql) that may gain a member without
// this map hearing about it, and the ordering stays total when it does.
var unrankedRole = len(participantRoleRank)

// participantRoleOrder renders participantRoleRank as SQL, so the bounded
// participant window is CUT by role rather than by id.
//
// This exists because the two are not interchangeable at the boundary. The
// reads below stop at graphExpansionLimit rows like every other leg of the
// walk, and a fifty-attendee meeting whose organizer happens to hold a high id
// would have its organizer cut from the window before the Go-side precedence
// ever saw them — the prep would then be built around whichever attendee
// sorted first, which is the one outcome the organizer rule exists to prevent.
//
// It is rendered FROM the map rather than spelled a second time in SQL: two
// copies of an ordering are two chances for the window and the fold to
// disagree, and they would disagree silently.
func participantRoleOrder(alias string) string {
	var order strings.Builder
	order.WriteString("CASE " + alias + ".role")
	for _, role := range slices.Sorted(maps.Keys(participantRoleRank)) {
		fmt.Fprintf(&order, " WHEN '%s' THEN %d", role, participantRoleRank[role])
	}
	fmt.Fprintf(&order, " ELSE %d END", unrankedRole)
	return order.String()
}

// linkOnlyRole is the role slot a link carries. A link names no party, so it
// ranks ahead of every participant role within its tier — which is the same
// thing namedByLink already says, and saying it twice keeps the comparison a
// plain field-by-field one.
const linkOnlyRole = -1

// activitySubjects resolves the records an event is about, best subject first.
//
// Every candidate is visibility-probed individually. The anchor's own scope is
// the ANY-link rule (auth.ActivityContentClause), so a meeting linked to one deal
// the caller owns and one they do not is readable while the second deal is not:
// dereferencing widens context, never authority.
func activitySubjects(ctx context.Context, tx pgx.Tx, activityID ids.UUID) ([]activitySubject, error) {
	linked, err := linkedSubjects(ctx, tx, activityID)
	if err != nil {
		return nil, err
	}
	attending, err := participantSubjects(ctx, tx, activityID)
	if err != nil {
		return nil, err
	}
	employers, err := employerSubjects(ctx, tx, activityID)
	if err != nil {
		return nil, err
	}
	return rankSubjects(ctx, tx, slices.Concat(linked, attending, employers))
}

// linkedSubjects reads the records capture linked to the event, one bounded
// read per arm of activity_link.
func linkedSubjects(ctx context.Context, tx pgx.Tx, activityID ids.UUID) ([]activitySubject, error) {
	var out []activitySubject
	for _, arm := range activityLinkArms {
		// The title goes in unqualified, as anchorProfile spells it: the lead
		// arm's is an expression over several columns, and none of the title
		// columns collides with one on activity_link, so `t` is the only table
		// they can resolve against.
		rows, err := tx.Query(ctx, fmt.Sprintf(`
			SELECT DISTINCT t.id, %s
			  FROM activity_link l JOIN %s t ON t.id = l.%s
			 WHERE l.activity_id = $1 AND l.%s IS NOT NULL AND t.archived_at IS NULL
			 ORDER BY t.id LIMIT %d`,
			arm.title(), arm.entity, arm.column, arm.column, graphExpansionLimit), activityID)
		if err != nil {
			return nil, fmt.Errorf("search: reading the records an event links to: %w", err)
		}
		for rows.Next() {
			subject := activitySubject{
				entityType: arm.entity, tier: subjectTier[arm.entity],
				named: namedByLink, role: linkOnlyRole,
			}
			if err := rows.Scan(&subject.id, &subject.title); err != nil {
				rows.Close()
				return nil, err
			}
			out = append(out, subject)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// participantSubjects reads the people capture matched to the event's parties.
// There is no project or organization half of activity_participant — those
// reach a prep through activity_link like everything else.
func participantSubjects(ctx context.Context, tx pgx.Tx, activityID ids.UUID) ([]activitySubject, error) {
	rows, err := tx.Query(ctx, `
		SELECT p.id, p.full_name, ap.role
		  FROM activity_participant ap JOIN person p ON p.id = ap.person_id
		 WHERE ap.activity_id = $1 AND p.archived_at IS NULL
		 ORDER BY `+participantRoleOrder("ap")+`, p.id LIMIT $2`, activityID, graphExpansionLimit)
	if err != nil {
		return nil, fmt.Errorf("search: reading the people on an event: %w", err)
	}
	defer rows.Close()
	var out []activitySubject
	for rows.Next() {
		subject := activitySubject{
			entityType: string(datasource.EntityPerson),
			tier:       subjectTier[string(datasource.EntityPerson)], named: namedByParticipant,
		}
		var role string
		if err := rows.Scan(&subject.id, &subject.title, &role); err != nil {
			return nil, err
		}
		rank, ok := participantRoleRank[role]
		if !ok {
			rank = unrankedRole
		}
		subject.role = rank
		out = append(out, subject)
	}
	return out, rows.Err()
}

// employerSubjects reads the companies the event's people currently work for.
//
// This is the hop that makes "a company is reached through the person who was
// in the room" true of the ASSEMBLY PATH rather than only of the model. Before
// it, an organization reached a prep by activity_link alone, so forbidding the
// direct link on a meeting removed the company from every surface that
// assembles context rather than removing a redundancy.
//
// BOTH ways a person is on an event, because they are different facts and
// capture writes each of them: activity_link is the person the event was filed
// against, activity_participant is the address it matched. A hop over one of
// them would leave the other's company unreachable, which is the whole failure
// this exists to prevent — and it is the same pair of arms
// activities.OrgLinkedActivityExists already walks for the account timeline.
//
// CURRENT employment only, and by design: an attendee's former employer is a
// company they left, and naming it in a prep would put the reader in the wrong
// room. A person with two current jobs contributes both — the primary first,
// because that is the one the rest of the product treats as theirs.
//
// Every organization it proposes still goes through rankSubjects like every
// other candidate, so an employer the caller may not read is ABSENT rather than
// a refusal — the same treatment the organization link arm has always had.
//
// The EDGE carries its own gate, and it is not the organization's. "This
// attendee works at that company" is a fact about a pair, which is what
// relationship.read governs and what neither endpoint's grant covers — so a
// caller with no edge grant learns no employer here at all, and one with a
// bounded grant learns only the edges their own scope reaches. Refused, the hop
// contributes nothing rather than failing the prep: the company is context the
// caller did not ask for by name, exactly as rankSubjects treats a record type
// they hold no grant on.
func employerSubjects(ctx context.Context, tx pgx.Tx, activityID ids.UUID) ([]activitySubject, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	activityPos := arg(activityID)
	limitPos := arg(graphExpansionLimit)
	edgeBound, err := auth.EdgeReadScope(ctx, "r", arg)
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if edgeBound == "" {
		edgeBound = "true"
	}
	// Two orderings, and they are not the same one. The inner ORDER BY is what
	// DISTINCT ON requires: it leads with the key and decides WHICH row survives
	// per company — the primary job of the best-placed party. The outer one is
	// what the LIMIT cuts by, for the reason participantRoleOrder exists: a
	// bound applied to the inner order would keep the companies whose ids sort
	// first, which is nobody's idea of the most relevant ones.
	//
	// A person the event both links and lists sorts at the LINK's rank, since
	// linkOnlyRole is ahead of every participant role — the same thing the fold
	// does for a record named twice.
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT e.id, e.display_name, e.role_rank FROM (
			SELECT DISTINCT ON (o.id)
			       o.id, o.display_name, onEvent.role_rank,
			       r.is_current_primary AS primary_job
			  FROM (
			        SELECT l.person_id, `+strconv.Itoa(linkOnlyRole)+` AS role_rank
			          FROM activity_link l
			         WHERE l.activity_id = $%[1]d AND l.person_id IS NOT NULL
			        UNION ALL
			        SELECT ap.person_id, `+participantRoleOrder("ap")+` AS role_rank
			          FROM activity_participant ap
			         WHERE ap.activity_id = $%[1]d AND ap.person_id IS NOT NULL
			       ) onEvent
			  JOIN person p ON p.id = onEvent.person_id
			  JOIN relationship r ON r.person_id = p.id
			  JOIN organization o ON o.id = r.organization_id
			 WHERE p.archived_at IS NULL
			   AND r.kind = 'employment' AND r.ended_at IS NULL AND r.archived_at IS NULL
			   AND o.archived_at IS NULL
			   AND %[3]s
			 ORDER BY o.id, r.is_current_primary DESC, onEvent.role_rank
		) e
		 ORDER BY e.primary_job DESC, e.role_rank, e.id
		 LIMIT $%[2]d`, activityPos, limitPos, edgeBound), args...)
	if err != nil {
		return nil, fmt.Errorf("search: reading the companies an event's people work for: %w", err)
	}
	defer rows.Close()
	var out []activitySubject
	organization := string(datasource.EntityOrganization)
	for rows.Next() {
		subject := activitySubject{
			entityType: organization,
			tier:       subjectTier[organization], named: namedByEmployer,
		}
		if err := rows.Scan(&subject.id, &subject.title, &subject.role); err != nil {
			return nil, err
		}
		out = append(out, subject)
	}
	return out, rows.Err()
}

// rankSubjects drops what the caller may not see, folds the duplicates, and
// orders what is left.
//
// A record reached twice — linked AND on the invitation, or copied under two
// roles — is ONE subject at its best rank. Without the fold the same account
// would appear in also_present beside itself, and a prep that lists the same
// company twice reads as two accounts.
// Two gates, and they answer different questions. Object RBAC asks whether the
// caller may read that KIND of record at all, and a denied kind is absent
// rather than a 403 — a subject is context the caller did not ask for by name.
// The row-scope probe then asks whether they may read THAT record.
//
// foldSubjects returns them in precedence order and this only ever drops, so
// the order survives and needs no second sort.
func rankSubjects(ctx context.Context, tx pgx.Tx, candidates []activitySubject) ([]activitySubject, error) {
	out := make([]activitySubject, 0, len(candidates))
	for _, subject := range foldSubjects(candidates) {
		if auth.Require(ctx, subject.entityType, principal.ActionRead) != nil {
			continue
		}
		visible, err := auth.VisibleTo(ctx, tx, subject.entityType, subject.id)
		if err != nil {
			return nil, err
		}
		if visible {
			out = append(out, subject)
		}
	}
	return out, nil
}

// foldSubjects reduces the candidates to one entry per record, at its best
// rank, in a deterministic order — the pure half of rankSubjects, so the
// precedence can be proven without a database.
func foldSubjects(candidates []activitySubject) []activitySubject {
	best := map[datasource.EntityRef]activitySubject{}
	for _, candidate := range candidates {
		ref := datasource.EntityRef{Type: datasource.EntityType(candidate.entityType), ID: candidate.id}
		if held, seen := best[ref]; seen && !subjectPrecedes(candidate, held) {
			continue
		}
		best[ref] = candidate
	}
	out := make([]activitySubject, 0, len(best))
	for _, subject := range best {
		out = append(out, subject)
	}
	sort.Slice(out, func(i, j int) bool { return subjectPrecedes(out[i], out[j]) })
	return out
}

// subjectPrecedes is the precedence, most significant first: the tier of
// record, then whether the event asserted the record or merely matched it,
// then the party's role, then the id so the order is total.
func subjectPrecedes(a, b activitySubject) bool {
	switch {
	case a.tier != b.tier:
		return a.tier < b.tier
	case a.named != b.named:
		return a.named < b.named
	case a.role != b.role:
		return a.role < b.role
	default:
		return a.id.String() < b.id.String()
	}
}

func subjectItem(subject activitySubject) graphItem {
	return graphItem{entityType: subject.entityType, id: subject.id, summary: subject.title}
}

// subjectItems renders a run of subjects, bounded. The order is the
// precedence's, so the bound keeps the best of them — sortAndTrim is
// deliberately not used, because these items carry no §10.7.2 rank score and a
// zero would reorder them by id.
func subjectItems(subjects []activitySubject, maxItems int) []graphItem {
	if len(subjects) > maxItems {
		subjects = subjects[:maxItems]
	}
	items := make([]graphItem, 0, len(subjects))
	for _, subject := range subjects {
		items = append(items, subjectItem(subject))
	}
	return items
}
