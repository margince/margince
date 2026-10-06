// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// The website-reading limits: how many automatic reads a day may start, and
// how much one read may fetch. Settings rather than process configuration
// because the right answer changes with the situation — a first backfill wants
// a higher daily cap than a quiet month — and an admin should not need a
// redeploy to give it. Capture declares all four so one PATCH commits them in
// one transaction under the one `capture_settings` gate the auto-enrich switch
// already takes.

import (
	"fmt"

	"github.com/margince/margince/backend/internal/platform/settings"
)

// The defaults and ceilings, exported for the readers that size something
// against them: the deep-read job timeout is computed from the HIGHEST wall an
// admin may choose, so a change here moves that timeout with it.
const (
	DefaultAutoEnrichDailyCap = 500
	MaxAutoEnrichDailyCap     = 20_000

	DefaultSiteReadMaxPages = 60
	MaxSiteReadMaxPages     = 200

	DefaultSiteReadMaxMiB = 32
	MaxSiteReadMaxMiB     = 128

	DefaultSiteReadWallSeconds = 240
	MinSiteReadWallSeconds     = 30
	MaxSiteReadWallSeconds     = 600
)

// AutoEnrichDailyCap is the installation-wide ceiling on automatic site reads
// started in one UTC day. Company auto-enrich and domain triage spend it from
// one atomically-reserved counter (capture_auto_enrich_budget).
//
// It paces, and only paces: concurrency is bounded by the deep-read worker
// pool and model spend by the AI budget, whatever this says. 500 is sized for
// a first backfill, which mints hundreds of companies at once; below that
// arrival rate the CRM visibly trickles. The ceiling exists to catch a typo,
// not to bound cost — at 20,000 the worker pool, not this number, is the limit.
var AutoEnrichDailyCap = settings.Define[int](
	"capture.auto_enrich_daily_cap", captureSettingsObject, "update",
	DefaultAutoEnrichDailyCap, between("reads a day", 1, MaxAutoEnrichDailyCap),
)

// SiteReadMaxPages bounds how many pages one deep read fetches. An automatic
// read runs under its own lower ceiling; this is what a read somebody asked
// for may reach.
var SiteReadMaxPages = settings.Define[int](
	"capture.site_read_max_pages", captureSettingsObject, "update",
	DefaultSiteReadMaxPages, between("pages", 1, MaxSiteReadMaxPages),
)

// SiteReadMaxMiB bounds the bytes one deep read holds, summed over its pages.
// Six reads run at once, so the ceiling is also what bounds the worker's
// memory for crawling.
var SiteReadMaxMiB = settings.Define[int](
	"capture.site_read_max_mib", captureSettingsObject, "update",
	DefaultSiteReadMaxMiB, between("MiB", 1, MaxSiteReadMaxMiB),
)

// SiteReadWallSeconds is how long one crawl may run before it stops and
// extracts what it has.
var SiteReadWallSeconds = settings.Define[int](
	"capture.site_read_wall_seconds", captureSettingsObject, "update",
	DefaultSiteReadWallSeconds, between("seconds", MinSiteReadWallSeconds, MaxSiteReadWallSeconds),
)

// between is the range validator the four limits share; the unit names the
// number in the refusal an admin reads.
func between(unit string, lowest, highest int) func(int) error {
	return func(n int) error {
		if n < lowest || n > highest {
			return fmt.Errorf("choose %d..%d %s, not %d", lowest, highest, unit, n)
		}
		return nil
	}
}

// MailSyncIntervalSeconds is how long a healthy mailbox connection waits
// between polls. Push notifications, where a mailbox has them, make this the
// safety net rather than the latency floor. Two minutes by default; at least
// thirty seconds so a fleet of mailboxes cannot poll a provider into a rate
// limit, at most an hour so a mailbox without push still feels live.
var MailSyncIntervalSeconds = settings.Define[int](
	"capture.mail_sync_interval_seconds", captureSettingsObject, "update",
	DefaultMailSyncIntervalSeconds, between("seconds", 30, 3600),
).MachineryApplied() // the sync applies it to the next_sync_at it writes

// DefaultMailSyncIntervalSeconds is a healthy connection's poll interval when
// nobody has chosen one.
const DefaultMailSyncIntervalSeconds = 120
