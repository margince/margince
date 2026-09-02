// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package org360

// The vocabulary every section read shares — the per-section row cap, the
// truncation flag, and the two row-scope predicates — plus the next-steps
// section itself. The larger sections live one concept per file
// (contacts.go, deals.go, collections.go).
//
// Every section prunes to the caller's row scope with the same
// platform/auth predicates the module lists use, so a section can never
// out-see the dedicated endpoint it summarizes.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/deadline"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// sectionLimit is how many rows of a nested collection one 360 carries.
// The section is a summary with a "there is more" flag, not a paging
// surface: follow-up pages come from the dedicated endpoint for that
// collection, which owns the cursor vocabulary.
const sectionLimit = 25

// truncate cuts a section to sectionLimit and reports whether it had to.
func truncate[T any](rows []T) ([]T, crmcontracts.PageInfo) {
	if len(rows) > sectionLimit {
		return rows[:sectionLimit], crmcontracts.PageInfo{HasMore: true}
	}
	return rows, crmcontracts.PageInfo{HasMore: false}
}

// scopeAll is the predicate an unbounded (admin) caller gets: the SQL
// that embeds a row-scope clause then needs only one spelling.
const scopeAll = "TRUE"

// scopeNone admits no row. It is what a section uses when the caller holds no
// grant over the rows at all, as against a row scope that narrows a set the
// grant has already opened.
const scopeNone = "FALSE"

// scopeClause resolves one object's row-scope predicate for the caller,
// answering scopeAll for an unbounded caller.
func scopeClause(ctx context.Context, object, alias string, arg func(any) int) (string, error) {
	clause, err := auth.ScopeClauseFor(ctx, object, alias, arg)
	if err != nil {
		return "", err
	}
	if clause == "" {
		return scopeAll, nil
	}
	return clause, nil
}

// edgeAlias is the alias every statement in this package gives the
// relationship table. Named rather than passed, because it is a property of how
// these statements are written and not a choice a caller makes — and a caller
// free to pass a different one could bound the wrong table.
const edgeAlias = "r"

// edgeScope resolves the relationship edge's READ admission and its row bound
// together, in the one spelling platform/auth owns, answering scopeAll for an
// unbounded caller.
//
// It is the edge's own gate and not a second reading of its endpoints': an edge
// discloses the two records it names AS A PAIR, so a caller holding both
// endpoint grants may still be refused the edge. A denial returns
// apperrors.ErrPermissionDenied unchanged, which is what lets the graph's group
// loop NAME the omission in groups_omitted rather than fail the whole card.
func edgeScope(ctx context.Context, arg func(any) int) (string, error) {
	clause, err := auth.EdgeReadScope(ctx, edgeAlias, arg)
	if err != nil {
		return "", err
	}
	if clause == "" {
		return scopeAll, nil
	}
	return clause, nil
}

// linkScope resolves "this activity_link row points at a record the caller
// can read" for one table alias, answering scopeAll for an unbounded caller.
func linkScope(ctx context.Context, alias string, arg func(any) int) (string, error) {
	clause, err := auth.LinkTargetVisibleClause(ctx, alias, arg)
	if err != nil {
		return "", err
	}
	if clause == "" {
		return scopeAll, nil
	}
	return clause, nil
}

// nextStepsSection reads the account's open tasks in the order a rep works
// them: overdue first, then dated, then undated, and among tasks sharing a
// date the one that has waited longest. That last tie-break is what makes
// this list agree with the contact page's (person360's byUrgency): the same
// two promises must not swap places depending on which record you opened
// them from, and without it each list fell back to its own id order. A task reaches the
// account through any of its links — the task itself, its deal, or the
// contact it is about — which is why the reachability test is an EXISTS
// over all three arms rather than one join.
//
// The two linked ids it reports carry their OWN row scope. Task visibility
// is an any-link rule, so a task reachable through a visible contact would
// otherwise hand back the id of a deal the caller may not read — the task
// is theirs to see, the colleague's deal is not.
func nextStepsSection(ctx context.Context, tx pgx.Tx, orgID ids.OrganizationID, now time.Time, opts AssembleOptions) ([]crmcontracts.Organization360NextStep, crmcontracts.PageInfo, error) {
	steps, page, _, err := readNextSteps(ctx, tx, orgID, now, opts, sectionLimit)
	return steps, page, err
}

