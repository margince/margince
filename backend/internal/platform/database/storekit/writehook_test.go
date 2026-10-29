// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
)

// A write the hook makes itself runs no hook, so a hook cannot recurse.
func TestAWriteHookDoesNotRunForItsOwnWrites(t *testing.T) {
	previous := writeHook.Load()
	t.Cleanup(func() { writeHook.Store(previous) })
	calls := 0
	SetWriteHook(func(ctx context.Context, tx pgx.Tx, w AuditedWrite) error {
		calls++
		return runWriteHook(ctx, tx, w)
	})
	if err := runWriteHook(context.Background(), nil, AuditedWrite{EntityType: "deal", Action: "create"}); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("hook ran %d times, want once", calls)
	}
}
