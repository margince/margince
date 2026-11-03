// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package network

import (
	"fmt"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/search"
)

// Reach and the intro routes answer "did this colleague write" the same way
// for one edge. That holds for every mix of inbound, outbound and sent counts,
// a withheld sent count included.
func TestReachAndRoutesAgreeOnWhoWrote(t *testing.T) {
	now := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	for _, in := range []int{0, 3} {
		for _, out := range []int{0, 2} {
			for _, sent := range []*int{nil, new(0), new(1)} {
				edge := search.InteractionEdge{InCount90d: in, OutCount90d: out, SentCount90d: sent, Count90d: in + out, LastAt: now}
				name := fmt.Sprintf("in=%d out=%d sent=%v", in, out, sentLabel(sent))
				t.Run(name, func(t *testing.T) {
					reach := WireColleague(edge, "Colleague", now)
					wire := wireEdge(edge, userNodeID(uuidFor(1)), contactNodeID(uuidFor(9)), now)
					route := evidenceOf(&wire, now, time.UTC)
					if reach.TwoWay == nil || *reach.TwoWay != route.TwoWay {
						t.Errorf("Reach says two-way %v, the route evidence %v", reach.TwoWay, route.TwoWay)
					}
				})
			}
		}
	}
}

func sentLabel(sent *int) string {
	if sent == nil {
		return "withheld"
	}
	return fmt.Sprint(*sent)
}
