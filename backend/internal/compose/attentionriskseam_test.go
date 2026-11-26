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

// A deal whose newest activity names only a date counts its quiet days from
// that date for every reader. The date is stored at noon UTC, which is already
// the next day at UTC+14.
func TestADateOnlyNewestActivityCountsQuietDaysFromItsOwnDate(t *testing.T) {
	stored := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	now := time.Date(2026, 11, 25, 10, 30, 0, 0, time.UTC)
	cases := []struct {
		zone     string
		dateOnly bool
		want     int
	}{
		{"Pacific/Kiritimati", true, 6},
		{"Pacific/Pago_Pago", true, 4},
		{"Pacific/Kiritimati", false, 5},
		{"Pacific/Pago_Pago", false, 4},
	}
	for _, c := range cases {
		zone, err := time.LoadLocation(c.zone)
		if err != nil {
			t.Fatal(err)
		}
		deal := agents.SlippingDeal{LastActivityAt: &stored, LastActivityDateOnly: c.dateOnly, CreatedAt: stored.AddDate(0, -1, 0)}
		if got := idleDaysOf(deal, now, zone); got != c.want {
			t.Errorf("in %s, a newest activity stored at noon UTC (date only %t) is %d day(s) quiet, want %d",
				c.zone, c.dateOnly, got, c.want)
		}
	}
}
