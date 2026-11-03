// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"context"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ReleasedRecordPin binds approval release to one reviewed record version.
type ReleasedRecordPin struct {
	EntityType string
	EntityID   ids.UUID
	Version    int64
	// ContextDigest pins facts outside the row that the reviewed outcome read.
	// Empty for a kind that registers no context pin.
	ContextDigest string
}

// ReleasedRecordPins keeps target and co-target versions together across redemption.
type ReleasedRecordPins struct {
	Target   *ReleasedRecordPin
	CoTarget *ReleasedRecordPin
}

type releasedRecordPinsKey struct{}

// WithReleasedRecordPins carries copied pins to native writers so they can check under their row locks.
func WithReleasedRecordPins(ctx context.Context, pins ReleasedRecordPins) context.Context {
	if pins.Target != nil {
		pin := *pins.Target
		pins.Target = &pin
	}
	if pins.CoTarget != nil {
		pin := *pins.CoTarget
		pins.CoTarget = &pin
	}
	return context.WithValue(ctx, releasedRecordPinsKey{}, pins)
}

// ReleasedRecordVersion matches both kind and ID so one pin cannot authorize another record.
func ReleasedRecordVersion(ctx context.Context, entityType string, entityID ids.UUID) (int64, bool) {
	pins, ok := ctx.Value(releasedRecordPinsKey{}).(ReleasedRecordPins)
	if !ok {
		return 0, false
	}
	for _, pin := range []*ReleasedRecordPin{pins.Target, pins.CoTarget} {
		if pin != nil && pin.EntityType == entityType && pin.EntityID == entityID {
			return pin.Version, true
		}
	}
	return 0, false
}

// ReleasedRecordContext answers the context digest the approval released for this record.
func ReleasedRecordContext(ctx context.Context, entityType string, entityID ids.UUID) (string, bool) {
	pins, ok := ctx.Value(releasedRecordPinsKey{}).(ReleasedRecordPins)
	if !ok || pins.Target == nil || pins.Target.ContextDigest == "" {
		return "", false
	}
	if pins.Target.EntityType != entityType || pins.Target.EntityID != entityID {
		return "", false
	}
	return pins.Target.ContextDigest, true
}

// ReleasedContextMoved is the refusal for an approval whose reviewed context changed.
// The approvals engine and the native writer both answer it, so the wording is shared.
func ReleasedContextMoved() error {
	return &apperrors.VersionSkewError{Message: "The company or evidence this approval was reviewed on changed, " +
		"so nothing was changed. Stage it again for a fresh approval."}
}
