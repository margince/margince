// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"context"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type ReleasedRecordPin struct {
	EntityType string
	EntityID   ids.UUID
	Version    int64
}

type ReleasedRecordPins struct {
	Target   *ReleasedRecordPin
	CoTarget *ReleasedRecordPin
}

type releasedRecordPinsKey struct{}

// Pins cross the redemption transaction so each native writer can check them under its row locks.
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
