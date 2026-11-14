// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package replayguard

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/redistest"
)

func TestAReleasedIDCanBeUsedOnceMore(t *testing.T) {
	guard := New(redistest.Client(t))
	ctx := t.Context()
	for i, want := range []bool{true, false} {
		if first, err := guard.FirstUse(ctx, "notice-1", time.Hour); err != nil || first != want {
			t.Fatalf("use %d: FirstUse = %v, %v; want %v", i+1, first, err, want)
		}
	}
	if err := guard.Release(ctx, "notice-1"); err != nil {
		t.Fatal(err)
	}
	if first, err := guard.FirstUse(ctx, "notice-1", time.Hour); err != nil || !first {
		t.Errorf("after Release: FirstUse = %v, %v; want the id taken again", first, err)
	}
}
