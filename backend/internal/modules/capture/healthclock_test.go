// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"testing"
	"time"
)

func TestConnectionHealthUsesDeliveryAndSyncEpisodes(t *testing.T) {
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	ago := func(d time.Duration) *time.Time { at := now.Add(-d); return &at }
	for _, tc := range []struct {
		name                         string
		success, delivery, dismissed *time.Time
		connected                    time.Time
		provider, status, want       string
	}{
		{"stopped sync", ago(25 * time.Hour), nil, nil, *ago(30 * time.Hour), "gmail", statusConnected, ConcernSyncStalled},
		{"sync boundary", ago(24 * time.Hour), ago(time.Hour), nil, *ago(30 * time.Hour), "gmail", statusConnected, ""},
		{"recent sync", ago(23 * time.Hour), nil, nil, *ago(30 * time.Hour), "gmail", statusConnected, ""},
		{"first sync late", nil, nil, nil, *ago(2 * time.Hour), "gmail", statusConnected, ConcernSyncStalled},
		{"first sync grace", nil, nil, nil, *ago(time.Hour), "gmail", statusConnected, ""},
		{"reconnect grace", ago(240 * time.Hour), ago(240 * time.Hour), nil, *ago(time.Minute), "gmail", statusConnected, ""},
		{"quiet mailbox", ago(time.Hour), ago(96 * time.Hour), nil, *ago(240 * time.Hour), "gmail", statusConnected, ConcernCaptureSilent},
		{"silence boundary", ago(time.Hour), ago(72 * time.Hour), nil, *ago(240 * time.Hour), "gmail", statusConnected, ""},
		{"recent delivery", ago(time.Hour), ago(48 * time.Hour), nil, *ago(240 * time.Hour), "gmail", statusConnected, ""},
		{"never delivered", ago(time.Hour), nil, nil, *ago(96 * time.Hour), "gmail", statusConnected, ConcernCaptureSilent},
		{"calendar", ago(time.Hour), nil, nil, *ago(240 * time.Hour), "gcal", statusConnected, ""},
		{"dismissed", ago(time.Hour), ago(96 * time.Hour), ago(96 * time.Hour), *ago(240 * time.Hour), "gmail", statusConnected, ""},
		{"delivery after dismissal", ago(time.Hour), ago(96 * time.Hour), ago(120 * time.Hour), *ago(240 * time.Hour), "gmail", statusConnected, ConcernCaptureSilent},
		{"dismiss cannot hide stall", ago(25 * time.Hour), ago(96 * time.Hour), ago(90 * time.Hour), *ago(240 * time.Hour), "gmail", statusConnected, ConcernSyncStalled},
		{"disconnected", ago(240 * time.Hour), nil, nil, *ago(240 * time.Hour), "gmail", statusDisconnected, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			view := ConnectionView{
				Provider: tc.provider, Status: tc.status, ConnectedAt: tc.connected,
				LastSuccessAt: tc.success, LastDeliveryAt: tc.delivery, SilenceDismissedSince: tc.dismissed,
			}
			classifyConnection(&view, now)
			if view.Concern != tc.want {
				t.Fatalf("concern = %q, want %q", view.Concern, tc.want)
			}
			if tc.want == ConcernCaptureSilent && (view.ConcernSince == nil || !view.ConcernSince.Equal(deliverySince(view))) {
				t.Fatal("silence lost its start")
			}
			if tc.want == "" && view.ConcernSince != nil {
				t.Fatal("healthy connection carries a concern start")
			}
		})
	}
}

func TestReachCountsAStalledCalendarAsBroken(t *testing.T) {
	view := ConnectionView{Provider: "gcal", Status: statusConnected, Concern: ConcernSyncStalled}
	if got := reachOf([]ConnectionView{view}, []string{"gcal"}); got != ReachBroken {
		t.Fatalf("reach = %q", got)
	}
}

func TestFailedHistoryDoesNotMakeAStalledSyncReachable(t *testing.T) {
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	last := now.Add(-25 * time.Hour)
	view := ConnectionView{
		Provider: "gcal", Status: statusConnected, ConnectedAt: now.Add(-48 * time.Hour),
		LastSuccessAt: &last, Backfill: &BackfillRun{Status: backfillStatusError},
	}
	classifyConnection(&view, now)
	if view.Concern != ConcernBackfillFailed {
		t.Fatalf("decided priority changed: %q", view.Concern)
	}
	if got := reachOf([]ConnectionView{view}, []string{"gcal"}); got != ReachBroken {
		t.Fatalf("stalled sync reached as %q", got)
	}
}
