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
	"github.com/margince/margince/backend/internal/shared/kernel/employment"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// CompanyLinkedActivityExists is the ONE spelling of "this activity reaches the
// account", for a query that aliases activity as a.
//
// A message, a task or a meeting belongs to a company through any of four
// arms — its own link, its deal's, the contact it is about, or the employer of
// somebody who was on it — and every reader of the account's timeline needs the
// same walk: the timeline list itself, the company view's next-steps section,
// and the two suggestion reads. Spelling it once is what keeps them from
// drifting apart: an arm added to the model reaches every reader, or none of
// them.
//
// It lives in this module rather than next to the company view because a module
// may not import compose, and the timeline list is a reader too — the drift this
// replaces was exactly that, a flat company_id match in the list against
// the three-arm walk in the view.
//
// The predicate answers WHICH activities belong to the account and nothing else.
// WHO may read one is auth.ActivityContentClause, which every caller composes
// alongside this.
//
// companyPos is the bind position carrying the company id; the caller registers
// it once and every arm reads the same one.
func CompanyLinkedActivityExists(companyPos int) string {
	return activityReachesCompany(sprintf("$%d", companyPos))
}

// CompanyLinkedActivityExistsAny is the same walk over a SET of companies, for
// a caller that binds an id array rather than one id.
//
// The hierarchy roll-up needs it, for a 30-day count that must answer the same
// question as the timeline it is displayed above. Capture files mail against
// the CONTACT it was with, so a flat company_id match misses an account's
// busiest correspondence. One walk, two operand shapes, so a fourth link added
// to the model still reaches both.
//
// companiesPos is the bind position carrying the company id array.
func CompanyLinkedActivityExistsAny(companiesPos int) string {
	return activityReachesCompany(sprintf("ANY($%d)", companiesPos))
}

// CompanyReachedActivities is the timeline's four-arm reach for the bound
// accounts: (activity_id, company_id) for each account an activity reaches.
// CompanyReachSet is the three-arm set signals are filed through instead.
func CompanyReachedActivities(companiesPos int) string {
	return companyReach(sprintf("ANY($%d)", companiesPos))
}

// companyReach is the walk as a set with one branch per arm. Each branch filters
// on its arm's indexed company column, so a plan starts from the account. An
// `OR` across the arms would leave Postgres no index to start from.
//
// participantEmployerArm correlates on a.occurred_at inside its join condition,
// so Postgres keeps it a per-activity probe that cannot start from the account.
// Its branch reaches the employees' activities through participantEntry first,
// and that same `EXISTS` decides which of them count.
func companyReach(operand string) string {
	participantBranch := sprintf(participantEntry+participantEmployerArm, operand, "e0.company_id")
	return companyLinkReach(operand) + "\n\t\t    UNION " + participantBranch
}

// companyLinkReach is companyReach without the participant arm: the activities
// filed against the account, its deals or its employees.
func companyLinkReach(operand string) string {
	linkBranch := func(column string) string {
		return sprintf("SELECT l.activity_id, %[1]s AS company_id %[2]s\n\t\t    WHERE %[1]s = %[3]s",
			column, companyArms, operand)
	}
	return linkBranch("l.company_id") + "\n\t\t    UNION " + linkBranch("d.company_id") +
		"\n\t\t    UNION " + linkBranch("r.company_id")
}

// CompanyFiledActivityExists is the predicate over the three link arms only. A
// file of a mail that merely CC'd an employee is not a document of that
// employee's employer.
func CompanyFiledActivityExists(companyPos int) string {
	return "a.id IN (SELECT reach.activity_id FROM (" + companyLinkReach(sprintf("$%d", companyPos)) + ") reach)"
}

// companyArms is the three links themselves — the account an activity is filed
// against, the account its deal belongs to, and the employer of the contact it
// is about.
//
// The arms live apart from the shapes built on them, because drift happens in
// the arms: one spelling gains a condition and another does not. The
// shapes differ for a reason that will not go away (CompanyReachSet says it).
//
// search/graphcompanyreach.go spells the SAME arms for the context walk, because a
// module never imports a sibling (ADR-0054). The two texts are held equal by
// gates/accountreachcopies_test.go rather than by anybody remembering to change
// both — an arm added here alone is a failure, not a divergence nobody sees.
//
// The deal arm deliberately does not exclude archived or lost deals: a set
// stricter than the predicate would show a message on the timeline whose
// account never gets a signal about it.
//
// The employment arm is bounded by when the message was sent. The set form has
// no outer activity to read that from, so the arms join the activity.
var companyArms = `FROM activity_link l
		    JOIN activity la ON la.id = l.activity_id
		    LEFT JOIN deal d ON d.id = l.deal_id
		    LEFT JOIN relationship r ON r.contact_id = l.contact_id AND r.kind = 'employment'
		      AND ` + employment.IsCurrentSQL("r.ended_at") + ` AND r.archived_at IS NULL
		      AND ` + employment.InPlaceAtSQL("r", "la.occurred_at")

