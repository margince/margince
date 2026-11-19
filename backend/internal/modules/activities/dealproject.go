// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// An activity filed on a deal is filed on the deal's project too. The
// project's timeline then holds it, and the account's other projects leave it
// out. The rule, for every door that files on a deal:
//
//   - A project the caller named wins. Only one project link fits an activity,
//     so a deal of another project adds nothing.
//   - Deals naming two projects, or a deal with no project, add nothing.
//   - The project link is a filing, with the update grant, the attach check
//     and the correspondence stamp of any other. An agent files none
//     unattended, so its note on a deal stays off the project.
//   - A deal that later changes project moves none of its earlier activities.
//     Taking a filing back is UndoProjectFiling's alone.

import (
	"context"
	"errors"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// dealProjectLink is the project link the deals among links carry. It answers
// false when the caller may not file it or the links already name a project.
func dealProjectLink(ctx context.Context, tx pgx.Tx, links []ActivityLinkInput) (ActivityLinkInput, bool, error) {
	var deals []ids.UUID
	for _, link := range links {
		switch link.EntityType {
		case linkEntityProject:
			return ActivityLinkInput{}, false, nil
		case linkEntityDeal:
			deals = append(deals, link.EntityID)
		}
	}
	if len(deals) == 0 || refuseAnUnattendedFiling(ctx) != nil {
		return ActivityLinkInput{}, false, nil
	}
	if err := auth.Require(ctx, "activity", principal.ActionUpdate); err != nil {
		return ActivityLinkInput{}, false, skipRefusal(err)
	}
	project, err := storekit.DealsProject(ctx, tx, deals)
	if err != nil || project.IsZero() {
		return ActivityLinkInput{}, false, err
	}
	if err := auth.EnsureAttachTarget(ctx, tx, linkEntityProject, project); err != nil {
		return ActivityLinkInput{}, false, skipRefusal(err)
	}
	return ActivityLinkInput{EntityType: linkEntityProject, EntityID: project}, true, nil
}

// skipRefusal turns a refusal into "no inherited link": the caller asked to
// file on the deal, and that filing stands without the project.
func skipRefusal(err error) error {
	if errors.Is(err, apperrors.ErrPermissionDenied) || errors.Is(err, apperrors.ErrNotFound) {
		return nil
	}
	return err
}

// inheritDealProject adds the deal's project to an activity about to be
// logged. An automatic link never spends the caller's link allowance.
func inheritDealProject(ctx context.Context, tx pgx.Tx, in *LogActivityInput) error {
	if len(in.Links) >= maxActivityLinks {
		return nil
	}
	link, ok, err := dealProjectLink(ctx, tx, in.Links)
	if err != nil || !ok {
		return err
	}
	in.Links = append(slices.Clone(in.Links), link)
	return nil
}

// fileRelinkedDealProject files an activity just relinked onto a deal under
// that deal's project, unless it already has a project.
func fileRelinkedDealProject(ctx context.Context, tx pgx.Tx, id ids.ActivityID, deal ids.UUID) error {
	link, ok, err := dealProjectLink(ctx, tx, []ActivityLinkInput{{EntityType: linkEntityDeal, EntityID: deal}})
	if err != nil || !ok {
		return err
	}
	_, _, err = relinkActivityRow(ctx, tx, id, RelinkActivityInput{
		EntityType: linkEntityProject, EntityID: link.EntityID, unfiledOnly: true,
		evidence: map[string]any{provenance.ProjectRungKey: provenance.ProjectRungDeal, fieldDealID: deal},
	}, linkColumn(linkEntityProject))
	return err
}
