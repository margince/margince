// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// Which activities belong to which record — the timeline list's filter, and
// the account walk every other reader of the account's timeline shares.

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// CompanyLinkedActivityExists tests the activity aliased as a against one account.
func CompanyLinkedActivityExists(companyPos int) string {
	return storekit.CompanyActivityExists(sprintf("$%d", companyPos))
}

// CompanyLinkedActivityExistsAny tests the activity against a bound array of accounts.
func CompanyLinkedActivityExistsAny(companiesPos int) string {
	return storekit.CompanyActivityExists(sprintf("ANY($%d)", companiesPos))
}

// CompanyReachedActivities returns (activity_id, company_id) for the bound accounts.
func CompanyReachedActivities(companiesPos int) string {
	return storekit.CompanyActivityReach(sprintf("ANY($%d)", companiesPos))
}

// CompanyFiledActivityExists leaves out participant employers: a file does
// not belong to an account merely because its employee was copied.
func CompanyFiledActivityExists(companyPos int) string {
	return storekit.CompanyFiledActivityExists(sprintf("$%d", companyPos))
}

// CompanyReachSet attributes signals through links only. A copied participant
// makes an activity relevant to an account's history without filing it there.
func CompanyReachSet() string {
	return storekit.CompanyActivityLinkAttribution()
}

// ActivityWithinProject is the ONE spelling of "this activity belongs to the
// body of work being asked about", for a query that aliases activity as a.
//
// The rule (D7): filed under THIS project, or filed under none. Not "not filed
// elsewhere" — those two readings differ on an activity linked to this project
// AND another, which this one keeps and that one drops.
// `uq_activity_link_project` makes that row unreachable today, but the rule is
// what the predicate owes its reader, so it is spelled as the rule rather than
// inferred from an index a later migration could relax without ever touching
// this file.
//
// KEEPING THE UNFILED ROWS IS THE POINT. Attribution is optional, so most
// correspondence on an account carries no project at all: a plain equality test
// would drop the general relationship history and describe an account as though
// nobody had ever spoken to it.
//
// It must be a subquery over the activity's links rather than a test on a link
// row already joined — `activity_link_shape` admits exactly ONE target per row,
// so a contact-link row carries a NULL project_id by construction and a
// predicate on it would be true everywhere and narrow nothing. Both arms probe
// `uq_activity_link_project`, which is keyed on activity_id.
//
// search.projectScope.clause is the same predicate for the context walk. The
// two are deliberate copies, not an oversight: a module never imports a sibling
// (ADR-0054), and this rule is about SUBJECT MATTER rather than authority, so
// platform/auth — where the activity scope clauses both modules do share
// already live — is the wrong home for it. Change one, change both.
//
// projectPos is the bind position carrying the project id.
func ActivityWithinProject(projectPos int) string {
	return sprintf(`(
			EXISTS (
			    SELECT 1 FROM activity_link scoped
			    WHERE scoped.activity_id = a.id AND scoped.project_id = $%[1]d)
			OR NOT EXISTS (
			    SELECT 1 FROM activity_link filed
			    WHERE filed.activity_id = a.id AND filed.project_id IS NOT NULL))`,
		projectPos)
}

// RequireProjectScope is the authority check every read narrowed BY a project
// owes before it filters on one. Filtering by a record is a read of it: the
// scoped page answers "these activities are filed under this project", which
// a caller with no project grant may not learn, and a caller outside its row
// scope may not learn it exists. Object denial is a 403; an invisible,
// archived or missing project is the same existence-hiding 404 a direct read
// gives.
//
// The LIVE probe, not the plain one: EnsureVisible lets an unbounded caller
// through without touching the table, so a scope naming a project nobody ever
// created would answer a full page as though the filter had matched, and an
// archived project is no longer a body of work a page can be narrowed to.
func RequireProjectScope(ctx context.Context, tx pgx.Tx, projectID ids.ProjectID) error {
	if err := auth.Require(ctx, string(datasource.RecordProject), principal.ActionRead); err != nil {
		return err
	}
	return auth.EnsureVisibleLive(ctx, tx, string(datasource.RecordProject), projectID.UUID)
}

// openTaskAssigneeClause narrows the timeline to the OPEN tasks one contact
// holds — the queue read the contract declares ("Open tasks for an
// assignee"), spelled as the predicate the partial index behind it is built
// on (idx_activity_tasks: workspace_id, assignee_id, due_at WHERE kind='task'
// AND NOT is_done AND archived_at IS NULL).
//
// Done-ness belongs to the filter rather than to a dial of its own, and that
// is the whole point: no parameter answers it, so binding assignee_id as a
// plain column match would hand back every task the contact ever closed under
// a name the contract says means the open ones. A wider answer wearing the
// declared answer's shape is the failure this filter exists to close, not a
// convenience to preserve.
//
// `kind` is stated rather than implied. The `activity_task_fields` CHECK
// already keeps assignee_id NULL on every other kind, so it narrows nothing —
// it is what lets the planner match the partial index.
func openTaskAssigneeClause(assignee *ids.UserID, asOf *time.Time, identities map[string][]string, arg func(any) int) string {
	if assignee == nil {
		return ""
	}
	if asOf == nil || asOf.IsZero() {
		return "FALSE"
	}
	return sprintf("a.assignee_id = $%d AND a.kind = 'task' AND NOT a.is_done", arg(*assignee)) + " AND " + CurrentTaskClause("a", *asOf, sprintf("$%d::jsonb", arg(identities)), arg)
}

// Personal queues contain assigned work; taskAssignee owns a human's new task.
// Automatic unassigned work belongs to unassignedQueueClause.
func ownQueueClause(reader *ids.UserID, arg func(any) int) string {
	if reader == nil {
		return ""
	}
	return sprintf("a.assignee_id = $%d AND a.kind = 'task' AND NOT a.is_done", arg(*reader))
}

// unassignedQueueClause is the open tasks nobody answers for.
//
// Its own scope rather than a corner of somebody's queue: unowned work is real
// and has to be visible, but it is picked up deliberately. Arriving unbidden in
// every reader's "mine" is what taught reps that the queue was not theirs.
func unassignedQueueClause() string {
	return "a.assignee_id IS NULL AND a.kind = 'task' AND NOT a.is_done"
}

// teamQueueClause is the open tasks a team view keeps: a seat's own, or
// nobody's, as the Worklist's roster filter keeps them.
func teamQueueClause(seats []ids.UUID, arg func(any) int) string {
	return sprintf("(a.assignee_id IS NULL OR a.assignee_id = ANY($%d::uuid[])) AND a.kind = 'task' AND NOT a.is_done", arg(seats))
}

// onMeetingOfClause is the meetings one seat answers for, by the one
// meeting-responsibility rule. It narrows a worklist and grants nothing: these
// rows have already passed the row-scope gate and the audience arm.
func onMeetingOfClause(reader *ids.UserID, arg func(any) int) string {
	if reader == nil {
		return ""
	}
	return "a.kind = 'meeting' AND " + storekit.MeetingResponsibilitySQL("a", sprintf("$%d", arg(*reader)))
}
