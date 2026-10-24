// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package company360

// The open_commitments section lists the promises read out of conversations
// with this account's contacts, ours and theirs. Without it, "what is still open
// here?" could count a task and miss a promise that is just as open.
//
// Both sides come from the contacts store reads that the moment card and the
// deal page already use. Those reads decide grants, the employment edge and the
// needs_review exclusion. The "ours" read is shared with the moment card.

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/deadline"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/owedwork"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const sectionCommitments = crmcontracts.Company360SectionsOmitted("commitments")

// ourCommitments is the read of what we promised, taken once per assembly.
// A refusal is remembered with the rows, so the moment card and the section
// are refused together instead of asking twice.
func (a *assembly) ourCommitments() ([]contacts.CompanyCommitment, bool, error) {
	if !a.oursRead {
		a.ours, a.oursComplete, a.oursErr = a.svc.ourClaims(a.ctx, a.tx, a.companyID)
		a.oursRead = true
	}
	return a.ours, a.oursComplete, a.oursErr
}

// ourClaims reads the promises our side made, the same read for the page and
// for the callers that run outside an assembly.
func (s *Service) ourClaims(ctx context.Context, tx pgx.Tx, companyID ids.CompanyID) ([]contacts.CompanyCommitment, bool, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return nil, false, err
	}
	return s.contacts.OpenCommitmentsForCompany(ctx, tx, companyID.UUID, companyMomentScanCap)
}

func (a *assembly) readOpenCommitments() error {
	ours, oursComplete, err := a.ourCommitments()
	if err != nil {
		return err
	}
	theirs, theirsComplete, err := a.svc.contacts.OpenTheirCommitmentsForCompany(a.ctx, a.tx, a.companyID.UUID, companyMomentScanCap)
	if err != nil {
		return err
	}
	if !oursComplete || !theirsComplete {
		markAttentionWithheld(a.out)
	}
	data, page := commitmentRows(ours, theirs, a.now)
	a.out.OpenCommitments = &struct {
		Data []crmcontracts.Company360Commitment `json:"data"`
		Page crmcontracts.PageInfo               `json:"page"`
	}{Data: data, Page: page}
	return nil
}

// commitmentRows ranks both sides by the one rule every owed-work list uses
// (owedwork.Sorted) and cuts the page. A promise they made to us ranks beside
// ours: the reader asks what is open, not whose it is.
func commitmentRows(ours, theirs []contacts.CompanyCommitment, now time.Time) ([]crmcontracts.Company360Commitment, crmcontracts.PageInfo) {
	items := make([]owedwork.Item, 0, len(ours)+len(theirs))
	for _, side := range []struct {
		kind   crmcontracts.Company360CommitmentKind
		claims []contacts.CompanyCommitment
	}{
		{crmcontracts.Company360CommitmentKindOurs, ours},
		{crmcontracts.Company360CommitmentKindTheirs, theirs},
	} {
		for _, claim := range side.claims {
			items = append(items, owedwork.Item{
				Ref:    commitmentRow(claim, side.kind, now),
				Source: owedwork.FromClaim, DueAt: claim.DueAt, FiledAt: claim.OccurredAt,
			})
		}
	}
	rows := make([]crmcontracts.Company360Commitment, 0, len(items))
	for _, item := range owedwork.Sorted(items, now) {
		if row, ok := item.Ref.(crmcontracts.Company360Commitment); ok {
			rows = append(rows, row)
		}
	}
	return truncate(rows)
}

func commitmentRow(claim contacts.CompanyCommitment, kind crmcontracts.Company360CommitmentKind, now time.Time) crmcontracts.Company360Commitment {
	return crmcontracts.Company360Commitment{
		ClaimId:     openapi_types.UUID(claim.ID),
		ActivityId:  openapi_types.UUID(claim.ActivityID),
		Kind:        kind,
		Body:        claim.Body,
		ContactId:   openapi_types.UUID(claim.ContactID.UUID),
		ContactName: claim.ContactName,
		DueAt:       claim.DueAt,
		Overdue:     deadline.Passed(claim.DueAt, now),
	}
}
