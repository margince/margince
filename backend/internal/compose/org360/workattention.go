// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package org360

// Why the account's work in flight needs a person.
//
// The company page used to open with a written account brief: one narrative
// over every deal and every project at once. On an account carrying several
// engagements that is the wrong shape — correspondence about one project
// becomes a sentence about another, and a figure read out of the blend has
// nowhere to be checked. This decorates each row with ONE fact drawn only
// from that row's own records, so a deal's reason comes from the deal and a
// project's from the project, and both point at the receipt.
//
// Deterministic on purpose. A model could phrase these more warmly; it could
// not make them checkable. The brief this replaced was not checkable, which is
// why the page stopped drawing it.
//
// Three set-based queries for the whole page, never one per row: a record
// page's cost may not grow with the size of the account, which
// org360_querycount_integration_test.go holds as a line.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/gradionhq/margince/backend/internal/contracts"
	"github.com/gradionhq/margince/backend/internal/modules/activities"
	"github.com/gradionhq/margince/backend/internal/modules/people"
	"github.com/gradionhq/margince/backend/internal/platform/auth"
	"github.com/gradionhq/margince/backend/internal/shared/apperrors"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/principal"
)

// overdueTask is one open task past its due date, on whichever record it was
// linked to. Who is empty when the assignee is nobody this caller may name.
type overdueTask struct {
	Subject string
	Who     string
	DueAt   time.Time
}

// readWorkAttention hangs the attention fact on the deals and projects the
// page has already read, and says so when it could not.
//
// It runs after those sections rather than inside them because a deal's
// reason may come from a task and a project's from a conversation, and split
// between the two sections that would be two half-copies of one set read.
//
// A caller without the activity grant gets the rows without reasons and a
// payload that SAYS the reasons are missing. This is why the denial is
// swallowed here rather than named in sections_omitted: the deals and the
// projects are present and true, and reporting them as withheld would hide
// the pipeline from a reader who may read it.
//
// The flag covers a PARTIAL refusal too — a caller who may read the tasks but
// not the people the commitments were made by keeps the reasons that were
// derived and is still told some are missing. Half the reasons with no such
// line would read as "these rows have nothing to explain", which is the one
// thing this card must never say by accident.
func (a *assembly) readWorkAttention() error {
	dealIDs, projects := attentionTargets(a.out)
	if len(dealIDs) == 0 && len(projects) == 0 {
		return nil
	}
	err := a.decorateWorkAttention(dealIDs, projects)
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		withheld := true
		a.out.AttentionWithheld = &withheld
		return nil
	}
	return err
}

func (a *assembly) decorateWorkAttention(dealIDs []ids.UUID, projects []ids.ProjectID) error {
	if err := auth.Require(a.ctx, "activity", principal.ActionRead); err != nil {
		return err
	}
	dealTasks, err := overdueTasksBy(a.ctx, a.tx, a.orgID, "deal_id", dealIDs, a.now)
	if err != nil {
		return err
	}
	projectTasks, err := overdueTasksBy(a.ctx, a.tx, a.orgID, "project_id", projectUUIDs(projects), a.now)
	if err != nil {
		return err
	}
	commitments, err := a.svc.people.CommitmentsTheirsForProjects(a.ctx, a.tx, projects, a.now)
	if err != nil {
		return err
	}
	if a.out.Deals != nil {
		for i := range a.out.Deals.Data {
			row := &a.out.Deals.Data[i]
			row.Attention = taskAttention(dealTasks[ids.UUID(row.DealId)])
		}
	}
	if a.out.Projects != nil {
		for i := range *a.out.Projects {
			row := &(*a.out.Projects)[i]
			id := ids.From[ids.ProjectKind](ids.UUID(row.ProjectId))
			attention := taskAttention(projectTasks[ids.UUID(row.ProjectId)])
			if attention == nil {
				attention = commitmentAttention(commitments[id])
			}
			row.Attention = attention
		}
	}
	return nil
}

// attentionTargets is what the page has in flight: the open deals and the
// live projects it already listed. A withheld section contributes nothing —
// there are no rows to decorate, and asking about ids the caller could not
// read would be the disclosure the section's own gate refused.
func attentionTargets(out *crmcontracts.Organization360) ([]ids.UUID, []ids.ProjectID) {
	var deals []ids.UUID
	if out.Deals != nil {
		for _, row := range out.Deals.Data {
			deals = append(deals, ids.UUID(row.DealId))
		}
	}
	var projects []ids.ProjectID
	if out.Projects != nil {
		for _, row := range *out.Projects {
			// A closed project is history, and "why does this need a person"
			// is not a question about history. It is also what the card shows,
			// so decorating one would be a fact nothing renders.
			if row.Phase == crmcontracts.Organization360ProjectPhaseClosed {
				continue
			}
			projects = append(projects, ids.From[ids.ProjectKind](ids.UUID(row.ProjectId)))
		}
	}
	return deals, projects
}

