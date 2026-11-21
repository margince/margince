// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package zonefault

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A nil pool proves a pass whose zone loaded reaches no database.
func TestAPassWhoseZoneLoadedRecordsNothing(t *testing.T) {
	ctx, watch := storekit.WatchZones(context.Background())
	storekit.EffectiveZone(ctx, "Europe/Berlin")
	if err := Record(ctx, nil, ids.NewV7(), watch); err != nil {
		t.Errorf("recording a clean pass failed: %v", err)
	}
}
