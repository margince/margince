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
		edge := search.InteractionEdge{InCount90d: counts.in, OutCount90d: counts.out, SentCount90d: &counts.out, Count90d: counts.in + counts.out, LastAt: now}
		colleague := WireColleague(edge, "Demo Colleague", now)
		if colleague.TwoWay == nil || *colleague.TwoWay != counts.want {
			t.Fatalf("direction %d/%d = %v", counts.in, counts.out, colleague.TwoWay)
		}
	}
}

func TestWireColleagueCarriesWhatTheColleagueSent(t *testing.T) {
	now := time.Date(2026, 8, 13, 9, 0, 0, 0, time.UTC)
	sent := 1
	edge := search.InteractionEdge{OutCount90d: 3, SentCount90d: &sent, Count90d: 3, LastAt: now}
	colleague := WireColleague(edge, "Demo Colleague", now)
	if colleague.Sent90d == nil || *colleague.Sent90d != 1 {
		t.Fatalf("sent_90d = %v, want 1 of the 3 outbound mails", colleague.Sent90d)
	}
	if colleague.Outbound90d == nil || *colleague.Outbound90d != 3 {
		t.Fatalf("outbound_90d = %v, want 3", colleague.Outbound90d)
	}
}

func TestAColleagueCopiedBothWaysIsNotTwoWay(t *testing.T) {
	now := time.Date(2026, 8, 13, 9, 0, 0, 0, time.UTC)
	none := 0
	edge := search.InteractionEdge{InCount90d: 1, OutCount90d: 1, SentCount90d: &none, Count90d: 2, LastAt: now}
	if colleague := WireColleague(edge, "Copied Colleague", now); colleague.TwoWay == nil || *colleague.TwoWay {
		t.Fatalf("two_way = %v for a colleague who sent nothing, want false", colleague.TwoWay)
	}
}

func TestARowFoldedBeforeTheSentCountKeepsTheOutboundReading(t *testing.T) {
	now := time.Date(2026, 8, 13, 9, 0, 0, 0, time.UTC)
	edge := search.InteractionEdge{InCount90d: 1, OutCount90d: 1, Count90d: 2, LastAt: now}
	colleague := WireColleague(edge, "Unfolded Colleague", now)
	if colleague.TwoWay == nil || !*colleague.TwoWay {
		t.Fatalf("two_way = %v for a row with no sent count yet, want the outbound reading (true)", colleague.TwoWay)
	}
	if colleague.Sent90d != nil {
		t.Fatalf("sent_90d = %v, want absent until the row is refolded", *colleague.Sent90d)
	}
}
