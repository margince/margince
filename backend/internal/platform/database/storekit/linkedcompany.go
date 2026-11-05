// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// linkedCompanyAlias names the company a LinkedCompanyColumn subquery reaches.
const linkedCompanyAlias = "linked_company"

// LinkedCompanyColumn is a column of the company reference names, NULL where the row scope
// does not reach it. Without company read it refuses; reference and column are code identifiers.
func LinkedCompanyColumn(ctx context.Context, reference, column string, arg func(any) int) (string, error) {
	if !auth.ReadGranted(ctx, "company") {
		return "", apperrors.ErrPermissionDenied
	}
	scope, err := auth.ScopeClauseFor(ctx, "company", linkedCompanyAlias, arg)
	if err != nil {
		return "", err
	}
	if scope == "" {
		scope = "TRUE"
	}
	return "(SELECT " + linkedCompanyAlias + "." + column + " FROM company " + linkedCompanyAlias +
		" WHERE " + linkedCompanyAlias + ".id = " + reference + " AND " + scope + ")", nil
}
