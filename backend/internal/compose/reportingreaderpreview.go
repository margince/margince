// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// roleReach is identity's answer to what one role may do. A seam because
// reporting may not import identity.
type roleReach interface {
	RoleReach(ctx context.Context, role string) (principal.Permissions, error)
}

// reportingReaderPreviewHandlers evaluates a saved report as a seat holding one
// role would read it.
type reportingReaderPreviewHandlers struct {
	reports *reporting.Service
	roles   roleReach
}

// PreviewReportingReportAsRole implements
// GET /analytics/reports/{id}/reader-preview.
func (h reportingReaderPreviewHandlers) PreviewReportingReportAsRole(
	w http.ResponseWriter, r *http.Request, id crmcontracts.Id, params crmcontracts.PreviewReportingReportAsRoleParams,
) {
	ctx, err := asRole(r.Context(), h.roles, params.Role)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	evaluation, err := h.reports.EvaluateReport(ctx, ids.UUID(id), params.Revision)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, evaluation)
}

// asRole narrows the caller to what both they and the role may do. The caller
// stays the actor, so ownership and teams are theirs, and the preview cannot
// show them a record their own access withholds.
func asRole(ctx context.Context, roles roleReach, role string) (context.Context, error) {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalHuman {
		return ctx, apperrors.ErrPermissionDenied
	}
	reach, err := roles.RoleReach(ctx, role)
	if err != nil {
		return ctx, err
	}
	actor.Permissions = actor.Permissions.Within(reach)
	return principal.WithActor(ctx, actor), nil
}
