// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The release guard's second half: the one that runs while the role does.
//
// AssertInstallationRelease answers at BOOT, which catches the case it was
// written for — a torn tag pull, where every role is new and every role boots.
// It cannot catch the other shape: one role restarting alone. Roll only the api
// and its entrypoint migrates and records the new release; the worker that was
// already running keeps the outbox relay, the retention evaluator and the agent
// runner pointed at a schema and an event contract that are not its own,
// indefinitely, with nothing logged and nothing crash-looping. That is exactly
// "half of one release beside half of another", which is the state the guard
// exists to prevent, and the next unrelated restart is when it surfaces.
//
// So the record is re-read on a tick, and a confirmed difference stops the
// process — non-zero, so the orchestrator restarts it into the visible crash
// loop AssertInstallationRelease already produces. This watcher never decides
// anything the boot guard would not; it only notices later.
//
// TWO RULES MAKE IT SAFE TO RUN, and both are about what it must NOT do.
//
// It must not exit on an unreadable record. A momentary database error is not a
// release change, and a guard that turned one into a process exit would be an
// outage lever pointed at the deployment it protects — the failure mode strictly
// worse than the one it prevents. Only a KNOWN difference stops the process; a
// read that failed is logged and the next tick asks again.
//
// It must not turn a rolling api deploy into worker churn. During a rollout, api
// replicas from two releases are alive at once and each records its own on boot,
// so the recorded value flaps between them for the length of the rollout. A
// single differing read is therefore not evidence of anything, and this waits
// for a run of them. The counter resets on any agreeing read, so a flap can
// never accumulate across a rollout — it only fires when the record has settled
// somewhere else.

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/buildinfo"
)

// ReleaseRecheckInterval is how often a running role re-reads the installation's
// release.
//
// Minutes rather than seconds: the event this watches for is a deploy, which no
// operator measures in seconds, and the cost of noticing it a minute later is
// one minute of a mixed pair that has already been running since the deploy
// began. Seconds would buy nothing and add a query per role per second for the
// life of every process.
const ReleaseRecheckInterval = time.Minute

// releaseSkewConfirmations is how many consecutive differing reads stop the
// process.
//
// Three, and the number is doing work rather than decorating. One is a flap
// during a rolling api deploy. The run has to outlast the window in which two
// api releases are alive — at ReleaseRecheckInterval that is three minutes of a
// record that never once agrees with this role, which no rollout produces and a
// completed deploy always does.
const releaseSkewConfirmations = 3

// WatchInstallationRelease re-reads the installation's recorded release on a
// tick and answers, exactly once, when it has settled on a different one.
//
// The channel carries the refusal AssertInstallationRelease would have given at
// boot — the same sentence, because it is the same fact reaching an operator at
// a different moment. It closes with nothing when ctx ends, which is the
// ordinary shutdown.
//
// A role with no comparable release version, or an installation nothing has
// recorded yet, watches nothing: the boot guard says so in its log once, and a
// watcher that repeated it every minute would be noise about a state that is
// not a fault.
func WatchInstallationRelease(
	ctx context.Context, pool *pgxpool.Pool, log *slog.Logger, version string, every time.Duration,
) <-chan error {
	return watchRelease(ctx, log, version, every, func(ctx context.Context) (string, error) {
		return recordedRelease(ctx, pool)
	})
}

// watchRelease is WatchInstallationRelease over any reader, which is what makes
// the rules above testable without a deploy: the flap, the read failure and the
// settled difference are three sequences of answers rather than three
// installations.
func watchRelease(
	ctx context.Context,
	log *slog.Logger,
	version string,
	every time.Duration,
	read func(context.Context) (string, error),
) <-chan error {
	skew := make(chan error, 1)
	if !buildinfo.Comparable(version) {
		close(skew)
		return skew
	}
	go func() {
		defer close(skew)
		tick := time.NewTicker(every)
		defer tick.Stop()
		differing := 0
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
			}
			installation, err := read(ctx)
			if err != nil {
				// Not a difference, and deliberately not a reset either: a read
				// that failed says nothing about the record, so it neither
				// confirms a skew nor clears one. What it must not do is end the
				// process.
				log.Warn("release guard: could not re-read the installation's release", "err", err)
				continue
			}
			if refuseMixedRelease(version, installation) == nil {
				differing = 0
				continue
			}
			differing++
			if differing < releaseSkewConfirmations {
				log.Warn("release guard: the installation's release differs from this role's",
					"release_version", version, "installation_release", installation,
					"confirmations", differing, "needed", releaseSkewConfirmations)
				continue
			}
			skew <- fmt.Errorf("the installation's release changed while this role was running: %w",
				refuseMixedRelease(version, installation))
			return
		}
	}()
	return skew
}

// recordedRelease is the read AssertInstallationRelease makes, without the
// bootstrap logging: a running role has already been told once.
func recordedRelease(ctx context.Context, pool *pgxpool.Pool) (string, error) {
	ctx, bootstrapped, err := bootLedgerScope(ctx, pool, "system:release-version")
	if err != nil {
		return "", fmt.Errorf("resolving the installation: %w", err)
	}
	if !bootstrapped {
		// Not an error and not a difference: an installation that is not
		// bootstrapped has recorded nothing, which refuseMixedRelease reads as
		// nothing to compare.
		return "", nil
	}
	var recorded string
	if err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		version, err := lastObservedRelease(ctx, tx)
		recorded = version
		return err
	}); err != nil {
		return "", err
	}
	return recorded, nil
}
