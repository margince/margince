// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import "testing"

// One broken connection outranks a live one: a reader with two calendars, one
// failing, cannot read an empty day as free. A parked one is the user's own
// choice, and a failed history import still delivers what happens from now on.
func TestReachOfSaysWhetherTheProvidersFeedTheProduct(t *testing.T) {
	failing := "provider_unavailable"
	calendars := []string{"gcal", "graphcal"}
	for _, tc := range []struct {
		name  string
		views []ConnectionView
		want  Reach
	}{
		{"no connection", nil, ReachNone},
		{"only a mailbox", []ConnectionView{{Provider: "gmail", Status: statusConnected}}, ReachNone},
		{"parked", []ConnectionView{{Provider: "gcal", Status: statusDisconnected, LastErrorClass: &failing}}, ReachNone},
		{"live", []ConnectionView{{Provider: "gcal", Status: statusConnected}}, ReachLive},
		{"history import failed", []ConnectionView{
			{Provider: "gcal", Status: statusConnected, Backfill: &BackfillRun{Status: backfillStatusError}},
		}, ReachLive},
		{"wants reauthorisation", []ConnectionView{{Provider: "gcal", Status: statusReauthRequired}}, ReachBroken},
		{"in error", []ConnectionView{{Provider: "graphcal", Status: statusError}}, ReachBroken},
		{"sync failing", []ConnectionView{{Provider: "gcal", Status: statusConnected, LastErrorClass: &failing}}, ReachBroken},
		{"one broken beside one live", []ConnectionView{
			{Provider: "gcal", Status: statusConnected},
			{Provider: "graphcal", Status: statusReauthRequired},
		}, ReachBroken},
		{"a broken mailbox says nothing of the calendar", []ConnectionView{
			{Provider: "gmail", Status: statusError},
			{Provider: "gcal", Status: statusConnected},
		}, ReachLive},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := reachOf(tc.views, calendars); got != tc.want {
				t.Fatalf("reachOf = %q, want %q", got, tc.want)
			}
		})
	}
}
