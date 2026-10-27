// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

type overlappingPull struct {
	fixtureConnector
	started     int
	finishNewer func() error
	olderError  error
	newerCursor connector.Cursor
	newerError  error
}

func (*overlappingPull) Descriptor() connector.Descriptor {
	return connector.Descriptor{Name: haltingProvider, Version: "fixture"}
}

func (c *overlappingPull) Sync(context.Context, connector.Auth, connector.Cursor, connector.Sink) (connector.Cursor, error) {
	c.started++
	if c.started == 1 {
		if err := c.finishNewer(); err != nil {
			return nil, err
		}
		return connector.Cursor(`{"watermark":"older"}`), c.olderError
	}
	return c.newerCursor, c.newerError
}

func TestAnOlderPullCannotReplaceANewerSyncOutcome(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		next connector.Cursor
	}{
		{"progress_success", nil, connector.Cursor(`{"watermark":"newer"}`)},
		{"progress_unreachable", connector.ErrUnreachable, connector.Cursor(`{"watermark":"newer"}`)},
		{"progress_rejected", connector.ErrAuthRejected, connector.Cursor(`{"watermark":"newer"}`)},
		{"unchanged_success", nil, nil},
		{"unchanged_unreachable", connector.ErrUnreachable, nil},
		{"unchanged_rejected", connector.ErrAuthRejected, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, reg, _, _ := newCaptureRegistryFixture(t)
			at := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
			newerAt := at.Add(time.Minute)
			reg.WithClock(func() time.Time { return at })
			reg.Register(&overlappingPull{olderError: tc.err, newerCursor: tc.next, finishNewer: func() error {
				at = newerAt
				err := reg.SyncOnce(ctx, connectionOf(ctx, t))
				at = newerAt.Add(time.Minute)
				return err
			}})
			if err := syncOnceOf(ctx, t, reg, haltingProvider); !errors.Is(err, tc.err) {
				t.Fatalf("older pull = %v, want %v", err, tc.err)
			}
			views, err := reg.Connections(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if len(views) != 1 || views[0].LastSuccessAt == nil || !views[0].LastSuccessAt.Equal(newerAt) || views[0].LastErrorClass != nil || views[0].Status != "connected" {
				t.Fatalf("older pull replaced newer health: %+v", views)
			}
			if len(tc.next) == 0 {
				if len(views[0].Cursor) != 0 {
					t.Fatalf("unchanged cursor replaced: %s", views[0].Cursor)
				}
				return
			}
			var cursor map[string]string
			if err := json.Unmarshal(views[0].Cursor, &cursor); err != nil {
				t.Fatal(err)
			}
			if cursor["watermark"] != "newer" {
				t.Fatalf("stored cursor = %v", cursor)
			}
		})
	}
}

func TestACompletedPullCanRecoverAFailureFromItsStartingSnapshot(t *testing.T) {
	ctx, reg, _, _ := newCaptureRegistryFixture(t)
	at := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	reg.WithClock(func() time.Time { return at })
	reg.Register(&overlappingPull{newerError: connector.ErrUnreachable, finishNewer: func() error {
		at = at.Add(time.Minute)
		if err := reg.SyncOnce(ctx, connectionOf(ctx, t)); !errors.Is(err, connector.ErrUnreachable) {
			t.Fatalf("overlapping failure = %v", err)
		}
		at = at.Add(time.Minute)
		return nil
	}})
	if err := syncOnceOf(ctx, t, reg, haltingProvider); err != nil {
		t.Fatal(err)
	}
	views, err := reg.Connections(ctx)
	if err != nil || len(views) != 1 {
		t.Fatalf("connections = %+v, %v", views, err)
	}
	view := views[0]
	if view.LastSuccessAt == nil || *view.LastSuccessAt != at || view.LastErrorClass != nil || view.Status != "connected" {
		t.Fatalf("failure prevented successful recovery: %+v", view)
	}
	var cursor map[string]string
	if err := json.Unmarshal(view.Cursor, &cursor); err != nil {
		t.Fatal(err)
	}
	if cursor["watermark"] != "older" {
		t.Fatalf("completed pull lost its cursor: %v", cursor)
	}
}
