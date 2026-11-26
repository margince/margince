// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// createAnchorCompany mints the installation's own company row. Two first saves
// have no row to lock, so the uq_company_anchor index refuses the second one.
// A twin captured from mail goes to the review queue, as every create files it.
func createAnchorCompany(ctx context.Context, tx pgx.Tx, displayName, by string) (ids.CompanyID, error) {
	if err := httperr.RequireWithin(fieldDisplayName, displayName, values.TextLabelCharacters); err != nil {
		return ids.CompanyID{}, err
	}
	match, err := DedupeCompanyForCreate(ctx, tx, CompanyCandidate{DisplayName: displayName})
	if err != nil {
		return ids.CompanyID{}, err
	}
	companyID, err := createCompany(ctx, tx, match, CompanySpec{
		DisplayName: displayName,
		IsAnchor:    true,
		Source:      activitySourceManual,
		CapturedBy:  by,
	})
	if constraint, dup := storekit.UniqueViolation(err); dup && constraint == "uq_company_anchor" {
		return ids.CompanyID{}, fmt.Errorf("the company was created by someone else just now: %w", apperrors.ErrConflict)
	}
	if err != nil {
		return ids.CompanyID{}, err
	}
	if _, err := match.recordIfReview(ctx, tx, companyID, activitySourceManual, by); err != nil {
		return ids.CompanyID{}, err
	}
	return companyID, nil
}