func projectUUIDs(projects []ids.ProjectID) []ids.UUID {
	out := make([]ids.UUID, 0, len(projects))
	for _, id := range projects {
		out = append(out, id.UUID)
	}
	return out
}

// overdueTasksBy reads the most overdue open task per linked record, for the
// whole set at once. linkColumn names which link the set is keyed on —
// activity_link.deal_id or activity_link.project_id — and is a compile-time
// literal from this file's two call sites, never a value off a request.
//
// The task must also reach the account: a link row alone would let a task
// filed under a project this company shares with another reach a page it does
// not belong to. activities.OrgLinkedActivityExists is the one spelling of
// that walk.
func overdueTasksBy(
	ctx context.Context, tx pgx.Tx, orgID ids.OrganizationID,
	linkColumn string, keys []ids.UUID, now time.Time,
) (map[ids.UUID]overdueTask, error) {
	out := map[ids.UUID]overdueTask{}
	if len(keys) == 0 {
		return out, nil
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	keysPos := arg(keys)
	orgPos := arg(orgID)
	nowPos := arg(now)
	activityScope, err := auth.ActivityContentClause(ctx, "a", arg)
	if err != nil {
		return nil, err
	}
	if activityScope == "" {
		activityScope = scopeAll
	}
	// DISTINCT ON needs its key first in ORDER BY, and the activity id breaks
	// the tie so two tasks due the same day do not swap places between two
	// reads of the same page.
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT DISTINCT ON (l.%[1]s)
		       l.%[1]s, coalesce(a.subject, ''), coalesce(u.display_name, ''), a.due_at
		  FROM activity a
		  JOIN activity_link l ON l.activity_id = a.id AND l.%[1]s = ANY($%[2]d)
		  LEFT JOIN app_user u ON u.id = a.assignee_id
		         AND u.status = 'active' AND u.archived_at IS NULL
		 WHERE a.kind = 'task' AND NOT a.is_done AND a.archived_at IS NULL
		   AND a.due_at IS NOT NULL AND a.due_at < $%[4]d
		   AND (%[5]s) AND %[6]s
		 ORDER BY l.%[1]s, a.due_at, a.id`,
		linkColumn, keysPos, orgPos, nowPos, activityScope,
		activities.OrgLinkedActivityExists(orgPos)), args...)
	if err != nil {
		return nil, fmt.Errorf("read the account's overdue tasks: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var key ids.UUID
		var task overdueTask
		if err := rows.Scan(&key, &task.Subject, &task.Who, &task.DueAt); err != nil {
			return nil, fmt.Errorf("scan an overdue task: %w", err)
		}
		out[key] = task
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read the account's overdue tasks: %w", err)
	}
	return out, nil
}

// taskAttention is the overdue task as the card's one fact, or nothing when
// the record had none. The zero value stands for "no task": a real one always
// carries a subject-or-empty and a due date, and the map returns the zero for
// a key it never saw.
func taskAttention(task overdueTask) *crmcontracts.Organization360WorkAttention {
	if task.DueAt.IsZero() {
		return nil
	}
	due := task.DueAt
	return &crmcontracts.Organization360WorkAttention{
		Kind:  crmcontracts.WorkAttentionOverdueTask,
		Title: task.Subject,
		Who:   namedOrNobody(task.Who),
		DueAt: &due,
	}
}

// commitmentAttention is the open commitment they made to us, as the card's
// one fact. The body travels verbatim: the extractor writes free text, so the
// card quotes what was said rather than asserting a paraphrase of it.
func commitmentAttention(commitment people.ProjectCommitment) *crmcontracts.Organization360WorkAttention {
	if commitment.Body == "" {
		return nil
	}
	source := openapi_types.UUID(commitment.ActivityID)
	return &crmcontracts.Organization360WorkAttention{
		Kind:             crmcontracts.WorkAttentionCommitmentTheirs,
		Title:            commitment.Body,
		Who:              namedOrNobody(commitment.Who),
		DueAt:            commitment.DueAt,
		SourceActivityId: &source,
	}
}

// namedOrNobody folds an unresolvable name to null rather than to an empty
// string, so a client renders the sentence without a name instead of leaving
// a gap where one belongs — the same fold the deal card's seat names take.
func namedOrNobody(name string) *string {
	if name == "" {
		return nil
	}
	return &name
}