// openTaskPromises reads the account's open tasks for the moment card, up to a
// bound wide enough that the ranking is not decided by where the read stopped,
// each paired with when it was filed.
//
// Held by: TestATaskCarriesTheMomentItWasFiled (moment_test.go), which fails
// when a task reaches the ranking without its filing moment.
//
// A SEPARATE CALL, not the section's rows. The section shows a PAGE — the
// twenty-five earliest deadlines — and the moment card RANKS over the set,
// asking which promise slipped most recently. Those two disagree exactly where
// it matters: on an account with twenty-six overdue tasks the page holds the
// oldest twenty-five, and the one that slipped yesterday, which is the one
// still worth rescuing, is the row that fell off.
//
// The filing moment travels because it breaks ties between two promises sharing
// a due date. The wire contract does not carry it — the account's task list does
// not show when a task was filed — so it rides beside the rows rather than
// widening a payload with a field no client renders.
func openTaskPromises(ctx context.Context, tx pgx.Tx, orgID ids.OrganizationID, now time.Time, opts AssembleOptions, limit int) ([]crmcontracts.Organization360NextStep, []time.Time, error) {
	steps, _, filed, err := readNextSteps(ctx, tx, orgID, now, opts, limit)
	return steps, filed, err
}

// readNextSteps is the read both callers share: same statement, same gates,
// same scan. Only the bound moves, and the filing moments come back for the
// caller that ranks on them.
func readNextSteps(ctx context.Context, tx pgx.Tx, orgID ids.OrganizationID, now time.Time, opts AssembleOptions, limit int) ([]crmcontracts.Organization360NextStep, crmcontracts.PageInfo, []time.Time, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	orgPos := arg(orgID)
	activityScope, err := auth.ActivityContentClause(ctx, "a", arg)
	if err != nil {
		return nil, crmcontracts.PageInfo{}, nil, err
	}
	if activityScope == "" {
		activityScope = scopeAll
	}
	// Two renderings of the same predicate, one per sub-select alias. Each
	// registers its own bind positions through arg(), which is why they are
	// built rather than string-substituted from one another.
	linkVisible, err := linkScope(ctx, "dl", arg)
	if err != nil {
		return nil, crmcontracts.PageInfo{}, nil, err
	}
	personVisible, err := linkScope(ctx, "pl", arg)
	if err != nil {
		return nil, crmcontracts.PageInfo{}, nil, err
	}
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT a.id, coalesce(a.subject, ''), a.due_at, a.assignee_id, a.occurred_at,
		       (SELECT dl.deal_id FROM activity_link dl
		         WHERE dl.activity_id = a.id AND dl.entity_type = 'deal' AND %[3]s
		         ORDER BY dl.id LIMIT 1),
		       (SELECT pl.person_id FROM activity_link pl
		         WHERE pl.activity_id = a.id AND pl.entity_type = 'person' AND %[4]s
		         ORDER BY pl.id LIMIT 1)
		FROM activity a
		WHERE a.kind = 'task' AND NOT a.is_done AND a.archived_at IS NULL AND %[1]s
		  AND %[2]s%[6]s
		ORDER BY (a.due_at IS NULL), a.due_at, a.occurred_at, a.id
		LIMIT %[5]d`,
		activityScope, activities.OrgLinkedActivityExists(orgPos), linkVisible, personVisible, limit+1,
		opts.projectScope(arg)), args...)
	if err != nil {
		return nil, crmcontracts.PageInfo{}, nil, err
	}
	var filed []time.Time
	steps, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (crmcontracts.Organization360NextStep, error) {
		var step crmcontracts.Organization360NextStep
		var id ids.UUID
		var assignee, dealID, personID *ids.UUID
		// Read only to order by: the account's task list does not show when a
		// task was filed, but two tasks due the same day have to rank the same
		// here as on the contact page, and there the older one leads.
		var occurredAt time.Time
		if err := row.Scan(&id, &step.Subject, &step.DueAt, &assignee, &occurredAt, &dealID, &personID); err != nil {
			return step, err
		}
		step.ActivityId = openapi_types.UUID(id)
		step.AssigneeId = uuidPtr(assignee)
		step.LinkedDealId = uuidPtr(dealID)
		step.LinkedPersonId = uuidPtr(personID)
		step.Overdue = deadline.Passed(step.DueAt, now)
		filed = append(filed, occurredAt)
		return step, nil
	})
	if err != nil {
		return nil, crmcontracts.PageInfo{}, nil, err
	}
	// Overdue leads by construction: the SQL orders dated before undated and
	// earliest first, and overdue is exactly "dated before now".
	steps, page := truncate(steps)
	// truncate drops the sentinel row the +1 fetched; the filing moments have
	// to lose the same one or the two slices stop lining up.
	if len(filed) > len(steps) {
		filed = filed[:len(steps)]
	}
	if steps == nil {
		steps = []crmcontracts.Organization360NextStep{}
	}
	return steps, page, filed, nil
}

func uuidPtr(id *ids.UUID) *openapi_types.UUID {
	if id == nil {
		return nil
	}
	v := openapi_types.UUID(*id)
	return &v
}
