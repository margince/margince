// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package events

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A held relay ships nothing. A committed row waits in the outbox, where no
// stream cap can trim it, and goes out once the hold lifts.
func TestAPausedRelayLeavesTheOutboxUntilItResumes(t *testing.T) {
	e := setup(t)
	env := e.stage(t, "contact.created", ids.NewV7())

	var held atomic.Bool
	held.Store(true)
	asked := make(chan struct{})
	relay := e.relay(t).WithPause(func(ctx context.Context) bool {
		select {
		case asked <- struct{}{}:
		case <-ctx.Done():
		}
		return held.Load()
	})
	relay.pauseEvery = time.Millisecond
	relay.interval = time.Millisecond

	runCtx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		relay.Run(runCtx)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
	awaitAsk := func() {
		t.Helper()
		select {
		case <-asked:
		case <-done:
			t.Fatal("Run returned before asking whether it is held")
		case <-time.After(10 * time.Second):
			t.Fatal("a running relay never asked whether it is held")
		}
	}

	// Two asks while held: the loop went round once and shipped nothing.
	awaitAsk()
	awaitAsk()
	if got := e.streamEventIDs(t, "gw:events:crm:contact"); len(got) != 0 {
		t.Fatalf("a held relay shipped %v", got)
	}

	held.Store(false)
	// The first ask after the hold lifts lets one pass run; the next comes
	// only once that pass has finished.
	awaitAsk()
	awaitAsk()
	got := e.streamEventIDs(t, "gw:events:crm:contact")
	if len(got) != 1 || got[0] != env.EventID.String() {
		t.Fatalf("after the hold lifted the stream carries %v, want [%s]", got, env.EventID)
	}
}
