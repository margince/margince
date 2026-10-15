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

import "github.com/margince/margince/backend/internal/platform/config"

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
var AutoEnrichDailyCap = config.DefineStored(config.Spec[int]{
	Key:     "capture.auto_enrich_daily_cap",
	Env:     "MARGINCE_CAPTURE_AUTO_ENRICH_DAILY_CAP",
	Type:    config.Int,
	Default: config.Some(DefaultAutoEnrichDailyCap),
	Min:     config.Some(1),
	Max:     config.Some(MaxAutoEnrichDailyCap),
	Doc:     "Automatic company web-page reads the installation may start in one UTC day.",
}, config.Store{RBACObject: captureSettingsObject, AuditVerb: captureAuditVerb})

// SiteReadMaxPages bounds how many pages one deep read fetches. An automatic
// read runs under its own lower ceiling; this is what a read somebody asked
// for may reach.
var SiteReadMaxPages = config.DefineStored(config.Spec[int]{
	Key:     "capture.site_read_max_pages",
	Env:     "MARGINCE_CAPTURE_SITE_READ_MAX_PAGES",
	Type:    config.Int,
	Default: config.Some(DefaultSiteReadMaxPages),
	Min:     config.Some(1),
	Max:     config.Some(MaxSiteReadMaxPages),
	Doc:     "Pages one company web-page read may fetch.",
}, config.Store{RBACObject: captureSettingsObject, AuditVerb: captureAuditVerb})

// SiteReadMaxMiB bounds the bytes one deep read holds, summed over its pages.
// Six reads run at once, so the ceiling is also what bounds the worker's
// memory for crawling.
var SiteReadMaxMiB = config.DefineStored(config.Spec[int]{
	Key:     "capture.site_read_max_mib",
	Env:     "MARGINCE_CAPTURE_SITE_READ_MAX_MIB",
	Type:    config.Int,
	Default: config.Some(DefaultSiteReadMaxMiB),
	Min:     config.Some(1),
	Max:     config.Some(MaxSiteReadMaxMiB),
	Doc:     "MiB one company web-page read may hold, summed over its pages.",
}, config.Store{RBACObject: captureSettingsObject, AuditVerb: captureAuditVerb})

// SiteReadWallSeconds is how long one crawl may run before it stops and
// extracts what it has.
var SiteReadWallSeconds = config.DefineStored(config.Spec[int]{
	Key:     "capture.site_read_wall_seconds",
	Env:     "MARGINCE_CAPTURE_SITE_READ_WALL_SECONDS",
	Type:    config.Int,
	Default: config.Some(DefaultSiteReadWallSeconds),
	Min:     config.Some(MinSiteReadWallSeconds),
	Max:     config.Some(MaxSiteReadWallSeconds),
	Doc:     "Seconds one company web-page read may run before it stops and extracts what it has.",
}, config.Store{RBACObject: captureSettingsObject, AuditVerb: captureAuditVerb})

// MailSyncIntervalSeconds is how long a healthy mailbox connection waits
// between polls. Push notifications, where a mailbox has them, make this the
// safety net rather than the latency floor. Two minutes by default; at least
// thirty seconds so a fleet of mailboxes cannot poll a provider into a rate
// limit, at most an hour so a mailbox without push still feels live.
var MailSyncIntervalSeconds = config.DefineStored(config.Spec[int]{
	Key:     "capture.mail_sync_interval_seconds",
	Env:     "MARGINCE_CAPTURE_MAIL_SYNC_INTERVAL_SECONDS",
	Type:    config.Int,
	Default: config.Some(DefaultMailSyncIntervalSeconds),
	Min:     config.Some(30),
	Max:     config.Some(3600),
	Doc:     "Seconds a healthy mailbox connection waits between polls.",
}, config.Store{
	RBACObject: captureSettingsObject, AuditVerb: captureAuditVerb,
	MachineryApplied: true, // the sync applies it to the next_sync_at it writes
})

// DefaultMailSyncIntervalSeconds is a healthy connection's poll interval when
// nobody has chosen one.
const DefaultMailSyncIntervalSeconds = 120
