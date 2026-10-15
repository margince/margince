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

	"github.com/margince/margince/backend/internal/platform/config"
)

// scheduleStore stores a pass interval. The worker reads it as a system actor,
// through the read gate, so it is not machinery-applied.
var scheduleStore = config.Store{RBACObject: installationSettingsObject, AuditVerb: installationAuditVerb}

const (
	day  = 24 * time.Hour
	week = 7 * day
)

func seconds(d time.Duration) config.Option[int] { return config.Some(int(d / time.Second)) }

// offOrWithin admits zero as "switched off" beside the range, which Min and
// Max cannot say.
func offOrWithin(lowest, highest time.Duration) func(int) error {
	return func(n int) error {
		if n != 0 && (n < int(lowest/time.Second) || n > int(highest/time.Second)) {
			return fmt.Errorf("choose %d..%d seconds, or 0 for off", int(lowest/time.Second), int(highest/time.Second))
		}
		return nil
	}
}

// The pass intervals, each named for the pass it paces. A pass the worker
// fans out per workspace every tick stays at seconds-to-an-hour; a daily
// sweep may run between hourly and weekly.
var (
	AgentRunnerIntervalSeconds = config.DefineStored(config.Spec[int]{
		Key: "installation.agent_runner_interval_seconds", Env: "MARGINCE_INSTALLATION_AGENT_RUNNER_INTERVAL_SECONDS",
		Type: config.Int, Default: seconds(30 * time.Second), Min: seconds(10 * time.Second), Max: seconds(time.Hour),
		Doc: "Seconds between the agent scheduler's passes.",
	}, scheduleStore)
	WebhookRetryIntervalSeconds = config.DefineStored(config.Spec[int]{
		Key: "installation.webhook_retry_interval_seconds", Env: "MARGINCE_INSTALLATION_WEBHOOK_RETRY_INTERVAL_SECONDS",
		Type: config.Int, Default: seconds(30 * time.Second), Min: seconds(10 * time.Second), Max: seconds(time.Hour),
		Doc: "Seconds between the passes that retry failed webhook deliveries.",
	}, scheduleStore)
	TimeScanIntervalSeconds = config.DefineStored(config.Spec[int]{
		Key: "installation.time_scan_interval_seconds", Env: "MARGINCE_INSTALLATION_TIME_SCAN_INTERVAL_SECONDS",
		Type: config.Int, Default: seconds(time.Hour), Min: seconds(time.Minute), Max: seconds(day),
		Doc: "Seconds between the time-scan passes.",
	}, scheduleStore)
	CloseDateSweepIntervalSeconds = config.DefineStored(config.Spec[int]{
		Key: "installation.close_date_sweep_interval_seconds", Env: "MARGINCE_INSTALLATION_CLOSE_DATE_SWEEP_INTERVAL_SECONDS",
		Type: config.Int, Default: seconds(day), Min: seconds(time.Hour), Max: seconds(week),
		Doc: "Seconds between the close-date sweeps.",
	}, scheduleStore)
	FollowUpReconcileIntervalSeconds = config.DefineStored(config.Spec[int]{
		Key: "installation.follow_up_reconcile_interval_seconds", Env: "MARGINCE_INSTALLATION_FOLLOW_UP_RECONCILE_INTERVAL_SECONDS",
		Type: config.Int, Default: seconds(day), Min: seconds(time.Hour), Max: seconds(week),
		Doc: "Seconds between the follow-up reconcile passes.",
	}, scheduleStore)
	RetentionSweepIntervalSeconds = config.DefineStored(config.Spec[int]{
		Key: "installation.retention_sweep_interval_seconds", Env: "MARGINCE_INSTALLATION_RETENTION_SWEEP_INTERVAL_SECONDS",
		Type: config.Int, Default: seconds(day), Min: seconds(time.Hour), Max: seconds(week),
		Doc: "Seconds between the retention sweeps.",
	}, scheduleStore)
	GeocodeBackfillIntervalSeconds = config.DefineStored(config.Spec[int]{
		Key: "installation.geocode_backfill_interval_seconds", Env: "MARGINCE_INSTALLATION_GEOCODE_BACKFILL_INTERVAL_SECONDS",
		Type: config.Int, Default: seconds(time.Hour), Validate: offOrWithin(5*time.Minute, week),
		Doc: "Seconds between the geocoding backfill sweeps, or 0 for off.",
	}, scheduleStore)
	TechnicalBackfillIntervalSeconds = config.DefineStored(config.Spec[int]{
		Key: "installation.technical_backfill_interval_seconds", Env: "MARGINCE_INSTALLATION_TECHNICAL_BACKFILL_INTERVAL_SECONDS",
		Type: config.Int, Default: seconds(6 * time.Hour), Validate: offOrWithin(5*time.Minute, week),
		Doc: "Seconds between the technical backfill sweeps, or 0 for off.",
	}, scheduleStore)
	// The watch scans run at least twice a day: a renewal window is at least
	// a day (below), so two scans always land inside it, with room for one
	// that runs late or fails.
	GmailWatchScanIntervalSeconds = config.DefineStored(config.Spec[int]{
		Key: "installation.gmail_watch_scan_interval_seconds", Env: "MARGINCE_INSTALLATION_GMAIL_WATCH_SCAN_INTERVAL_SECONDS",
		Type: config.Int, Default: seconds(6 * time.Hour), Min: seconds(10 * time.Minute), Max: seconds(day / 2),
		Doc: "Seconds between the scans for Gmail push subscriptions due for renewal.",
	}, scheduleStore)
	GraphWatchScanIntervalSeconds = config.DefineStored(config.Spec[int]{
		Key: "installation.graph_watch_scan_interval_seconds", Env: "MARGINCE_INSTALLATION_GRAPH_WATCH_SCAN_INTERVAL_SECONDS",
		Type: config.Int, Default: seconds(6 * time.Hour), Min: seconds(10 * time.Minute), Max: seconds(day / 2),
		Doc: "Seconds between the scans for Microsoft Graph mail subscriptions due for renewal.",
	}, scheduleStore)
)

