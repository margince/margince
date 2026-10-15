// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package programsettings

import (
	"errors"
	"time"

	"github.com/margince/margince/backend/internal/platform/config"
)

// DefaultJobDrainWindow is how long a job caught by shutdown gets to finish.
// An operator sizes the pod's termination grace period against the drain, the
// cancel wait and the teardown; twenty seconds keeps them inside thirty.
const DefaultJobDrainWindow = 20 * time.Second

// DefaultAITaskDir is gitignored, because a fetched page or a real fixture
// carries whatever its source carried. Relative to the repo root; the make
// target passes an absolute path.
const DefaultAITaskDir = ".tmp/aitask"

// The settings only the worker reads.
var (
	GmailPubsubTopic = config.Define(config.Spec[string]{
		Key: "gmail_pubsub_topic", Env: "MARGINCE_GMAIL_PUBSUB_TOPIC", Flag: "gmail-pubsub-topic", Type: config.String, Roles: workerOnly,
		Doc: "Gmail Pub/Sub topic (projects/<p>/topics/<t>) that enables the push-watch job; unset leaves capture on the poll.",
	})
	GraphNotificationURL = config.Define(config.Spec[string]{
		Key: "graph_notification_url", Env: "MARGINCE_GRAPH_NOTIFICATION_URL", Flag: "graph-notification-url", Type: config.String, Roles: workerOnly,
		Doc: "Public URL, token included, that Microsoft posts Graph change notifications to; unset leaves Outlook capture on the poll.",
	})
	JobDrainWindow = config.Define(config.Spec[time.Duration]{
		Key: "job_drain_window", Env: "MARGINCE_JOB_DRAIN_WINDOW", Flag: "job-drain-window", Type: config.Duration,
		Default: config.Some(DefaultJobDrainWindow), Validate: positiveWindow, Roles: workerOnly,
		Doc: "How long a running job gets to finish at shutdown; the grace period must cover it plus 5s and the teardown.",
	})
	CertLogBaseURL = config.Define(config.Spec[string]{
		Key: "certlog_base_url", Env: "MARGINCE_CERTLOG_BASE_URL", Flag: "certlog-base-url", Type: config.String, Roles: workerOnly,
		Doc: "Certificate-transparency base URL, or public for crt.sh; unset makes no outbound lookups of what a company runs.",
	})
	ObserveAddr = config.Define(config.Spec[string]{
		Key: "observe_addr", Env: "MARGINCE_OBSERVE_ADDR", Flag: "observe-addr", Type: config.String, Roles: workerOnly,
		Doc: "Address to serve the worker's /healthz, /readyz and /metrics on, such as 127.0.0.1:9101; unset serves nothing.",
	})
	ObservePprof = config.Define(config.Spec[bool]{
		Key: "observe_pprof", Env: "MARGINCE_OBSERVE_PPROF", Flag: "observe-pprof", Type: config.Bool, Default: config.Some(false), Roles: workerOnly,
		Doc: "Mount the unauthenticated pprof handlers under /debug/pprof/ on the --observe-addr listener; turn it on only to diagnose.",
	})
	AITaskDir = config.Define(config.Spec[string]{
		Key: "work_dir", Env: "MARGINCE_AITASK_DIR", Flag: "work-dir", Type: config.NonEmpty, Default: config.Some(DefaultAITaskDir), Roles: workerOnly,
		Doc: "Directory the worker aitask probes write their artifacts to.",
	})
)

// Zero would make River's stop hard: every running job cancelled the moment
// the shutdown signal arrives, which is what the window prevents.
func positiveWindow(d time.Duration) error {
	if d <= 0 {
		return errors.New("must be a positive duration")
	}
	return nil
}

// ObserveFault refuses pprof with no listener: there is nothing to mount the
// handlers on, and the operator who set it believes it did something.
func ObserveFault(c *config.Config) error {
	if config.Val(c, ObservePprof) && config.Val(c, ObserveAddr) == "" {
		return errors.New("MARGINCE_OBSERVE_PPROF=true serves profiles on the observe listener, " +
			"and MARGINCE_OBSERVE_ADDR is unset so there is none: set an address, or drop the pprof setting")
	}
	return nil
}
