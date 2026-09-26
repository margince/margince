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

// A CLI whose call is abandoned is killed with everything it started: a child
// left holding the pipes would keep the call waiting out cliWaitDelay.
func TestALateCLIJudgeIsKilledWithItsChildren(t *testing.T) {
	withCredential(t)
	started := filepath.Join(t.TempDir(), "child-started")
	stubClaude(t, "sleep 30 &\n: > '"+started+"'\nwait")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// Cancel only once the child exists, so a slow host starting the script
	// cannot turn this into a test of nothing.
	var cancelled time.Time
	go func() {
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for range ticker.C {
			if _, err := os.Stat(started); err == nil {
				cancelled = time.Now()
				cancel()
				return
			}
		}
	}()
	_, err := claudeCLIJudge{model: "sonnet"}.Complete(ctx, judgeRequest())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want the cancellation", err)
	}
	// A surviving child holds the call for the whole cliWaitDelay past the
	// cancel; a killed group releases it at once.
	if held := time.Since(cancelled); held >= cliWaitDelay/2 {
		t.Errorf("the call was held %s after the cancel: the CLI's child outlived the kill and held its pipes", held)
	}
}
