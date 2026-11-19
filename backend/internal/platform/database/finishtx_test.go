// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package database

import (
	"context"
	"testing"
	"time"
)

// A context that can never end keeps its old terms: no detach and no budget.
func TestAContextThatCannotEndFinishesOnItsOwnTerms(t *testing.T) {
	ctx := context.Background()
	finish, cancel := finishContext(ctx, time.Unix(0, 0))
	defer cancel()

	if finish != ctx {
		t.Error("a context that cannot end was given a commit budget it never had")
	}
}

// A context that can end is detached, so its cancel cannot stop a COMMIT that
// has started.
func TestAFinishOutlivesItsCallersCancel(t *testing.T) {
	ctx, end := context.WithCancel(context.Background())
	finish, cancel := finishContext(ctx, time.Date(2999, 1, 1, 0, 0, 0, 0, time.UTC))
	defer cancel()
	end()

	if err := finish.Err(); err != nil {
		t.Errorf("the finishing context ended with its caller: %v", err)
	}
}

func TestAFinishEndsByTheRightDeadline(t *testing.T) {
	now := time.Unix(1_000, 0)
	writeBy := now.Add(time.Second)
	for _, tc := range []struct {
		name string
		ctx  func() (context.Context, context.CancelFunc)
		want time.Time
	}{
		{"a cancellable caller with no deadline", func() (context.Context, context.CancelFunc) {
			return context.WithCancel(context.Background())
		}, now.Add(commitBudget)},
		{"a job whose own deadline is later", func() (context.Context, context.CancelFunc) {
			return context.WithDeadline(context.Background(), now.Add(time.Hour))
		}, now.Add(time.Hour)},
		{"a caller whose deadline is sooner", func() (context.Context, context.CancelFunc) {
			return context.WithDeadline(context.Background(), now.Add(time.Second))
		}, now.Add(commitBudget)},
		{"a request with a write deadline", func() (context.Context, context.CancelFunc) {
			return context.WithDeadline(WithCommitDeadline(context.Background(), writeBy), now.Add(time.Hour))
		}, writeBy},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := tc.ctx()
			defer cancel()
			if got := finishBy(ctx, now); !got.Equal(tc.want) {
				t.Errorf("finish by %s, want %s", got, tc.want)
			}
		})
	}
}
