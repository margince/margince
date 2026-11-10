// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"

	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/workarea"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

func stagedCallSpec(kind string) (mcp.ToolSpec, error) {
	spec := mcp.ToolSpec{Name: kind, Areas: workarea.OfVerb(kind)}
	for _, policy := range agentPolicies {
		if policy.Tool != kind || policy.Scope == "" {
			continue
		}
		scope := principal.Scope(policy.Scope)
		if spec.RequiredScope != "" && spec.RequiredScope != scope {
			return spec, fmt.Errorf("approval scope differs between routes for %s", kind)
		}
		spec.RequiredScope = scope
	}
	if extension, found := approvals.RegisteredExtensionKind(kind); found {
		spec.RequiredScope = extension.RequiredScope
	}
	if spec.RequiredScope == "" {
		return spec, apperrors.ErrPermissionDenied
	}
	return spec, nil
}

func stageCallAdmission(ctx context.Context, in approvals.StageInput) error {
	actor, ok := principal.Actor(ctx)
	if !ok {
		return apperrors.ErrPermissionDenied
	}
	if actor.Type != principal.PrincipalAgent {
		return nil
	}
	spec, err := stagedCallSpec(in.Kind)
	if err != nil {
		return err
	}
	if len(spec.Areas) > 1 {
		spec.Areas = nil
		if area, placed := workarea.OfRecordCall(in.Kind, in.TargetType); placed {
			spec.Areas = []string{area}
		}
	}
	if err := auth.RequireToolScope(actor, spec); err != nil {
		return err
	}
	if err := auth.RequireToolSeat(actor, spec); err != nil {
		return err
	}
	return auth.AreaRefusal(actor, spec)
}
