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
		// the same way a lead's is: an active human seat that can write. The FK
		// alone would admit a suspended seat or an agent.
		if err := auth.EnsureOwnerHandOn(ctx, tx, (*ids.UUID)(current.OwnerId), in.OwnerID); err != nil {
			return err
		}
		p.Set(ownerColumn, current.OwnerId, *in.OwnerID)
	}
	if in.ProjectID != nil {
		if err := ensureProjectAttachable(ctx, tx, in.ProjectID.UUID); err != nil {
			return auth.NameMissingReference("project_id", err)
		}
		p.Set("project_id", current.ProjectId, *in.ProjectID)
	}
	return applyPartnerAttributionPatch(ctx, tx, current, in, p, clearPartner, ensurePartner)
}

// applyCompanyLinkPatch points a deal at a company the caller may see and
// that the deal's contracts name (contractseam.go). Every caller of
// updateDealInTx reaches it: REST, the agent tools, the CSV import, restores.
//
// Clearing the company stays allowed. The filing check admits a contract on a
// deal with no company, and a deal's row scope does not read its company.
func applyCompanyLinkPatch(ctx context.Context, tx pgx.Tx, current crmcontracts.Deal,
	companyID ids.CompanyID, p *storekit.Patch, ensureContractsShareCompany EnsureContractsShareCompany,
) error {
	if err := auth.EnsureReferenceTarget(ctx, tx, "company_id", "company", companyID.UUID); err != nil {
		return err
	}
	// Resending the company a deal already names moves nothing, so an older
	// mismatch does not refuse an edit of some other field.
	unchanged := current.CompanyId != nil && ids.UUID(*current.CompanyId) == companyID.UUID
	if !unchanged {
		if err := ensureContractsShareCompany(ctx, tx, ids.From[ids.DealKind](ids.UUID(current.Id)), companyID); err != nil {
			return err
		}
	}
	p.Set("company_id", current.CompanyId, companyID)
	return nil
}
