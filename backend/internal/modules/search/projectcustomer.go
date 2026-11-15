// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

import (
	"context"

	"github.com/margince/margince/backend/internal/platform/auth"
)

// projectCustomerColumn is the project reference that is not its stored
// column: a project's company_id is its first customer edge, which the
// project_company relationship rows hold and project.company_id does not track.
const projectCustomerColumn = "company_id"

// referenceExpr renders one record's reference column on alias as this caller
// may read it. A project's company_id is the readable first customer edge
// (auth.ProjectCustomerSQL), so a filter, a hop and an excerpt answer the
// company a project read names and never one it withholds.
func referenceExpr(ctx context.Context, entity, column, alias string, arg func(any) int) (string, error) {
	if entity != entityProject || column != projectCustomerColumn {
		return alias + "." + sanitize(column), nil
	}
	customer, err := auth.ProjectCustomerSQL(ctx, alias+".id", arg)
	if err != nil {
		return "", err
	}
	return customer.Readable, nil
}
