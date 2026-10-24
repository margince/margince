// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// The partner set-up command: starting a partner program on a company, with no
// margin tier. The tier is the commission rate, so it has no door here.

import (
	"context"
	"fmt"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/baselanguage"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// SetUpPartnerCommand is one partner set-up, whichever door asked for it.
type SetUpPartnerCommand struct {
	CompanyID ids.UUID
	Role      string
}

// NewSetUpPartnerCall binds one set-up to the resolver that answers for it.
//
//nolint:ireturn // the call IS the product: a resolver named concretely here is exactly the thing that must not leave this package
func NewSetUpPartnerCall(records datasource.SystemOfRecordProvider, partners PartnerSetUp, language baselanguage.Resolver, cmd SetUpPartnerCommand) GovernedCall {
	return bind[SetUpPartnerCommand](&setUpPartnerResolver{
		partners: partners,
		language: language,
		company:  anchoredRecord{records: records, entityType: datasource.EntityCompany},
	}, cmd)
}

type setUpPartnerResolver struct {
	company  anchoredRecord
	partners PartnerSetUp
	language baselanguage.Resolver
}

// Subject names the company, pinned at the version the human is shown.
func (r *setUpPartnerResolver) Subject(ctx context.Context, cmd SetUpPartnerCommand) (StageInfo, error) {
	rec, err := r.company.row(ctx, cmd.CompanyID)
	if err != nil {
		return StageInfo{}, err
	}
	return StageInfo{
		TargetType:    string(datasource.EntityCompany),
		TargetID:      cmd.CompanyID,
		TargetVersion: &rec.Version,
		Summary:       fmt.Sprintf(summaryIn(ctx, r.language).setUpPartner, recordLabel(rec), cmd.Role),
	}, nil
}

// Guards refuses before anything is staged what the write would refuse
// afterwards: a company the caller cannot see or whose record lives in another
// system, a missing grant, a company the caller cannot write, and a company that
// already has a live partner program.
func (r *setUpPartnerResolver) Guards(ctx context.Context, cmd SetUpPartnerCommand) error {
	if err := r.company.refuse(ctx, cmd.CompanyID); err != nil {
		return err
	}
	return r.partners.Check(ctx, cmd.CompanyID)
}
