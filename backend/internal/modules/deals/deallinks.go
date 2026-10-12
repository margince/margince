// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// applyDealLinkPatches sets the fields that point at another record. They are
// grouped because they share an obligation the plain columns do not: a link is
// only settable to a target the caller may see, so each one gates before it
// patches (auth.EnsureLinkTarget), and a miss reads as not-found rather than
// disclosing that the row exists.
//
// The project pointer is the exception, and it needs WRITE authority
// (ensureProjectAttachable). Pointing a deal at a project is not a read of the
// project: winning that deal advances the project's phase and writes its
// history (startDeliveryForWonDeal), and that advance deliberately does not
// re-check the caller's authority over the project — the authority to attach
// is what stands in for it. A visibility-only gate here would let any seat
// attach any project in the workspace and then force it into `delivering`.
func applyDealLinkPatches(ctx context.Context, tx pgx.Tx,
	current crmcontracts.Deal, in UpdateDealInput, p *storekit.Patch, clearPartner bool,
	ensurePartner EnsurePartner, ensureProjectAttachable EnsureProjectAttachable,
	ensureContractsShareCompany EnsureContractsShareCompany,
) error {
	if in.CompanyID != nil {
		if err := applyCompanyLinkPatch(ctx, tx, current, *in.CompanyID, p, ensureContractsShareCompany); err != nil {
			return err
		}
	}
	if in.OwnerID != nil {
		// Handing the deal on is an assignment, and the destination is checked
		// the same way a lead's is: an active human seat inside the caller's
		// own write scope. Without it a deal could be handed to a suspended
		// seat or out of the assigner's reach, which the FK alone does not
		// refuse.
		if err := auth.EnsureOwnerHandOn(ctx, tx, (*ids.UUID)(current.OwnerId), in.OwnerID); err != nil {
			return err
		}
		p.Set(ownerColumn, current.OwnerId, *in.OwnerID)
	}
	if in.ProjectID != nil {
		if err := ensureProjectAttachable(ctx, tx, in.ProjectID.UUID); err != nil {
			return err
		}
		p.Set("project_id", current.ProjectId, *in.ProjectID)
	}
	return applyPartnerAttributionPatch(ctx, tx, current, in, p, clearPartner, ensurePartner)
}

// applyCompanyLinkPatch re-points a deal at another company: a read of that
// company, and a question about the agreements already filed against the deal.
//
// A contract with a deal is visible through that deal alone. So a deal moved to
// company B would publish company A's agreements to everyone who can see it.
//
// The deal row is held before the agreements are read. A contract filed against
// this deal at the same time takes a share lock on that row. The second
// transaction waits and reads what the first committed.
//
// The hold comes after the company link probe, which locks `company`. Every
// other deal write takes the company first and the deal second.
//
// Clearing the company stays on the clear path. A deal naming nobody shows its
// agreements only to readers its own scope already admits.
func applyCompanyLinkPatch(ctx context.Context, tx pgx.Tx, current crmcontracts.Deal,
	companyID ids.CompanyID, p *storekit.Patch,
	ensureContractsShareCompany EnsureContractsShareCompany,
) error {
	if err := auth.EnsureLinkTarget(ctx, tx, "company", companyID.UUID); err != nil {
		return err
	}
	dealID := ids.From[ids.DealKind](ids.UUID(current.Id))
	if err := auth.HoldWritableLive(ctx, tx, dealTable, dealID.UUID); err != nil {
		return err
	}
	if err := ensureContractsShareCompany(ctx, tx, dealID, companyID); err != nil {
		return err
	}
	p.Set("company_id", current.CompanyId, companyID)
	return nil
}