// participantEmployerArm is the READER's fourth arm: the employer of somebody
// who is on the event as a participant rather than as a link.
//
// The three arms above all start from activity_link, and a meeting may not
// carry a direct company link at all — a company is not somebody you can
// meet. So a meeting whose only contact is on the invitation reaches no company
// through them, and the account whose contact sat in the room sees an empty
// afternoon. search.employerSubjects already walks BOTH contact sources for the
// context prep; this is the same pair for the reach walk, and the two agreeing
// is the point.
//
// Anchored on the activity rather than folded into companyArms, because companyArms is
// shared with CompanyReachSet and this arm deliberately is not — the ruling, and
// why, is on CompanyReachSet.
//
// CURRENT employment, exactly as the link arm reads it: a former colleague's
// company is one they left.
//
// The alias is emp rather than r: the row-scope clause a caller composes
// alongside this one is recognized by its "r." tokens, and a second
// relationship in scope under that name reads as the first.
var participantEmployerArm = `EXISTS (
		    SELECT 1 FROM activity_participant ap
		      JOIN relationship emp ON emp.contact_id = ap.contact_id AND emp.kind = 'employment'
		        AND ` + employment.IsCurrentSQL("emp.ended_at") + ` AND emp.archived_at IS NULL
		        AND ` + employment.InPlaceAtSQL("emp", "a.occurred_at") + `
		    WHERE ap.activity_id = a.id AND emp.company_id = %s)`

// participantEntry is where companyReach's participant branch starts: every
// activity an employee of the account is on, for participantEmployerArm to
// decide.
var participantEntry = `SELECT a.id, e0.company_id FROM relationship e0
		    JOIN activity_participant p0 ON p0.contact_id = e0.contact_id
		    JOIN activity a ON a.id = p0.activity_id
		    WHERE e0.company_id = %s AND e0.kind = 'employment' AND `

// activityReachesCompany is the walk as a PREDICATE, for a query that aliases
// activity as a. operand is what each arm compares its company id against: a
// single bind or ANY(array).
//
// It is a membership test against companyReach rather than an `EXISTS` per arm.
// Correlated `EXISTS` joined by `OR` leave Postgres walking activity
// newest-first, so a quiet account costs the whole table. The set starts from
// the account, at the price of building a busy account's whole set first.
func activityReachesCompany(operand string) string {
	return "a.id IN (SELECT reach.activity_id FROM (" + companyReach(operand) + ") reach)"
}

// CompanyReachSet is the same walk as a SET: the body of a derived table producing
// one (activity_id, company_id) row per account an activity reaches.
//
// The predicate above answers "does this activity reach account X" and takes
// the account as a bind. A producer scanning the whole workspace has the
// opposite question — it holds an activity and needs the accounts — so it
// cannot use the predicate at all.
//
// THREE ARMS, NOT THE PREDICATE'S FOUR. The predicate also reaches through a
// participant's employer; this set does not, and the difference is the point
// rather than a lag. This set is what a SIGNAL is filed through, and the note
// below already draws the line it sits on: a timeline showing something is
// arguably being helpful, a signal filed against that account is a claim
// nobody made. Somebody Cc'd on a message is weaker evidence than somebody the
// message was filed against, and filing against every Cc'd contact's employer
// would put claims on accounts that were never in the conversation.
//
// TestTheReachSetDoesNotFileThroughParticipants holds the split, so collapsing
// it is a decision somebody makes rather than a line they add.
//
// DISTINCT collapses an activity that reaches one account through several arms
// (its own link and its deal's, say) to one row, so a caller counting messages
// is not counting links. An activity that reaches TWO accounts is two rows on
// purpose: whether that is an ambiguity to refuse or a fact to file twice is
// the caller's ruling, not this fragment's.
//
// No entity_type filter: the activity_link_shape CHECK already guarantees
// exactly one of the three id columns is set per row, and the predicate omits
// it for the same reason.
//
// No workspace filter: activity_link, deal and relationship all carry FORCE
// row-level security, and every caller runs inside WithWorkspaceTx.
//
// The employment arm reaches only mail sent while the edge was in place
// (employment.InPlaceAtSQL). Mail from someone's previous job is therefore not
// filed against their employer today. An edge with no date stays unbounded.
// For those, the extractor's one-account rule carries the weight: a contact
// with two live employers makes their conversations ambiguous and skipped.
func CompanyReachSet() string {
	return sprintf(`SELECT DISTINCT l.activity_id, o.company_id AS company_id
		    %s
		    CROSS JOIN LATERAL (VALUES (l.company_id), (d.company_id),
		                              (r.company_id)) AS o(company_id)
		    WHERE o.company_id IS NOT NULL`, companyArms)
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

// onMeetingOfClause is the meetings one seat answers for, by the one
// meeting-responsibility rule. It narrows a worklist and grants nothing: these
// rows have already passed the row-scope gate and the audience arm.
func onMeetingOfClause(reader *ids.UserID, arg func(any) int) string {
	if reader == nil {
		return ""
	}
	return "a.kind = 'meeting' AND " + storekit.MeetingResponsibilitySQL("a", sprintf("$%d", arg(*reader)))
}
