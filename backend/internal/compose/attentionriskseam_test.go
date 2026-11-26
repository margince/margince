// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

func TestEmptyRiskPagesNeedNoZoneRead(t *testing.T) {
	lane := attentionAtRisk{
		lister: func(context.Context, *storekit.OwnerSet) ([]agents.SlippingDeal, bool, error) {
			return nil, false, nil
		},
		tail: func(context.Context, time.Time, *storekit.OwnerSet, string, int) ([]agents.SlippingDeal, []string, bool, error) {
			return nil, nil, false, nil
		},
		now: func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) },
	}
	quiet, cut, err := lane.Quiet(context.Background(), nil)
	if err != nil || cut || quiet == nil || len(quiet) != 0 {
		t.Errorf("empty risk lane = %v, cut %t, error %v", quiet, cut, err)
	}
	scroll, more, err := lane.RiskAfter(context.Background(), lane.now(), nil, "", 20)
	if err != nil || more || scroll == nil || len(scroll) != 0 {
		t.Errorf("empty risk scroll = %v, more %t, error %v", scroll, more, err)
	}
}
