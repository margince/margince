// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

// The worker's boot banner: the one line an operator reads to see which lanes
// this process role actually came up with. It is prose about the wiring, kept
// out of main.go so startJobRunner stays the wiring itself.

import (
	"fmt"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/platform/keyvault"
)

// jobRunnerBanner names, for every lane, the configuration that enabled it or
// the reason it is off.
func jobRunnerBanner(cfg workerConfig, watchCfg compose.GmailWatchConfig, graphWatchRuns bool, modelPath compose.ModelPath, vault keyvault.Vault, runnerSvc *compose.RunnerService) string {
	gmailWired := cfg.gmailAppWired()
	providers := "imap"
	if gmailWired {
		providers += "+gmail"
	}
	if cfg.graphClientID != "" && cfg.graphClientSecret != "" {
		providers += "+graph"
	}
	captureNote := fmt.Sprintf("capture sweep every %s: %s", cfg.gmailSyncInterval, providers)
	switch {
	case gmailWired && watchCfg.Topic != "":
		captureNote = fmt.Sprintf("capture sweep every %s: %s, watch renew every %s", cfg.gmailSyncInterval, providers, cfg.gmailWatchInterval)
	case gmailWired:
		captureNote = fmt.Sprintf("capture sweep every %s: %s (watch off: no pubsub topic)", cfg.gmailSyncInterval, providers)
	}
	captureNote += graphWatchNote(cfg, graphWatchRuns)
	overlayNote := "overlay reconcile off (no keyvault configured)"
	if vault != nil {
		overlayNote = fmt.Sprintf("overlay reconcile every %s", cfg.overlayInterval)
	}
	// The Telegram poller is gated on the same vault (it unseals each bot's
	// token), and it must say so by name: a worker booted without the key
	// registers no poller at all, while an api that HAS the key still accepts
	// Connect — the connection then reads `connected` and nothing ever polls.
	// This line is the one place that split-brain is visible.
	channelNote := "telegram poll off (no keyvault: bot tokens cannot be unsealed, connected bots are NOT polled)"
	if vault != nil {
		channelNote = "telegram poll on"
	}
	deepReadNote := "deep read on"
	if modelPath.SiteExtract == nil {
		deepReadNote = "deep read degraded: no model path, queued reads will fail (configure --ai-routing)"
	}
	// Gated on the signing key, and it must say so by name: an api that HAS the
	// key still accepts subscriptions and parks failed deliveries, while a
	// worker booted without it re-attempts none of them.
	webhookNote := "webhook retry off (no --webhook-key: parked deliveries are NOT re-attempted)"
	if cfg.webhookKey != "" {
		webhookNote = fmt.Sprintf("webhook retry every %s", cfg.webhookRetryInterval)
	}
	// Read off the SERVICE, which is the value registration itself gates on
	// (compose.AgentSchedulerConfig.Service) — not off the model path that
	// happens to decide it today. A banner announcing a cadence on a worker
	// that registered no scheduler is worse than no banner, and this line is
	// the one place an operator looks. Without a service the agent catalog is
	// never seeded, so no morning brief and no at-risk sweep ever runs while
	// every other lane here reads healthy; say so by name.
	schedulerNote := "agent scheduler off (no model path: no brief and no at-risk sweep will run — configure --ai-routing)"
	if runnerSvc != nil {
		schedulerNote = fmt.Sprintf("agent scheduler every %s", cfg.runnerInterval)
	}
	return fmt.Sprintf("worker running River jobs (close-date every %s, reconcile every %s, time-scan every %s, retention every %s, %s, %s, %s, %s, %s, %s)",
		cfg.closeDateInterval, cfg.reconcileInterval, cfg.timeScanInterval, cfg.retentionInterval,
		captureNote, channelNote, overlayNote, deepReadNote, webhookNote, schedulerNote)
}

// graphWatchNote says what the Graph push lane is doing, in the banner's own
// voice.
//
// WHAT CAME UP, not what was asked for. The renewal pair needs a notification
// url AND a Graph connector in the registry, so a worker given the url with
// incomplete client credentials registers nothing — and a banner reading the url
// alone told an operator the lane was running while no job existed to run it.
// This is the one place they look to check.
//
// Its own function because the banner is a series of these and the linter counts
// the branches of all of them together; splitting one out is the cheapest way to
// keep the next line from having to argue with a budget.
func graphWatchNote(cfg workerConfig, runs bool) string {
	switch {
	case cfg.graphClientID == "" && cfg.graphNotifyURL == "":
		return ""
	case runs:
		return fmt.Sprintf(", graph subscription renew every %s", cfg.graphWatchInterval)
	case cfg.graphNotifyURL == "":
		return ", graph push off: no notification url"
	default:
		return ", graph push off: no graph app configured for this role"
	}
}
