// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

// The run-level re-drive: which failures a run is driven again for, how long it
// waits between attempts, and what a run that never got past them becomes.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/modules/ai"
)

// runAttempts is how many times one run is driven before its task is given up.
//
// The router itself retries nothing: ai.attemptLadder walks each bound rung
// exactly once, and the cert lane binds ONE model to every rung, so a dropped
// connection burns the whole ladder in milliseconds and returns. Three attempts
// here is what stands between a transient fault and discarding every run the
// task had already paid for.
const runAttempts = 3

// runRetryBackoff is the wait before each re-drive, indexed by the attempt
// about to be made. It rises because the fault worth retrying — a connection
// dropped under an idle HTTP/2 ping, a broker shedding load — clears on its own
// timescale rather than instantly, and an immediate re-drive usually just buys
// the same failure at the price of another call.
var runRetryBackoff = [runAttempts - 1]time.Duration{2 * time.Second, 8 * time.Second}

// runThrottleBackoff is the wait when the provider is RATE LIMITING this
// installation rather than having dropped a connection. A throttle clears on
// the window the provider is enforcing, not on the timescale above: three
// attempts spaced 2s and 8s land inside one saturated window, so the ladder
// buys three refusals at the price of three calls and the task is abandoned
// with no record — which the certification page then shows as untested, a word
// reserved for an honest gap in what has been measured.
//
// Single digits to tens of seconds because that is the order of the windows
// these providers enforce. Still a guess: the provider often NAMES the moment
// to come back in Retry-After, and honouring that would beat any table here.
// ai.providerRefusal already reads the header to classify the refusal, but the
// value reaches no caller — carrying it out would change that module's error
// contract, so it is its own change rather than a rider on this one.
var runThrottleBackoff = [runAttempts - 1]time.Duration{30 * time.Second, 90 * time.Second}

// backoffFor is the table the NEXT wait comes from, chosen by what the last
// attempt failed with. The two faults ask for different waits and the error
// already says which it is, so the choice is read rather than configured.
func backoffFor(err error) [runAttempts - 1]time.Duration {
	if errors.Is(err, ai.ErrProviderThrottled) {
		return runThrottleBackoff
	}
	return runRetryBackoff
}

// sleepFunc is this file's injectable delay, the seam a test swaps so a retry
// path is exercised without a real wait — the same pattern runner.go's nowFunc
// uses, and for the same reason: a test that slept for real would be the sort of
// clock-dependent flake this repo forbids.
var sleepFunc = func(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}

// driveRun drives one run, re-driving it whole when the router came back having
// failed on every bound rung.
//
// The retry is at RUN granularity and not call granularity because a run is the
// smallest thing this lane can repeat honestly: a site may turn a multi-turn
// conversation or a whole tool loop, and there is no resuming one of those from
// the middle. Re-driving costs one run; the alternative — what happens today —
// costs every run the task had already paid for.
//
// Only an exhausted ladder is retried. A validator failure, a mixed-model
// refusal or a caps miss is a measurement, and repeating it until it reads
// better is how a certification lane starts lying.
//
// A candidate that broke off its answer on every attempt is a measurement too:
// the model was reached and could not finish, so the run is scored invalid and
// the task keeps its record. It is re-driven first because one such break can
// be a transient crash; only every attempt breaking off is evidence.
func driveRun(ctx context.Context, candidate *ai.Router, candidateRec *traceRecorder, judge *ai.Router, judgeRec *traceRecorder,
	sc Scenario, task ai.Task, census *aitasks.Registry, log *slog.Logger, trace *payloadTrace, journal taskJournal, run int,
) (runOutcome, error) {
	var lastErr, lastOutage error
	var brokenOff runOutcome
	for attempt := 1; attempt <= runAttempts; attempt++ {
		if attempt > 1 {
			log.WarnContext(ctx, "aicert: re-driving a run after the router exhausted every bound tier — the calls the failed attempt made are paid for and discarded",
				"task", string(task), "scenario", sc.Name, "run", run, "attempt", attempt, "err", lastErr)
			if err := sleepFunc(ctx, backoffFor(lastErr)[attempt-2]); err != nil {
				return runOutcome{}, errors.Join(err, lastErr)
			}
		}
		outcome, err := runOnce(ctx, candidate, candidateRec, judge, judgeRec, sc, task, census, log, trace, run, attempt)
		switch {
		case err == nil && !outcome.Abandoned:
			return outcome, nil
		case err == nil:
			brokenOff, lastErr = outcome, errAnswerBrokenOff
		case !worthRedriving(err):
			return runOutcome{}, err
		default:
			lastErr, lastOutage = err, err
		}
	}
	if lastOutage == nil {
		log.WarnContext(ctx, "aicert: the candidate broke off its answer on every attempt — the run is scored invalid",
			"task", string(task), "scenario", sc.Name, "run", run)
		return brokenOff, nil
	}
	return runOutcome{}, fmt.Errorf(
		"every bound tier failed on all %d attempts — re-run the same command once the provider is reachable%s: %w",
		runAttempts, journal.restartHint(), lastOutage,
	)
}

// worthRedriving reports whether err is the router having exhausted its ladder,
// which is the one failure a later attempt could get past.
//
// An exhausted ACCOUNT is excluded by the sentinel itself rather than by a
// second test here: ai.attemptLadder stops that walk at the refusing rung and
// returns the refusal alone, never ErrAllTiersFailed, so a spending cap can
// never be retried into — and one place decides what "the ladder ran out" means.
// A throttle keeps the sentinel and stays retryable, because backoff is exactly
// what it asks for. A withheld answer and a rejected request never carry it:
// the ladder returns an outcome bare. A preference no host meets may, as the
// last rung's cause, and is excluded: it fails every attempt alike.
func worthRedriving(err error) bool {
	return errors.Is(err, ai.ErrAllTiersFailed) && !errors.Is(err, ai.ErrNoUpstreamHost)
}

// errAnswerBrokenOff is what a re-drive logs and backs off for after an attempt
// whose candidate broke off its answer.
var errAnswerBrokenOff = errors.New("the candidate broke off its answer")
