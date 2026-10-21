// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const (
	mergeReadCompany      = "company"
	mergeReadDeal         = "deal"
	mergeReadProject      = "project"
	mergeReadContract     = "contract"
	mergeReadOffer        = "offer"
	mergeReadCommission   = "commission"
	mergeReadFinance      = "finance"
	mergeReadIntegrations = "integrations"
)

// Grants precede the census so a missing grant cannot reveal whether a hidden row exists.
func requireCompanyMergeReads(ctx context.Context) error {
	if err := auth.Require(ctx, mergeReadCompany, principal.ActionRead); err != nil {
		return err
	}
	if err := auth.Require(ctx, mergeReadDeal, principal.ActionRead); err != nil {
		return err
	}
	if err := auth.Require(ctx, mergeReadProject, principal.ActionRead); err != nil {
		return err
	}
	if err := auth.Require(ctx, mergeReadContract, principal.ActionRead); err != nil {
		return err
	}
	if err := auth.Require(ctx, mergeReadOffer, principal.ActionRead); err != nil {
		return err
	}
	if err := auth.Require(ctx, mergeReadCommission, principal.ActionRead); err != nil {
		return err
	}
	if err := auth.Require(ctx, mergeReadFinance, principal.ActionRead); err != nil {
		return err
	}
	if err := auth.Require(ctx, mergeReadIntegrations, principal.ActionRead); err != nil {
		return err
	}
	return nil
}
