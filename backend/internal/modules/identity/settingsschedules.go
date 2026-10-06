// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// How often the worker's background passes run, and how far ahead of expiry a
// mailbox's push subscription is renewed. Settings rather than process flags
// so an admin who sees a pass running too often or too rarely on the job
// health page can change it there; the worker re-reads them while it runs.
//
// Every bound is chosen so a value inside it cannot break what the pass is
// for. Only the two backfill sweeps may be switched off: each is a catch-up
// over addresses or domains nothing else revisits, and an installation that
// makes no outbound lookups has nothing for them to do. The retention pass is
// a privacy obligation and the watch scans keep push capture alive, so
// neither has an off.

import (
	"fmt"
	"time"

	"github.com/margince/margince/backend/internal/platform/settings"
)

// scheduleSeconds declares one pass interval in whole seconds. allowOff admits
// zero as "switched off" beside the range.
func scheduleSeconds(key string, def time.Duration, lowest, highest time.Duration, allowOff bool) *settings.Entry[int] {
	return settings.Define[int](key, installationSettingsObject, "update", int(def/time.Second),
		func(seconds int) error {
			if allowOff && seconds == 0 {
				return nil
			}
			if seconds < int(lowest/time.Second) || seconds > int(highest/time.Second) {
				off := ""
				if allowOff {
					off = ", or 0 for off"
				}
				return fmt.Errorf("choose %d..%d seconds%s, not %d", int(lowest/time.Second), int(highest/time.Second), off, seconds)
			}
			return nil
		})
}

const (
	day  = 24 * time.Hour
	week = 7 * day
)

// The pass intervals, each named for the pass it paces. A pass the worker
// fans out per workspace every tick stays at seconds-to-an-hour; a daily
// sweep may run between hourly and weekly.
var (
	AgentRunnerIntervalSeconds       = scheduleSeconds("installation.agent_runner_interval_seconds", 30*time.Second, 10*time.Second, time.Hour, false)
	WebhookRetryIntervalSeconds      = scheduleSeconds("installation.webhook_retry_interval_seconds", 30*time.Second, 10*time.Second, time.Hour, false)
	TimeScanIntervalSeconds          = scheduleSeconds("installation.time_scan_interval_seconds", time.Hour, time.Minute, day, false)
	CloseDateSweepIntervalSeconds    = scheduleSeconds("installation.close_date_sweep_interval_seconds", day, time.Hour, week, false)
	FollowUpReconcileIntervalSeconds = scheduleSeconds("installation.follow_up_reconcile_interval_seconds", day, time.Hour, week, false)
	RetentionSweepIntervalSeconds    = scheduleSeconds("installation.retention_sweep_interval_seconds", day, time.Hour, week, false)
	GeocodeBackfillIntervalSeconds   = scheduleSeconds("installation.geocode_backfill_interval_seconds", time.Hour, 5*time.Minute, week, true)
	TechnicalBackfillIntervalSeconds = scheduleSeconds("installation.technical_backfill_interval_seconds", 6*time.Hour, 5*time.Minute, week, true)
	// The watch scans may not run less than daily: a renewal window is at
	// least a day (below), so a scan always lands inside it.
	GmailWatchScanIntervalSeconds = scheduleSeconds("installation.gmail_watch_scan_interval_seconds", 6*time.Hour, 10*time.Minute, day, false)
	GraphWatchScanIntervalSeconds = scheduleSeconds("installation.graph_watch_scan_interval_seconds", 6*time.Hour, 10*time.Minute, day, false)
)

// How far ahead of a subscription's expiry it is renewed: at least a day, so
// the daily-at-most watch scan always meets it, and below the provider's own
// lifetime, or every scan would renew everything.
var (
	// A Gmail watch lasts seven days.
	GmailWatchRenewWithinHours = boundedInt("installation.gmail_watch_renew_within_hours", "hours", 48, 24, 144)
	// A Graph mail subscription lasts at most 4230 minutes, just under three
	// days, so the margin stops well short of it.
	GraphWatchRenewWithinHours = boundedInt("installation.graph_watch_renew_within_hours", "hours", 24, 24, 60)
)

// scheduleDefinitions is this file's share of identity's catalog.
func scheduleDefinitions() []settings.Definition {
	return []settings.Definition{
		AgentRunnerIntervalSeconds, WebhookRetryIntervalSeconds, TimeScanIntervalSeconds,
		CloseDateSweepIntervalSeconds, FollowUpReconcileIntervalSeconds, RetentionSweepIntervalSeconds,
		GeocodeBackfillIntervalSeconds, TechnicalBackfillIntervalSeconds,
		GmailWatchScanIntervalSeconds, GraphWatchScanIntervalSeconds,
		GmailWatchRenewWithinHours, GraphWatchRenewWithinHours,
	}
}
