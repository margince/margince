// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type reportingAuthority struct{ users *identity.Service }

func accountableHuman(ctx context.Context, users *identity.Service, workspace, user ids.UUID) (context.Context, error) {
	rbac, seat, err := users.EffectiveAuthority(ctx, workspace, user)
	if err != nil {
		return nil, err
	}
	human := principal.WithActor(ctx, principal.Principal{Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user, SeatType: seat, TeamIDs: rbac.TeamIDs, Permissions: rbac.Permissions})
	return principal.WithCorrelationID(human, ids.NewV7()), nil
}

func reportingRequested(scope crmcontracts.ReportingScope) RequestedScope {
	// My teams is a live, deduplicated default, never a caller-selected union.
	if scope.Kind == ScopeKindManagedTeams {
		return RequestedScope{}
	}
	return RequestedScope{Kind: string(scope.Kind), ID: (*ids.UUID)(scope.Id)}
}

func (a reportingAuthority) Scope(ctx context.Context, tx pgx.Tx, scope crmcontracts.ReportingScope, write bool) (crmcontracts.ReportingScope, error) {
	resolved, err := ResolveAnalyticsScope(ctx, tx, reportingRequested(scope))
	if err != nil {
		return crmcontracts.ReportingScope{}, err
	}
	if scope.Kind == ScopeKindManagedTeams && resolved.Kind != ScopeKindManagedTeams {
		return crmcontracts.ReportingScope{}, apperrors.ErrPermissionDenied
	}
	if write {
		if err := reportingWriteReach(ctx, tx, resolved); err != nil {
			return crmcontracts.ReportingScope{}, err
		}
	}

	return crmcontracts.ReportingScope{Kind: crmcontracts.ReportingScopeKind(resolved.Kind), Id: (*openapi_types.UUID)(resolved.ID), Label: &resolved.Label}, nil
}

func (a reportingAuthority) Members(ctx context.Context, tx pgx.Tx) ([]ids.UUID, error) {
	return reportingMembers(ctx, tx, crmcontracts.ReportingScope{})
}

func reportingMembers(ctx context.Context, tx pgx.Tx, scope crmcontracts.ReportingScope) ([]ids.UUID, error) {
	var b reportingBindings
	_, clause, err := analyticsPopulationExpression(ctx, tx, reportingRequested(scope), "u.id", b.arg, unownedIsExcluded)
	if err != nil {
		return nil, err
	}
	if clause == "" {
		clause = sqlUnnarrowed
	}
	rows, err := tx.Query(ctx, "SELECT u.id FROM app_user u WHERE "+identity.LiveMemberSQL("u")+" AND NOT u.is_agent AND "+clause+" ORDER BY u.id", b.values...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
}

func (a reportingAuthority) ValidateFramework(ctx context.Context, tx pgx.Tx, in crmcontracts.ReportingFrameworkInput) error {
	pipelines := map[openapi_types.UUID]bool{}
	for _, qualification := range in.Qualification {
		if pipelines[qualification.PipelineId] || len(qualification.StageIds) == 0 {
			return fmt.Errorf("choose distinct pipelines and at least one qualifying stage: %w", apperrors.ErrInvalidArgument)
		}
		pipelines[qualification.PipelineId] = true
		if err := reportingPipeline(ctx, tx, &qualification.PipelineId); err != nil {
			return err
		}
		stages := map[openapi_types.UUID]bool{}
		for _, stage := range qualification.StageIds {
			if stages[stage] {
				return apperrors.ErrInvalidArgument
			}
			stages[stage] = true
			var b reportingBindings
			var valid bool
			err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM stage WHERE id="+b.add(stage)+" AND pipeline_id="+b.add(qualification.PipelineId)+" AND archived_at IS NULL)", b.values...).Scan(&valid)
			if err != nil {
				return err
			}
			if !valid {
				return apperrors.ErrNotFound
			}
		}
	}
	contexts := map[string]bool{}
	for _, capture := range in.CaptureContexts {
		if capture.Scope.Kind != "team" && capture.Scope.Kind != ScopeKindWorkspace {
			return fmt.Errorf("capture a fixed team or company scope: %w", apperrors.ErrInvalidArgument)
		}
		if _, err := a.Scope(ctx, tx, capture.Scope, true); err != nil {
			return err
		}
		if err := reportingPipeline(ctx, tx, capture.PipelineId); err != nil {
			return err
		}
		key := reportingCaptureKey(capture)
		if contexts[key] {
			return fmt.Errorf("choose each capture context once: %w", apperrors.ErrInvalidArgument)
		}
		contexts[key] = true
	}
	return nil
}

func reportingPipeline(ctx context.Context, tx pgx.Tx, id *openapi_types.UUID) error {
	if id == nil {
		return nil
	}
	if err := auth.Require(ctx, "pipeline", principal.ActionRead); err != nil {
		return err
	}
	_, err := deals.ReadPipelineTx(ctx, tx, ids.From[ids.PipelineKind](ids.UUID(*id)))
	return err
}

func (reportingAuthority) Pipeline(ctx context.Context, tx pgx.Tx, id *ids.UUID) error {
	return reportingPipeline(ctx, tx, (*openapi_types.UUID)(id))
}

func (a reportingAuthority) PublicationHuman(ctx context.Context, tx pgx.Tx, user ids.UUID) (context.Context, error) {
	rbac, seat, err := a.users.PublicationAuthority(ctx, tx, user)
	if err != nil {
		return nil, err
	}
	human := principal.WithActor(ctx, principal.Principal{Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user, SeatType: seat, TeamIDs: rbac.TeamIDs, Permissions: rbac.Permissions})
	return principal.WithCorrelationID(human, ids.NewV7()), nil
}

func (a reportingAuthority) MembersFor(ctx context.Context, tx pgx.Tx, scope crmcontracts.ReportingScope) ([]ids.UUID, error) {
	return reportingMembers(ctx, tx, scope)
}

func reportingWriteReach(ctx context.Context, tx pgx.Tx, scope ResolvedScope) error {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalHuman || !actor.SeatType.CanMutate() {
		return apperrors.ErrPermissionDenied
	}
	if scope.Kind == ScopeKindOwner && scope.ID != nil && *scope.ID == actor.UserID {
		return nil
	}
	reach := auth.TeamWeekReachOf(ctx)
	if scope.Kind == ScopeKindManagedTeams || reach == auth.ReachesNoTeam {
		return apperrors.ErrPermissionDenied
	}
	if reach == auth.ReachesEveryTeam {
		return nil
	}
	if scope.ID == nil {
		return apperrors.ErrPermissionDenied
	}
	return reportingTeamWriteReach(ctx, tx, scope)
}

func reportingTeamWriteReach(ctx context.Context, tx pgx.Tx, scope ResolvedScope) error {
	var allowed bool
	var err error
	switch scope.Kind {
	case ScopeKindTeam:
		allowed, err = identity.CallerLeadsLiveTeamTx(ctx, tx, *scope.ID)
	case ScopeKindOwner:
		allowed, err = identity.SharesLiveTeamWithCallerTx(ctx, tx, ids.From[ids.UserKind](*scope.ID))
	default:
		return apperrors.ErrPermissionDenied
	}
	if err != nil {
		return err
	}
	if !allowed {
		return apperrors.ErrNotFound
	}
	return nil
}
