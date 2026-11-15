// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

// A project's companies are relationship edges, not its stored company_id
// column, which does not follow them. A filter or hop by company matches any
// live edge the caller may read, the rule the project list's own company
// filter keeps. A hit's excerpt names the company a project read names.

import (
	"context"
	"fmt"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// projectCompanyColumn is the project field and the relation a company filter
// or hop reaches a project's companies through.
const projectCompanyColumn = "company_id"

// isProjectCompany says whether entity.column is a project's company.
func isProjectCompany(entity, column string) bool {
	return entity == entityProject && column == projectCompanyColumn
}

// projectCompanyClause renders a predicate on a project's company_id as a test
// of its readable live company edges. A caller who may read no edge matches
// nothing, whatever the operator, so no answer depends on a withheld pair.
func (c *planCompiler) projectCompanyClause(
	ctx context.Context, alias, at string, field Field, clause Predicate,
) (string, *apperrors.FieldRefusal, error) {
	edges, err := auth.ProjectCompanyEdgesSQL(ctx, c.arg)
	if err != nil || !edges.Readable {
		return "false", nil, err
	}
	matching, negate := clause, clause.Op == OpNeq
	if negate {
		matching.Op = OpEq
	}
	inner, refusal := c.clause(auth.ProjectEdgeCompany, at, field, matching)
	if refusal != nil {
		return "", refusal, nil
	}
	exists := fmt.Sprintf("EXISTS (SELECT 1 %s AND %s = %s.id AND %s)", edges.Source, auth.ProjectEdgeProject, alias, inner)
	if negate {
		return "NOT " + exists, nil, nil
	}
	return exists, nil, nil
}

// projectCompanyHop joins a project and a company on any live edge the caller
// may read. project and company are the two records' id columns.
func (c *planCompiler) projectCompanyHop(ctx context.Context, project, company string) (string, error) {
	edges, err := auth.ProjectCompanyEdgesSQL(ctx, c.arg)
	if err != nil || !edges.Readable {
		return "false", err
	}
	return fmt.Sprintf("EXISTS (SELECT 1 %s AND %s = %s AND %s = %s)",
		edges.Source, auth.ProjectEdgeProject, project, auth.ProjectEdgeCompany, company), nil
}
