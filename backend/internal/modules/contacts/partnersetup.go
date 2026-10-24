// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Setting up a partner program without a margin tier: the write an AI
// assistant may ask for. The tier decides commission on every later deal, so
// it stays a human's choice on the company's Partner tab.

import (
	"context"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// PartnerAlreadySetUpError refuses a set-up on a company whose partner program
// is live. It answers 409 with the code partner_row_exists.
type PartnerAlreadySetUpError struct{}

func (e *PartnerAlreadySetUpError) Error() string { return partnerRowExistsConflictMessage() }

func (e *PartnerAlreadySetUpError) Unwrap() error { return apperrors.ErrConflict }

// MessageFault names the contract's code for a live program.
func (e *PartnerAlreadySetUpError) MessageFault() (code, message string) {
	return codePartnerRowExists, e.Error()
}

// CheckPartnerSetUp answers whether the caller could set up a partner program
// on the company now. The agent path asks it before staging an approval.
// UpsertPartner asks it again under the company lock. A program set up by a
// human while the approval waited is then refused at the write.
func (s *Store) CheckPartnerSetUp(ctx context.Context, companyID ids.CompanyID) error {
	return s.tx(ctx, func(tx pgx.Tx) error {
		return checkPartnerSetUp(ctx, tx, companyID)
	})
}

func checkPartnerSetUp(ctx context.Context, tx pgx.Tx, companyID ids.CompanyID) error {
	if err := requirePartnerWriteGrants(ctx); err != nil {
		return err
	}
	if err := auth.EnsureWritable(ctx, tx, "company", companyID.UUID); err != nil {
		return err
	}
	live, err := hasPartnerRow(ctx, tx, companyID)
	if err != nil {
		return err
	}
	if live {
		return &PartnerAlreadySetUpError{}
	}
	return nil
}

// requirePartnerWriteGrants holds the two object grants a partner write takes.
// Promotion adds `partner` to the company's relationship types, so the partner
// grant alone must not become a side door onto companies.
func requirePartnerWriteGrants(ctx context.Context) error {
	if err := auth.Require(ctx, "partner", principal.ActionUpdate); err != nil {
		return err
	}
	return auth.Require(ctx, "company", principal.ActionUpdate)
}

// withoutMarginTier appends the margin tier to the fields the write sets to
// NULL, after the caller's mask has been applied. A revived archived row then
// starts with no tier.
func withoutMarginTier(cleared []string) []string {
	if slices.Contains(cleared, partnerFieldMarginTier) {
		return cleared
	}
	return append(slices.Clone(cleared), partnerFieldMarginTier)
}
