// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// dealTopicProjectName is the SQL for a candidate's project name: the name
// where the seat may read projects and that project, else empty.
func dealTopicProjectName(ctx context.Context, arg func(any) int) (string, error) {
	if !auth.ReadGranted(ctx, "project") {
		return "''", nil
	}
	scope, err := auth.ScopeClauseFor(ctx, "project", "p", arg)
	if err != nil {
		return "", err
	}
	if scope == "" {
		scope = dealTopicUnscoped
	}
	return `CASE WHEN p.id IS NOT NULL AND (` + scope + `) THEN p.name ELSE '' END`, nil
}

// dealTopicUnscoped stands in for a scope clause a caller's grant leaves empty.
const dealTopicUnscoped = "TRUE"

// readDealTopicInput reads what the topic check shows the model, as the
// capturing seat may read it. That is the mail, and per candidate deal its
// name, its project and the subjects of the newest mail already on it. A mail
// the seat cannot read answers false; a deal it cannot read is left out.
func readDealTopicInput(ctx context.Context, tx pgx.Tx, mail ids.ActivityID, deals []ids.UUID) (dealTopicInput, bool, error) {
	var in dealTopicInput
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			return in, false, nil
		}
		return in, false, err
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	mailArg, limitArg := arg(mail), arg(dealTopicBodyLimit)
	content, err := auth.ActivityContentClause(ctx, "a", arg)
	if err != nil {
		return in, false, err
	}
	if content == "" {
		content = dealTopicUnscoped
	}
	err = tx.QueryRow(ctx, fmt.Sprintf(`SELECT coalesce(a.subject, ''), left(coalesce(a.body, ''), $%d)
		  FROM activity a WHERE a.id = $%d AND a.restricted_at IS NULL AND (%s)`, limitArg, mailArg, content), args...).
		Scan(&in.Subject, &in.Body)
	if errors.Is(err, pgx.ErrNoRows) {
		return in, false, nil
	}
	if err != nil {
		return in, false, fmt.Errorf("compose: reading the mail a deal topic check judges: %w", err)
	}
	in.Deals, err = readDealTopicDeals(ctx, tx, mail, deals)
	if err != nil {
		return in, false, err
	}
	return in, sameDeals(in.Deals, deals), nil
}

// sameDeals answers whether the read returned every deal asked about. A model
// that judged a partial set would be judging a different question.
func sameDeals(read []dealTopicDeal, asked []ids.UUID) bool {
	if len(read) != len(asked) {
		return false
	}
	for i, deal := range read {
		if deal.ID != asked[i] {
			return false
		}
	}
	return true
}

// readDealTopicDeals describes each deal the seat may read, in the order asked.
func readDealTopicDeals(ctx context.Context, tx pgx.Tx, mail ids.ActivityID, deals []ids.UUID) ([]dealTopicDeal, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	mailArg, dealsArg, recentArg := arg(mail), arg(deals), arg(dealTopicRecentSubjects)
	dealScope, err := auth.ScopeClauseFor(ctx, "deal", "d", arg)
	if err != nil {
		return nil, err
	}
	content, err := auth.ActivityContentClause(ctx, "a", arg)
	if err != nil {
		return nil, err
	}
	for _, clause := range []*string{&dealScope, &content} {
		if *clause == "" {
			*clause = dealTopicUnscoped
		}
	}
	project, err := dealTopicProjectName(ctx, arg)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, fmt.Sprintf(`SELECT d.id, d.name, `+project+`,
		       coalesce((SELECT array_agg(recent.subject) FROM (
		         SELECT a.subject FROM activity a JOIN activity_link l ON l.activity_id = a.id AND l.deal_id = d.id
		          WHERE a.id <> $%[1]d AND a.kind = 'email' AND a.subject IS NOT NULL
		            AND a.restricted_at IS NULL AND (%[5]s)
		          ORDER BY a.occurred_at DESC, a.id DESC LIMIT $%[3]d) recent), '{}')
		  FROM deal d LEFT JOIN project p ON p.id = d.project_id
		 WHERE d.id = ANY($%[2]d) AND (%[4]s)
		 ORDER BY array_position($%[2]d::uuid[], d.id)`, mailArg, dealsArg, recentArg, dealScope, content), args...)
	if err != nil {
		return nil, fmt.Errorf("compose: reading the deals a topic check weighs: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (dealTopicDeal, error) {
		var deal dealTopicDeal
		err := row.Scan(&deal.ID, &deal.Name, &deal.Project, &deal.Recent)
		return deal, err
	})
}
