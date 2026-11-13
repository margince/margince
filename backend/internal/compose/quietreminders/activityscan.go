// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package quietreminders reads the records a quiet-relationship rule watches:
// their last activity and the list, pipeline or deal population they belong to.
package quietreminders

import (
	"context"
	"time"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/automation"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// activityScanAdapter keeps activity-link storage names out of the automation contract.
type activityScanAdapter struct {
	store *activities.Store
}

var _ automation.ActivityScan = activityScanAdapter{}

func (a activityScanAdapter) LastTouchBefore(ctx context.Context, cutoff time.Time, limit int, reminder string) ([]automation.EntityAnchor, error) {
	candidates, err := a.store.LastTouchBefore(ctx, cutoff, limit, reminder)
	if err != nil {
		return nil, err
	}
	out := make([]automation.EntityAnchor, len(candidates))
	for i, c := range candidates {
		out[i] = automation.EntityAnchor{
			Ref:    datasource.EntityRef{Type: datasource.EntityType(c.EntityType), ID: c.EntityID},
			Anchor: c.LastTouch,
		}
	}
	return out, nil
}
