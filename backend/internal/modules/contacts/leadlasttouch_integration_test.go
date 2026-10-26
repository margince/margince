// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
)

// A lead's last touch says how precisely it is known, from the activity it
// names. A day note's stored noon reads as its date, a later mail as an instant.
func TestALeadsLastTouchCarriesThePrecisionOfItsActivity(t *testing.T) {
	e := setupPromoteConsent(t)
	date := time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC)
	cases := map[string]struct {
		mail time.Time
		want time.Time
		day  bool
	}{
		"a day note after the morning's mail":  {date.Add(9 * time.Hour), occurredday.Anchor(date), true},
		"an afternoon mail after the day note": {date.Add(15 * time.Hour), date.Add(15 * time.Hour), false},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			lead := e.seedBareLead(t, name+"@touch.test", date)
			e.seedLeadTouch(t, lead, "email", "outbound", "human:"+e.user.String(), "manual", c.mail)
			e.seedDayTouch(t, lead, "note", "", date)
			got, err := e.store.GetLead(e.ctx, lead, storekit.LiveOnly)
			if err != nil {
				t.Fatal(err)
			}
			if got.LastActivityAt == nil || !got.LastActivityAt.Equal(c.want) {
				t.Fatalf("last_activity_at = %v, want %v", got.LastActivityAt, c.want)
			}
			if got.LastActivityPrecision == nil || occurredday.IsDay(*got.LastActivityPrecision) != c.day {
				t.Errorf("last_activity_precision = %v, want day %t", got.LastActivityPrecision, c.day)
			}
		})
	}
	if got, err := e.store.GetLead(e.ctx, e.seedBareLead(t, "untouched@touch.test", date), storekit.LiveOnly); err != nil ||
		got.LastActivityPrecision != nil {
		t.Errorf("an untouched lead reads precision %v (%v), want none", got.LastActivityPrecision, err)
	}
}