// How far ahead of a subscription's expiry it is renewed: at least a day, so
// the twice-daily-at-least watch scan always meets it, and below the
// provider's own lifetime, or every scan would renew everything.
var (
	// A Gmail watch lasts seven days.
	GmailWatchRenewWithinHours = config.DefineStored(config.Spec[int]{
		Key: "installation.gmail_watch_renew_within_hours", Env: "MARGINCE_INSTALLATION_GMAIL_WATCH_RENEW_WITHIN_HOURS",
		Type: config.Int, Default: config.Some(48), Min: config.Some(24), Max: config.Some(144),
		Doc: "Hours before a Gmail push subscription expires that it is renewed.",
	}, scheduleStore)
	// A Graph mail subscription lasts at most 4230 minutes, just under three
	// days, so the margin stops well short of it.
	GraphWatchRenewWithinHours = config.DefineStored(config.Spec[int]{
		Key: "installation.graph_watch_renew_within_hours", Env: "MARGINCE_INSTALLATION_GRAPH_WATCH_RENEW_WITHIN_HOURS",
		Type: config.Int, Default: config.Some(24), Min: config.Some(24), Max: config.Some(60),
		Doc: "Hours before a Microsoft Graph mail subscription expires that it is renewed.",
	}, scheduleStore)
)

// ScheduleSettings are the pass intervals and renewal margins the worker reads
// as it runs. Compose looks one up by the key a job cadence names.
func ScheduleSettings() []*config.Setting[int] {
	return []*config.Setting[int]{
		AgentRunnerIntervalSeconds, WebhookRetryIntervalSeconds, TimeScanIntervalSeconds,
		CloseDateSweepIntervalSeconds, FollowUpReconcileIntervalSeconds, RetentionSweepIntervalSeconds,
		GeocodeBackfillIntervalSeconds, TechnicalBackfillIntervalSeconds,
		GmailWatchScanIntervalSeconds, GraphWatchScanIntervalSeconds,
		GmailWatchRenewWithinHours, GraphWatchRenewWithinHours,
	}
}
