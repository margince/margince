// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// NarrowHistoryBatch runs one batch in workspace of the background history narrowing, as
// the job does, and answers whether the workspace had one pending.
func NarrowHistoryBatch(t *testing.T, workspace ids.UUID, registry *capture.Registry) bool {
	t.Helper()
	ctx := principal.WithWorkspaceID(context.Background(), workspace)
	ctx = principal.WithActor(ctx, principal.Principal{Type: principal.PrincipalSystem, ID: "system:history_narrowing"})
	worked, err := registry.NarrowPendingHistory(principal.WithCorrelationID(ctx, ids.NewV7()))
	if err != nil {
		t.Fatalf("narrowing a batch of history: %v", err)
	}
	return worked
}

// NarrowHistoryToTheEnd runs the background history narrowing until nothing is
// pending, and answers how many batches did work.
func NarrowHistoryToTheEnd(t *testing.T, workspace ids.UUID, registry *capture.Registry) int {
	t.Helper()
	for batches := 0; batches < 100; batches++ {
		if !NarrowHistoryBatch(t, workspace, registry) {
			return batches
		}
	}
	t.Fatal("the history narrowing did not end within 100 batches")
	return 0
}
