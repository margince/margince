// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ratelimit

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestWeightedPacingSharesBudgetsAndRefusalsSpendNothing(t *testing.T) {
	now := time.Unix(0, 0)
	registry := NewRegistry()
	first, second := registry.NewPacer(func() time.Time { return now }), registry.NewPacer(func() time.Time { return now })
	project := Budget{Key: "project", PerMinute: 1200000, Cost: 60}
	mailbox := Budget{Key: "mailbox", PerMinute: 6000, Cost: 60}
	if wait, _, err := first.Reserve(t.Context(), project, mailbox); wait != 0 || err != nil {
		t.Fatalf("first: %v %v", wait, err)
	}
	for range 100 {
		if wait, _, err := second.Reserve(t.Context(), project, mailbox); wait != 600*time.Millisecond || err != nil {
			t.Fatalf("probe changed wait: %v %v", wait, err)
		}
	}
	now = now.Add(3 * time.Millisecond)
	other := Budget{Key: "other-mailbox", PerMinute: 6000, Cost: 20}
	if wait, _, err := second.Reserve(t.Context(), project, other); wait != 0 || err != nil {
		t.Fatalf("refused mailbox spent project: %v %v", wait, err)
	}
	now = now.Add(597 * time.Millisecond)
	if wait, _, err := second.Reserve(t.Context(), project, mailbox); wait != 0 || err != nil {
		t.Fatalf("shared budget did not reopen: %v %v", wait, err)
	}
}

func TestCancelledPacingDoesNotConsumeQuota(t *testing.T) {
	now := time.Unix(0, 0)
	meter := NewRegistry().NewPacer(func() time.Time { return now })
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	budget := Budget{Key: "mailbox", PerMinute: 6000, Cost: 60}
	if _, _, err := meter.Reserve(ctx, budget); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel: %v", err)
	}
	if wait, _, err := meter.Reserve(t.Context(), budget); wait != 0 || err != nil {
		t.Fatalf("cancel spent quota: %v %v", wait, err)
	}
}

func TestWeightedPacingRejectsInvalidBudgets(t *testing.T) {
	meter := NewPacer()
	for _, budget := range []Budget{{Key: "", PerMinute: 100, Cost: 1}, {Key: "mail", PerMinute: 0, Cost: 1}, {Key: "mail", PerMinute: 100, Cost: 101}, {Key: "mail", PerMinute: 100, Cost: -1}} {
		if _, _, err := meter.Reserve(t.Context(), budget); err == nil {
			t.Fatalf("accepted %+v", budget)
		}
	}
}

func TestProviderPauseIsSharedAndCannotBeShortened(t *testing.T) {
	now := time.Unix(0, 0)
	registry := NewRegistry()
	first, second := registry.NewPacer(func() time.Time { return now }), registry.NewPacer(func() time.Time { return now })
	if err := first.Pause(t.Context(), "mailbox", 2*time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := second.Pause(t.Context(), "mailbox", time.Second); err != nil {
		t.Fatal(err)
	}
	budget := Budget{Key: "mailbox", PerMinute: 6000, Cost: 60}
	if wait, key, err := second.Reserve(t.Context(), budget); wait != 2*time.Minute || key != pacedKey("mailbox") || err != nil {
		t.Fatalf("shared pause %v %q %v", wait, key, err)
	}
	other := Budget{Key: "another-mailbox", PerMinute: 6000, Cost: 60}
	if wait, _, err := second.Reserve(t.Context(), other); wait != 0 || err != nil {
		t.Fatalf("mailbox pause blocked another mailbox: %v %v", wait, err)
	}
	now = now.Add(2 * time.Minute)
	if wait, _, err := second.Reserve(t.Context(), budget); wait != 0 || err != nil {
		t.Fatalf("pause did not expire: %v %v", wait, err)
	}
}
