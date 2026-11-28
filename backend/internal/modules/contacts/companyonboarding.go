// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// OnboardingCompanyState resolves the anchor state inside the caller's
// transaction. The workspace lock serializes the no-anchor to anchor transition
// with SaveCompany, making creator/member routing one atomic decision.
func (s *Store) OnboardingCompanyState(ctx context.Context, tx pgx.Tx) (bool, bool, error) {
	if err := auth.Require(ctx, "company", principal.ActionRead); err != nil {
		return false, false, err
	}
	if err := lockCompanyState(ctx, tx); err != nil {
		return false, false, err
	}
	companyID, err := anchorCompany(ctx, tx, false)
	if errors.Is(err, apperrors.ErrNotFound) {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}
	if err := auth.EnsureVisible(ctx, tx, "company", companyID.UUID); err != nil {
		return false, false, err
	}
	company, err := readAnchorCompany(ctx, tx, companyID)
	if err != nil {
		return false, false, err
	}
	return true, company.MinimumComplete, nil
}

// lockCompanyState makes a reader of the company's onboarding state and a writer
// of its anchor company take turns for the rest of the transaction.
func lockCompanyState(ctx context.Context, tx pgx.Tx) error {
	return storekit.LockWriteIdentity(ctx, tx, "company_state", "anchor")
}
