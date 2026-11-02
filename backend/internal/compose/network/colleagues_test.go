// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package network

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/search"
)

func TestColleagueSummaryUsesTheRoutesDirectionProof(t *testing.T) {
	for _, counts := range []struct {
		in, out int
		want    bool
	}{{1, 0, false}, {0, 1, false}, {1, 1, true}} {
		now := time.Date(2026, 8, 13, 9, 0, 0, 0, time.UTC)
		edge := search.InteractionEdge{InCount90d: counts.in, OutCount90d: counts.out, Count90d: counts.in + counts.out, LastAt: now}
		colleague := WireColleague(edge, "Demo Colleague", now)
		if colleague.TwoWay == nil || *colleague.TwoWay != counts.want {
			t.Fatalf("direction %d/%d = %v", counts.in, counts.out, colleague.TwoWay)
		}
	}
}
