// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

//gate:kind shape H2

package testdb

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
)

// TestResetReturnsTheInstallationModeToNative is the rail under
// resetInstallationSingletons, and it asserts BOTH halves of one claim because
// each half alone is a different bug.
//
// The row must SURVIVE the reset: deleting it would leave the installation with
// no system-of-record mode, and the next test's first write fails with "no rows
// in result set" — surfacing as a 500 on an unrelated request, with nothing
// pointing back here.
//
// And it must be BACK AT THE DEFAULT: a suite that flips the installation into
// overlay mode would otherwise leave it there for whatever ran next, which
// reads as an order-dependent flake and never as this function.
//
// Without this test neither half is checked. overlay_mode is excluded from both
// census queries in reset_integration_test.go — correctly, since it is spared
// the sweep — so nothing else here would notice if resetInstallationSingletons
// silently stopped running.
func TestResetReturnsTheInstallationModeToNative(t *testing.T) {
	ctx := context.Background()
	owner := ownerConn(t)

	if _, err := owner.Exec(ctx,
		`UPDATE overlay_mode SET sor_mode = 'overlay', incumbent = 'hubspot'`); err != nil {
		t.Fatalf("putting the installation into overlay mode: %v", err)
	}
	if err := Reset(ctx, owner); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	var mode string
	var incumbent *string
	if err := owner.QueryRow(ctx,
		`SELECT sor_mode, incumbent FROM overlay_mode`).Scan(&mode, &incumbent); err != nil {
		if err == pgx.ErrNoRows {
			t.Fatal("the reset left no overlay_mode row — the installation has no system-of-record mode, and the next test's first write fails somewhere else entirely")
		}
		t.Fatalf("reading the mode back: %v", err)
	}
	if mode != "native" || incumbent != nil {
		t.Errorf("mode after reset = %q/%v, want native/<nil> — an overlay-mode fixture outlives the reset and reaches whatever runs next", mode, incumbent)
	}
}
