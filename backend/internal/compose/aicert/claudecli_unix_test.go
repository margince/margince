// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build !windows

package aicert

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A CLI that runs past its deadline is killed with everything it started: a
// child left holding the pipes would keep the call waiting out cliWaitDelay.
func TestALateCLIJudgeIsKilledWithItsChildren(t *testing.T) {
	withCredential(t)
	started := filepath.Join(t.TempDir(), "child-started")
	stubClaude(t, "sleep 30 &\n: > '"+started+"'\nwait")
	// Long enough for a freshly written script to start on a slow host; the
	// assertion below is about what happens after the deadline, not before it.
	const deadline = 2 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), deadline)
	defer cancel()
	begun := time.Now()
	_, err := claudeCLIJudge{model: "sonnet"}.Complete(ctx, judgeRequest())
	elapsed := time.Since(begun)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want the deadline", err)
	}
	if _, statErr := os.Stat(started); statErr != nil {
		t.Fatalf("the stub never started its child before the deadline, so this proved nothing: %v", statErr)
	}
	// A surviving child holds the call for the whole cliWaitDelay past the
	// deadline; a killed group releases it at once.
	if elapsed >= deadline+cliWaitDelay/2 {
		t.Errorf("the call took %s: the CLI's child outlived the kill and held its pipes", elapsed)
	}
}
