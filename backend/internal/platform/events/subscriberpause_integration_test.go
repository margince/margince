// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package events

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	kevents "github.com/margince/margince/backend/internal/shared/kernel/events"
)

// A held group reads nothing, so an entry published while it is held waits in
// the stream and is delivered once the hold lifts.
func TestAPausedSubscriberDeliversNothingUntilItResumes(t *testing.T) {
	ctx, rdb := purgeTestRedis(t)
	group := kevents.Groups()[0]

	var held atomic.Bool
	held.Store(true)
	asked := make(chan struct{}, 64)
	delivered := make(chan kevents.Envelope, 1)
	sub := NewSubscriber(rdb, group, func(_ context.Context, env kevents.Envelope) error {
		delivered <- env
		return nil
	}, slog.New(slog.NewTextHandler(io.Discard, nil))).WithPause(func(context.Context) bool {
		select {
		case asked <- struct{}{}:
		default:
		}
		return held.Load()
	})
	sub.pauseEvery = time.Millisecond

	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- sub.Run(runCtx) }()
	t.Cleanup(func() {
		cancel()
		if err := <-done; err != nil && !errors.Is(err, context.Canceled) {
			t.Errorf("Run: %v", err)
		}
	})

	// Run's error goes back on done for the cleanup to read.
	awaitAsk := func() {
		t.Helper()
		select {
		case <-asked:
		case err := <-done:
			done <- err
			t.Fatalf("Run returned before asking whether it is held: %v", err)
		case <-time.After(10 * time.Second):
			t.Fatal("a running subscriber never asked whether it is held")
		}
	}

	awaitAsk()
	publishProbe(ctx, t, rdb, group.Streams[0])
	// Three more asks after the publish: the loop went round while held and
	// read nothing on any of those turns.
	for range 3 {
		awaitAsk()
	}
	select {
	case env := <-delivered:
		t.Fatalf("a held subscriber delivered %s", env.Type)
	default:
	}

	held.Store(false)
	select {
	case <-delivered:
	case <-time.After(10 * time.Second):
		t.Fatal("the entry published while held was never delivered after the hold lifted")
	}
}
