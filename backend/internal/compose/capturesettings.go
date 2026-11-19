// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/margince/margince/backend/internal/modules/capture/gmail"
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
		Doc: "Domains of mail services added to the built-in list; their mail keeps the activity and derives no counterparty.",
	})
	CaptureTransactionalNever = config.Define(config.Spec[[]string]{
		Key: "capture.transactional_never", Env: "MARGINCE_CAPTURE_TRANSACTIONAL_NEVER", Type: config.List(config.String), Roles: bothRoles,
		Doc: "Domains never held back as system mail, whatever the built-in rules say.",
	})
	// Only the deployment turns it off. A member must not be able to switch on
	// retention of colleagues' subjects. An operator who turns it off makes a
	// decision with a name attached.
	CaptureTracePayloads = config.Define(config.Spec[bool]{
		Key: "capture.trace_payloads", Env: "MARGINCE_CAPTURE_TRACE_PAYLOADS", Type: config.Bool, Default: config.Some(true), Roles: bothRoles,
		Doc: "Keep each traced message's sender and a bounded subject in the 24-hour capture trace, dropped messages included.",
	})
	CaptureHistoryConcurrency = config.Define(config.Spec[int]{
		Key: "capture.history_concurrency", Env: "MARGINCE_CAPTURE_HISTORY_CONCURRENCY", Type: config.Int, Default: config.Some(1), Roles: bothRoles,
		Validate: func(value int) error {
			if value < 1 || value > 64 {
				return errors.New("must be between 1 and 64")
			}
			return nil
		},
		Doc: "Limit on history imports running at once across the installation. Use the same value in every process.",
	})
	CaptureHistoryPageSize = config.Define(config.Spec[int]{
		Key: "capture.history_page_size", Env: "MARGINCE_CAPTURE_HISTORY_PAGE_SIZE", Type: config.Int, Default: config.Some(0), Roles: bothRoles,
		Validate: func(value int) error {
			if value < 0 || value > gmail.MaxBackfillPageSize {
				return fmt.Errorf("must be between 0 and %d", gmail.MaxBackfillPageSize)
			}
			return nil
		},
		Doc: "Messages one history page lists, and so how many it works through before it saves its place. 0 keeps the built-in 100 for Gmail and for Microsoft.",
	})
	CaptureHistoryFetchWorkers = config.Define(config.Spec[int]{
		Key: "capture.history_fetch_workers", Env: "MARGINCE_CAPTURE_HISTORY_FETCH_WORKERS", Type: config.Int, Default: config.Some(0), Roles: bothRoles,
		Validate: func(value int) error {
			if value < 0 || value > 16 {
				return errors.New("must be between 0 and 16")
			}
			return nil
		},
		Doc: "Messages one history page downloads at once. 0 keeps the built-in 4 for Gmail and 2 for Microsoft. Microsoft allows four requests at once per mailbox, so it never uses more than 4. Memory held by Gmail downloads stays bounded at any value.",
	})
	CaptureHistoryLookahead = config.Define(config.Spec[int]{
		Key: "capture.history_lookahead", Env: "MARGINCE_CAPTURE_HISTORY_LOOKAHEAD", Type: config.Int, Default: config.Some(0), Roles: bothRoles,
		Validate: func(value int) error {
			if value < 0 || value > 128 {
				return errors.New("must be between 0 and 128")
			}
			return nil
		},
		Doc: "How many messages a history page may download before the earlier ones are saved. 0 means twice the fetch workers; a value under the workers is raised to match.",
	})
	CaptureHistoryPageDelay = config.Define(config.Spec[time.Duration]{
		Key: "capture.history_page_delay", Env: "MARGINCE_CAPTURE_HISTORY_PAGE_DELAY", Type: config.Duration, Default: config.Some(time.Duration(0)), Roles: bothRoles,
		Validate: func(value time.Duration) error {
			if value < 0 || value > 10*time.Minute {
				return errors.New("must be between 0s and 10m")
			}
			return nil
		},
		Doc: "How long a history import waits after saving a page before it starts the next. 0 starts it at once. A longer wait lowers the load on the database and the worker and slows the import. Applies to Gmail and Microsoft.",
	})
	CaptureGmailMailboxQuota = config.Define(config.Spec[int]{
		Key: "capture.gmail_mailbox_quota", Env: "MARGINCE_CAPTURE_GMAIL_MAILBOX_QUOTA", Type: config.Int, Default: config.Some(6000), Roles: bothRoles,
		Validate: validateGmailQuota, Doc: "Effective Gmail quota units per minute per mailbox. Shared across live and history requests and OAuth apps; 100 units are kept free. Match the limit shown by Google.",
	})
	CaptureGmailProjectQuota = config.Define(config.Spec[int]{
		Key: "capture.gmail_project_quota", Env: "MARGINCE_CAPTURE_GMAIL_PROJECT_QUOTA", Type: config.Int, Default: config.Some(1200000), Roles: bothRoles,
		Validate: validateGmailQuota, Doc: "Effective Gmail quota units per minute across this installation's OAuth projects. Use the smallest project limit; 100 units are kept free. Use identical values in every process.",
	})
	CaptureHistoryHold = config.Define(config.Spec[bool]{
		Key: "capture.history_hold", Env: "MARGINCE_CAPTURE_HISTORY_HOLD", Type: config.Bool, Default: config.Some(false), Roles: bothRoles,
		Doc: "Accept queued history requests without starting another page. Stop older history workers before starting workers with a new admission protocol.",
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
		CaptureTracePayloads, CaptureMaxBackfillMonths, CaptureHistoryConcurrency, CaptureHistoryHold, CaptureHistoryPageSize, CaptureHistoryFetchWorkers, CaptureHistoryLookahead, CaptureHistoryPageDelay, CaptureGmailMailboxQuota, CaptureGmailProjectQuota, CaptureSkipReservedDomainProposals,
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
		TransactionalExtra:  config.Val(c, CaptureTransactionalExtra),
		TransactionalNever:  config.Val(c, CaptureTransactionalNever),
		TracePayloads:       config.Val(c, CaptureTracePayloads),
		MaxBackfillMonths:   config.Val(c, CaptureMaxBackfillMonths),
		HistoryConcurrency:  config.Val(c, CaptureHistoryConcurrency),
		HistoryHold:         config.Val(c, CaptureHistoryHold),
		HistoryPageSize:     config.Val(c, CaptureHistoryPageSize),
		HistoryFetchWorkers: config.Val(c, CaptureHistoryFetchWorkers),
		HistoryLookahead:    config.Val(c, CaptureHistoryLookahead),
		HistoryPageDelay:    config.Val(c, CaptureHistoryPageDelay),
		GmailMailboxQuota:   config.Val(c, CaptureGmailMailboxQuota),
		GmailProjectQuota:   config.Val(c, CaptureGmailProjectQuota),
		Logger:              log,

		SkipReservedDomainProposals: config.Val(c, CaptureSkipReservedDomainProposals),
	}
}

func validateGmailQuota(value int) error {
	if value < 1300 || value > 1_000_000_000 {
		return errors.New("must be between 1300 and 1000000000 quota units per minute")
	}
	return nil
}
