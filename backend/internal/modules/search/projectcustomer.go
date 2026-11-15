// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

import (
	"context"

	"github.com/margince/margince/backend/internal/platform/auth"
)

// projectCustomerColumn names a project's company_id, which is its first
// customer edge. The stored project.company_id does not track that edge.
const projectCustomerColumn = "company_id"

// referenceExpr renders one record's reference column on alias as this caller
// may read it. A project's company_id is auth.ProjectCustomerSQL's readable
// first customer, so search never names a company a project read withholds.
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
