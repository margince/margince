// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/approvalplan"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// PlanSight reads a plan's footprint and checks a reader against it. An
// approval no longer pending then keeps the sight and mask rules of a pending one.
type PlanSight struct {
	// Footprint builds the plan as a pending read does and keeps what it
	// touched; nil when the build proves nothing a reader could clear.
	Footprint func(context.Context, crmcontracts.Approval) (*approvalplan.Footprint, error)
	// Clears answers whether the caller opens every record and sees every
	// field the footprint names. A nil footprint is one no writer stored.
	Clears func(context.Context, crmcontracts.Approval, *approvalplan.Footprint) (bool, error)
}

// WithPlanSight connects decided and expired approvals to the readers of the
// records their plan touched.
func (s *Service) WithPlanSight(sight PlanSight) *Service {
	s.planSight = sight
	return s
}

// planFootprint builds the footprint of a's plan, with edited in place of the
// staged call when the decision carries an edit.
func (s *Service) planFootprint(ctx context.Context, a row, edited json.RawMessage) (*approvalplan.Footprint, error) {
	proposal, err := editedProposal(a, edited, s.now())
	if err != nil {
		return nil, err
	}
	return s.planSight.Footprint(s.withPreviewEffect(ctx, a), proposal)
}

// footprintToStore is the footprint a decision or an expiry of id writes,
// built before its transaction because the build opens its own.
func (s *Service) footprintToStore(ctx context.Context, id ids.ApprovalID, edited json.RawMessage) (json.RawMessage, error) {
	if s.planSight.Footprint == nil {
		return nil, nil
	}
	var a row
	err := s.db.Tx(ctx, func(tx pgx.Tx) (err error) {
		a, err = get(ctx, tx, id)
		return err
	})
	if errors.Is(err, apperrors.ErrNotFound) {
		// The writer refuses a missing row in its own words.
		return nil, nil
	}
	if err != nil || a.Status != statusPending {
		return nil, err
	}
	return s.encodedFootprint(ctx, a, edited), nil
}

// bundleFootprints is footprintToStore for every pending member of a bundle.
func (s *Service) bundleFootprints(ctx context.Context, bundleID ids.UUID) (map[ids.ApprovalID]json.RawMessage, error) {
	out := map[ids.ApprovalID]json.RawMessage{}
	if s.planSight.Footprint == nil {
		return out, nil
	}
	var members []row
	err := s.db.Tx(ctx, func(tx pgx.Tx) (err error) {
		members, _, err = bundleMembers(ctx, tx, bundleID, false)
		return err
	})
	if err != nil {
		return nil, err
	}
	for _, a := range members {
		if a.Status == statusPending {
			out[a.ID] = s.encodedFootprint(ctx, a, nil)
		}
	}
	return out, nil
}

// encodedFootprint answers nil when the plan cannot be built, so the row reads
// as metadata only rather than blocking the verdict on a preview outage.
func (s *Service) encodedFootprint(ctx context.Context, a row, edited json.RawMessage) json.RawMessage {
	footprint, err := s.planFootprint(ctx, a, edited)
	if err != nil {
		s.logger().WarnContext(ctx, "approval plan footprint not built; the approval will show metadata only", "approval", a.ID, "err", err)
		return nil
	}
	if footprint == nil {
		return nil
	}
	encoded, err := json.Marshal(footprint)
	if err != nil {
		s.logger().WarnContext(ctx, "approval plan footprint not encoded; the approval will show metadata only", "approval", a.ID, "err", err)
		return nil
	}
	return encoded
}

// pastPlanHidden answers whether the plan of an approval no longer pending
// stays behind for the caller. A row still stored pending past its expiry is
// footprinted live; a decided one is read by what its writer stored.
func (s *Service) pastPlanHidden(ctx context.Context, approval crmcontracts.Approval, subject row) (bool, error) {
	if s.planSight.Clears == nil {
		return false, nil
	}
	footprint, err := s.pastFootprint(ctx, subject)
	if err != nil {
		return false, err
	}
	if footprint == nil && s.runsAsAgent(stagedByRow(subject)) {
		return true, nil
	}
	clears, err := s.planSight.Clears(ctx, approval, footprint)
	return !clears, err
}

func (s *Service) pastFootprint(ctx context.Context, subject row) (*approvalplan.Footprint, error) {
	if subject.Status == statusPending && s.planSight.Footprint != nil {
		return s.planFootprint(ctx, subject, nil)
	}
	if len(subject.PlanFootprint) == 0 {
		return nil, nil
	}
	var footprint approvalplan.Footprint
	if json.Unmarshal(subject.PlanFootprint, &footprint) != nil {
		// A footprint that does not decode proves nothing, as a missing one.
		return nil, nil
	}
	return &footprint, nil
}

// withheldPlan is the preview a reader gets for a past plan hidden from them.
// Every surface then words it as it words a pending one.
func withheldPlan() *crmcontracts.ApprovalPreview {
	return &crmcontracts.ApprovalPreview{Version: 1, State: PreviewNeedsWiderSight, Changes: []crmcontracts.ApprovalPreviewChange{}}
}
