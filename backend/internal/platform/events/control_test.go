// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package events

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"
)

// The subscription is reopened after every drop, the delay grows while
// subscriptions keep failing before they confirm, and a confirmed subscription
// restarts the count.
func TestSuperviseResetResubscribesAfterTheSubscriptionDrops(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	var delays []time.Duration
	sleep := func(_ context.Context, d time.Duration) error {
		delays = append(delays, d)
		return nil
	}
	attempt := 0
	run := func(_ context.Context, ready chan<- struct{}) error {
		attempt++
		switch attempt {
		case 1, 2:
			return errors.New("dial refused")
		case 3:
			close(ready)
			return errSubscriptionClosed
		case 4:
			return errors.New("dial refused")
		}
		cancel()
		return context.Canceled
	}

	superviseReset(ctx, log, run, sleep)

	if attempt != 5 {
		t.Fatalf("subscribed %d times, want 5: one per drop, then the stop", attempt)
	}
	if len(delays) != 4 {
		t.Fatalf("slept %d times, want 4", len(delays))
	}
	if delays[1] <= delays[0] {
		t.Errorf("delays %v: a second failure in a row must wait longer than the first", delays)
	}
	if delays[2] >= delays[1] {
		t.Errorf("delays %v: a confirmed subscription must restart the count, so the next wait is short", delays)
	}
}

func TestSuperviseResetStopsWhenTheContextEndsDuringTheBackoff(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	attempts := 0
	run := func(context.Context, chan<- struct{}) error {
		attempts++
		return errors.New("dial refused")
	}
	sleep := func(context.Context, time.Duration) error {
		cancel()
		return context.Canceled
	}

	superviseReset(ctx, log, run, sleep)

	if attempts != 1 {
		t.Errorf("subscribed %d times after the context ended, want 1", attempts)
	}
}
