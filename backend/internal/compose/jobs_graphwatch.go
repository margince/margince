// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The Microsoft Graph subscription-renewal pair, kept apart from the Gmail pull
// it sits beside in the runner.
//
// Its own file rather than a section of jobs_capture.go: the two share a
// registry and nothing else, and this pass answers a question the Gmail one
// never asks — whether a notification URL exists to renew a subscription
// against.

import (
	"log/slog"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/jobs"
)

// addGraphWatchJobs registers the Microsoft Graph subscription-renewal pair.
//
// Its own function rather than a branch inside the Gmail block: the two share a
// registry and nothing else, and the Graph pass takes a SECOND condition of its
// own — a notification URL. A subscription registered against no URL notifies
// nobody, so a deployment that never opted into Outlook push keeps the poll.
func addGraphWatchJobs(reg *jobRegistry, cfg JobRunnerConfig, log *slog.Logger) {
	if !GraphWatchWillRun(cfg.GmailRegistry, cfg.GraphWatch) {
		return
	}
	addDeclaredWorker[GraphWatchArgs](reg, &graphWatchWorker{
		registry: cfg.GmailRegistry, renewWithin: cfg.GraphWatch.RenewWithin, log: log,
	})
	addDeclaredWorker[GraphWatchRenewArgs](reg, &graphWatchRenewWorker{
		registry: cfg.GmailRegistry, notificationURL: cfg.GraphWatch.NotificationURL,
	})
}

// GraphWatchWillRun reports whether this build registers the Graph
// subscription-renewal pair.
//
// A NOTIFICATION URL IS NOT ENOUGH, and a connector to renew through is the
// other half. Both workers resolve the Graph connector out of the registry, so
// without one every renewal fails on a connector that was never registered —
// for every Outlook connection, on every scan, forever. A pass that can only
// fail is worse than no pass: the poll still runs either way, and the difference
// is a fleet-wide error rate an operator has to explain.
//
// The registry is the thing asked, rather than the environment that helped build
// it, because an app configured through the product registers a connector no
// environment variable names — and the two roles have already come apart over
// exactly that once.
//
// EXPORTED so the boot banner can ask the same question rather than restate it.
// A banner naming a lane that did not come up is worse than no banner: it is the
// one place an operator looks to check.
//
// ASKED OF THE DECLARATION, not of the fields directly. The periodic schedule
// gates on api/jobs.yaml's registration.when through the same `registers`, so a
// condition spelled only here would be one the scheduler does not know about —
// and it enqueues under its own answer. That divergence does not fail loudly:
// the rows are inserted, no worker claims them, and the lane looks idle rather
// than broken.
func GraphWatchWillRun(reg *capture.Registry, cfg GraphWatchConfig) bool {
	spec, declared := jobs.SpecFor(graphWatchKind)
	if !declared {
		panic("compose: api/jobs.yaml does not declare " + graphWatchKind)
	}
	return registers(JobRunnerConfig{GmailRegistry: reg, GraphWatch: cfg}, spec.Registration)
}

// registryOffers reports whether reg holds a connector for provider.
//
// Asked of the registry rather than of the config that built it: what decides
// whether a job can run is the connector it will look up, and the two have
// already disagreed once — a stored app registers a connector no environment
// variable names.
func registryOffers(reg *capture.Registry, provider string) bool {
	for _, desc := range reg.Connectors() {
		if desc.Name == provider {
			return true
		}
	}
	return false
}
