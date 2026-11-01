// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// RedeemRecords carries the approved versions into native multi-record writers.
func (s *Service) RedeemRecords(ctx context.Context, id ids.ApprovalID, tool, diffHash string) (auth.ReleasedRecordPins, error) {
	var pins auth.ReleasedRecordPins
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		a, err := s.redeemRow(ctx, tx, id, tool, diffHash)
		if err != nil {
			return err
		}
		pins.Target = releasedRecordPin(a.TargetType, a.TargetID, a.TargetVersion)
		pins.CoTarget = releasedRecordPin(a.CoTargetType, a.CoTargetID, a.CoTargetVersion)
		if pins.Target != nil && a.ContextDigest != nil {
			pins.Target.ContextDigest = *a.ContextDigest
		}
		if a.Kind == previewMergeRecords && a.TargetType != nil && (*a.TargetType == "contact" || *a.TargetType == tableCompany) && (pins.Target == nil || pins.CoTarget == nil) {
			return &PreviewContextError{State: previewIncomplete}
		}
		return nil
	})
	if err != nil {
		return auth.ReleasedRecordPins{}, err
	}
	return pins, nil
}

func releasedRecordPin(entityType *string, entityID *ids.UUID, version *int64) *auth.ReleasedRecordPin {
	if entityType == nil || entityID == nil || version == nil {
		return nil
	}
	return &auth.ReleasedRecordPin{EntityType: *entityType, EntityID: *entityID, Version: *version}
}
