// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"log/slog"

	"github.com/margince/margince/backend/internal/platform/config"
)

// The capture pipeline's deployment tuning. The consumer-mail lists moved to
// the workspace's own settings. Their keys still load, so a file that carries
// them boots, and a warning says they act on nothing.
var (
	CaptureFreemailExtra = config.Define(config.Spec[[]string]{
		Key: "capture.freemail_extra", Env: "MARGINCE_CAPTURE_FREEMAIL_EXTRA", Type: config.List(config.String), Roles: bothRoles,
		Doc: "Ignored, with a warning at boot; the consumer-mail list is edited under Settings.",
	})
	CaptureFreemailNever = config.Define(config.Spec[[]string]{
		Key: "capture.freemail_never", Env: "MARGINCE_CAPTURE_FREEMAIL_NEVER", Type: config.List(config.String), Roles: bothRoles,
		Doc: "Ignored, with a warning at boot; the consumer-mail list is edited under Settings.",
	})
	CaptureTransactionalExtra = config.Define(config.Spec[[]string]{
		Key: "capture.transactional_extra", Env: "MARGINCE_CAPTURE_TRANSACTIONAL_EXTRA", Type: config.List(config.String), Roles: bothRoles,
		Doc: "Mail-infrastructure domains added to the built-in list; their mail keeps the activity and derives no counterparty.",
	})
	CaptureTransactionalNever = config.Define(config.Spec[[]string]{
		Key: "capture.transactional_never", Env: "MARGINCE_CAPTURE_TRANSACTIONAL_NEVER", Type: config.List(config.String), Roles: bothRoles,
		Doc: "Domains never suppressed as transactional, whatever the built-in rules say.",
	})
	// Only the deployment turns it off. A member must not be able to switch on
	// retention of colleagues' subjects. An operator who turns it off makes a
	// decision with a name attached.
	CaptureTracePayloads = config.Define(config.Spec[bool]{
		Key: "capture.trace_payloads", Env: "MARGINCE_CAPTURE_TRACE_PAYLOADS", Type: config.Bool, Default: config.Some(true), Roles: bothRoles,
		Doc: "Keep each traced message's sender and a bounded subject in the 24-hour capture trace, dropped messages included.",
	})
	CaptureMaxBackfillMonths = config.Define(config.Spec[int]{
		Key: "capture.max_backfill_months", Env: "MARGINCE_CAPTURE_MAX_BACKFILL_MONTHS", Type: config.Int, Default: config.Some(0), Roles: bothRoles,
		Doc: "How far back a mailbox import may reach, for the whole installation; 0 is no cap.",
	})
	CaptureSkipReservedDomainProposals = config.Define(config.Spec[bool]{
		Key: "capture.skip_reserved_domain_proposals", Env: "MARGINCE_CAPTURE_SKIP_RESERVED_DOMAIN_PROPOSALS", Type: config.Bool,
		Default: config.Some(true), Roles: bothRoles,
		Doc: "Raise no contact proposal for a sender on an RFC 2606 reserved name; the mail is still captured.",
	})
)

// captureSettings are the capture tuning keys above.
func captureSettings() []config.Any {
	return []config.Any{
		CaptureFreemailExtra, CaptureFreemailNever, CaptureTransactionalExtra, CaptureTransactionalNever,
		CaptureTracePayloads, CaptureMaxBackfillMonths, CaptureSkipReservedDomainProposals,
	}
}

// CaptureConfigFromDeploy maps the capture settings onto the config the Sink
// gates read. Every role that boots goes through here, so the stale-key
// warning is logged here, once, at boot.
func CaptureConfigFromDeploy(c *config.Config, log *slog.Logger) CaptureConfig {
	if len(config.Val(c, CaptureFreemailExtra)) > 0 || len(config.Val(c, CaptureFreemailNever)) > 0 {
		log.Warn("capture configuration: capture.freemail_extra / capture.freemail_never are ignored: the consumer-mail " +
			"list moved to the workspace, editable under Settings or at POST /v1/capture/consumer-mail-domains. Remove them from margince.yaml.")
	}
	return CaptureConfig{
		TransactionalExtra: config.Val(c, CaptureTransactionalExtra),
		TransactionalNever: config.Val(c, CaptureTransactionalNever),
		TracePayloads:      config.Val(c, CaptureTracePayloads),
		MaxBackfillMonths:  config.Val(c, CaptureMaxBackfillMonths),
		Logger:             log,

		SkipReservedDomainProposals: config.Val(c, CaptureSkipReservedDomainProposals),
	}
}
