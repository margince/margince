// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// StagePrecheck validates an agent call without committing domain changes.
type StagePrecheck func(context.Context, StageInput) error

// WithStagePrecheck uses a kind’s writer checks before staging or deciding.
func (s *Service) WithStagePrecheck(kind string, check StagePrecheck) *Service {
	s.stagePrechecks[kind] = check
	return s
}

// WithStageAdmission applies shared scope and seat checks to staged calls.
func (s *Service) WithStageAdmission(check StagePrecheck) *Service {
	s.stageAdmission = check
	return s
}

func (s *Service) checkStageContext(ctx context.Context, in StageInput) error {
	if s.stageAdmission != nil {
		if err := s.stageAdmission(ctx, in); err != nil {
			return err
		}
	}
	if check := s.stagePrechecks[in.Kind]; check != nil {
		return check(ctx, in)
	}
	return nil
}

func stageAuthorityRow(in StageInput) row {
	return row{
		Kind: in.Kind, TargetType: nullStr(in.TargetType), TargetID: nullUUID(in.TargetID),
		CoTargetType: nullStr(in.CoTargetType), CoTargetID: nullUUID(in.CoTargetID), ProposedChange: in.ProposedChange,
	}
}

func requireCallAuthority(ctx context.Context, tx pgx.Tx, p principal.Principal, proposal row) error {
	if err := requireDecisionGrants(p, proposal); err != nil {
		return err
	}
	permitted, err := authorityRows(ctx, tx, p, []row{proposal})
	if err != nil {
		return err
	}
	if !permitted[0] {
		return apperrors.ErrPermissionDenied
	}
	return nil
}

func (s *Service) precheckAgentCall(ctx context.Context, proposal row) error {
	var actor principal.Principal
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		actor, err = s.agentCallAuthority(ctx, tx, proposal)
		return err
	})
	if err != nil {
		return err
	}
	return s.checkStageContext(principal.WithActor(ctx, actor), stageInputFromRow(proposal))
}

func stageInputFromRow(proposal row) StageInput {
	in := StageInput{Kind: proposal.Kind, ProposedChange: proposal.ProposedChange}
	if proposal.TargetType != nil {
		in.TargetType = *proposal.TargetType
	}
	if proposal.TargetID != nil {
		in.TargetID = *proposal.TargetID
	}
	if proposal.CoTargetType != nil {
		in.CoTargetType = *proposal.CoTargetType
	}
	if proposal.CoTargetID != nil {
		in.CoTargetID = *proposal.CoTargetID
	}
	return in
}

func (s *Service) agentCallAuthority(ctx context.Context, tx pgx.Tx, proposal row) (principal.Principal, error) {
	if s.effectActor == nil {
		return principal.Principal{}, apperrors.ErrPermissionDenied
	}
	effect, err := s.effectContext(ctx, tx, proposal.Kind, stagedByRow(proposal))
	if err != nil {
		return principal.Principal{}, err
	}
	actor, ok := principal.Actor(effect)
	if !ok {
		return principal.Principal{}, apperrors.ErrPermissionDenied
	}
	if err := requireCallAuthority(effect, tx, actor, proposal); err != nil {
		return principal.Principal{}, err
	}
	if s.stageAdmission != nil {
		if err := s.stageAdmission(effect, stageInputFromRow(proposal)); err != nil {
			return principal.Principal{}, err
		}
	}
	return actor, nil
}

func agentCallNeedsRecheck(proposal row) bool {
	return !serverProposed(proposal) && proposal.Kind != KindVolumeRelease
}
