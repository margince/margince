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
		pins, err = releasedPinsOf(a.Kind, pinnedRow{a.TargetType, a.TargetID, a.TargetVersion},
			pinnedRow{a.CoTargetType, a.CoTargetID, a.CoTargetVersion}, a.ContextDigest)
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return auth.ReleasedRecordPins{}, err
	}
	return pins, nil
}

// pinnedRow is one row a release rests on, as staged or as a confirmation pinned it.
type pinnedRow struct {
	entityType *string
	entityID   *ids.UUID
	version    *int64
}

func (r pinnedRow) released() *auth.ReleasedRecordPin {
	if r.entityType == nil || r.entityID == nil || r.version == nil {
		return nil
	}
	return &auth.ReleasedRecordPin{EntityType: *r.entityType, EntityID: *r.entityID, Version: *r.version}
}

// releasedPinsOf is the pins a release hands its writers, from a redeemed
// approval or a call confirmed in the conversation alike.
func releasedPinsOf(kind string, target, coTarget pinnedRow, contextDigest *string) (auth.ReleasedRecordPins, error) {
	pins := auth.ReleasedRecordPins{Target: target.released(), CoTarget: coTarget.released()}
	if pins.Target != nil && contextDigest != nil {
		pins.Target.ContextDigest = *contextDigest
	}
	if kind == previewMergeRecords && target.entityType != nil && (*target.entityType == "contact" || *target.entityType == tableCompany) && (pins.Target == nil || pins.CoTarget == nil) {
		return auth.ReleasedRecordPins{}, &PreviewContextError{State: previewIncomplete}
	}
	return pins, nil
}
