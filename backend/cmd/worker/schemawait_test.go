// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

// A rolling deploy starts this role beside the api that migrates. New job code
// reading a column its migration adds fails until that lands, and a job that
// fails three times is discarded. These hold that the worker waits for its
// schema before it starts any lane or job.

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

var errBehind = errors.New("the database is behind this binary")

func TestAWorkerWaitsUntilTheSchemaIsAtHead(t *testing.T) {
	t.Parallel()
	var asked atomic.Int32
	atHead := func(context.Context) error {
		if asked.Add(1) < 3 {
			return errBehind
		}
		return nil
	}
	if !awaitSchema(context.Background(), atHead, time.Millisecond, slog.New(slog.DiscardHandler)) {
		t.Fatal("awaitSchema gave up although the schema reached head")
	}
	if got := asked.Load(); got != 3 {
		t.Fatalf("asked %d times, want 3: it must keep asking until the migration lands, then stop", got)
	}
}

func TestAWorkerAtHeadStartsWithoutWaiting(t *testing.T) {
	t.Parallel()
	var asked atomic.Int32
	atHead := func(context.Context) error { asked.Add(1); return nil }
	if !awaitSchema(context.Background(), atHead, time.Hour, slog.New(slog.DiscardHandler)) {
		t.Fatal("awaitSchema refused a schema already at head")
	}
	if got := asked.Load(); got != 1 {
		t.Fatalf("asked %d times, want 1", got)
	}
}

// A shutdown while waiting ends the wait rather than starting the lanes.
func TestAShutdownWhileWaitingStartsNothing(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	atHead := func(context.Context) error { cancel(); return errBehind }
	if awaitSchema(ctx, atHead, time.Millisecond, slog.New(slog.DiscardHandler)) {
		t.Fatal("awaitSchema reported the schema ready after a shutdown, with the database still behind")
	}
}

// The wait has to sit before every phase that does work. Like the lane lifetime
// test beside it, this reads run's body, because run needs a database and a bus
// before it reaches these calls.
func TestTheWorkerWaitsForItsSchemaBeforeItStartsLanesOrJobs(t *testing.T) {
	t.Parallel()
	body := runBody(t, "main.go", "run")
	waitAt := indexOfStatement(body, func(s string) bool { return strings.Contains(s, "observeUntilAtHead(") })
	if waitAt < 0 {
		t.Fatal("run() no longer waits for its schema, so new job code can run against the old one and be discarded")
	}
	for _, phase := range []string{"startEventLanes(", "startJobRunner("} {
		at := indexOfStatement(body, func(s string) bool { return strings.Contains(s, phase) })
		if at < 0 {
			t.Fatalf("run() no longer calls %s, so this gate is about a shape that is gone", phase)
		}
		if at < waitAt {
			t.Errorf("run() calls %s at statement %d, before it waits for its schema at %d", phase, at, waitAt)
		}
	}
	waits := runBody(t, "schemawait.go", "observeUntilAtHead")
	if indexOfStatement(waits, func(s string) bool { return strings.Contains(s, "awaitSchema(") }) < 0 {
		t.Fatal("observeUntilAtHead no longer calls awaitSchema, so run() starts its lanes without waiting")
	}
}

// A release refusal cancels the boot's context, which also ends the wait. The
// early return must still answer the refusal, or the worker would exit zero
// past the crash loop the release guard relies on.
func TestAReleaseRefusalWhileWaitingStillExitsNonZero(t *testing.T) {
	t.Parallel()
	body := runBody(t, "main.go", "run")
	waitAt := indexOfStatement(body, func(s string) bool { return strings.Contains(s, "observeUntilAtHead(") })
	lanesAt := indexOfStatement(body, func(s string) bool { return strings.Contains(s, "startEventLanes(") })
	if waitAt < 0 || lanesAt < waitAt {
		t.Fatal("run() no longer waits for its schema before its lanes, so this gate is about a shape that is gone")
	}
	for _, stmt := range body[waitAt+1 : lanesAt] {
		if strings.Contains(stmt, "releaseSkewRefusal(") {
			return
		}
	}
	t.Error("run() returns from an ended schema wait without answering releaseSkewRefusal, " +
		"so a release refusal during the wait exits zero")
}
