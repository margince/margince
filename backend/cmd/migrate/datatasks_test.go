// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"io"
	"testing"

	"github.com/margince/margince/backend/internal/platform/datatask"
)

func TestDataTaskCommandRejectsUnknownActionsBeforeConnecting(t *testing.T) {
	for _, args := range [][]string{nil, {"apply"}, {"status", "--wait"}, {"execute", "--max-batches", "0"}} {
		if err := runDataTasks(t.Context(), args, io.Discard, io.Discard); err == nil {
			t.Fatalf("invalid command accepted: %v", args)
		}
	}
}

func TestDataTaskWaitDistinguishesPausedFailedAndIncomplete(t *testing.T) {
	for _, status := range []string{datatask.Pending, datatask.Running, datatask.Done, datatask.Paused, datatask.Failed} {
		done, err := dataTasksFinished([]datatask.Progress{{TaskID: "core/100_fix", Step: "first", Status: status}})
		if done != (status == datatask.Done) || (err != nil) != (status == datatask.Paused || status == datatask.Failed) {
			t.Fatalf("status%s done%v error%v", status, done, err)
		}
	}
}

func TestDataTaskWaitReportsReleaseAndSchemaBlocks(t *testing.T) {
	for _, reason := range []string{"missing_prerequisite", "newer_release_required"} {
		done, err := dataTasksFinished([]datatask.Progress{{Status: datatask.Done, BlockedReason: reason}})
		if done || err == nil {
			t.Fatalf("block %s reported done=%v err=%v", reason, done, err)
		}
	}
}
