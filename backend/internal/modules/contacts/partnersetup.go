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

func (e *PartnerAlreadySetUpError) Error() string { return partnerRowExistsMessage() }

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
	if err := requirePartnerWriteGrant(ctx); err != nil {
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

// requirePartnerWriteGrant holds the grant every partner write takes. Promotion
// adds `partner` to the company's relationship types, so a program is part of
// the company and whoever may edit the company may run it.
func requirePartnerWriteGrant(ctx context.Context) error {
	return auth.Require(ctx, "company", principal.ActionUpdate)
}

// requireMarginTierGrant holds the partner grant on a write that sets or clears
// the margin tier, because the tier prices every later commission. It reads the
// clears the caller sent, before a set-up adds its own.
func requireMarginTierGrant(ctx context.Context, in UpsertPartnerInput) error {
	if in.MarginTier == nil && !slices.Contains(in.Cleared, partnerFieldMarginTier) {
		return nil
	}
	return auth.Require(ctx, "partner", principal.ActionUpdate)
}

// asFreshProgram makes a set-up start from the column defaults even when it
// revives an archived row. That means no margin tier, no gate metrics, no
// served segments and certification "applied". Terms the caller sent stay.
// The rest must not carry over from a program a human once approved.
func asFreshProgram(in UpsertPartnerInput) UpsertPartnerInput {
	cleared := slices.Clone(in.Cleared)
	cleared = append(cleared, partnerFieldMarginTier)
	if in.CertifiedStaff == nil && in.RetentionRate == nil {
		cleared = append(cleared, "gate_metrics")
	}
	if in.ServedSegments == nil {
		cleared = append(cleared, "served_segments")
	}
	if in.NextStep == nil {
		cleared = append(cleared, "next_step")
	}
	if in.NextStepDueAt == nil {
		cleared = append(cleared, partnerFieldNextStepDue)
	}
	in.Cleared = cleared
	if in.CertStatus == nil {
		in.CertStatus = new("applied")
	}
	if in.RelationshipStage == nil {
		in.RelationshipStage = new("research")
	}
	return in
}
