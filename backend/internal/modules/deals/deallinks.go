// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
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
) error {
	if in.CompanyID != nil {
		if err := auth.EnsureLinkTarget(ctx, tx, "company", in.CompanyID.UUID); err != nil {
			return err
		}
		p.Set("company_id", current.CompanyId, *in.CompanyID)
	}
	if in.OwnerID != nil {
		// A named owner is an assignment, and the destination is checked the
		// same way a lead's is: an active human seat inside the caller's own
		// write scope. Without it a deal could be handed to a suspended seat
		// or out of the assigner's reach, which the FK alone does not refuse.
		if err := auth.EnsureAssignee(ctx, tx, in.OwnerID.UUID); err != nil {
			return err
		}
		p.Set("owner_id", current.OwnerId, *in.OwnerID)
	}
	if in.ProjectID != nil {
		if err := ensureProjectAttachable(ctx, tx, in.ProjectID.UUID); err != nil {
			return err
		}
		p.Set("project_id", current.ProjectId, *in.ProjectID)
	}
	return applyPartnerAttributionPatch(ctx, tx, current, in, p, clearPartner, ensurePartner)
}
