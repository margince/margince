// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

type rotatingFailurePull struct {
	fixtureConnector
	sink    connector.CredentialSink
	failure error
}

func (*rotatingFailurePull) Descriptor() connector.Descriptor {
	return connector.Descriptor{Name: haltingProvider, Version: "fixture"}
}

func (c *rotatingFailurePull) WithCredentialSink(sink connector.CredentialSink) connector.Connector {
	bound := *c
	bound.sink = sink
	return &bound
}

func (c *rotatingFailurePull) Sync(ctx context.Context, _ connector.Auth, prior connector.Cursor, _ connector.Sink) (connector.Cursor, error) {
	if err := c.sink.Rotated(ctx, connector.Auth("rotated-fixture-token")); err != nil {
		return nil, err
	}
	return prior, c.failure
}

func TestAPullsOwnTokenRotationKeepsItsFailureAndBackoff(t *testing.T) {
	for _, tc := range []struct {
		name    string
		failure error
		class   string
	}{
		{"limited", &connector.RateLimitedError{RetryAfter: time.Hour}, "rate_limited"},
		{"unreachable", connector.ErrUnreachable, "unreachable"},
		{"rejected", connector.ErrAuthRejected, "auth"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, reg, _, _ := newCaptureRegistryFixture(t)
			at := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
			reg.WithClock(func() time.Time { return at })
			reg.Register(&rotatingFailurePull{failure: tc.failure})
			if err := syncOnceOf(ctx, t, reg, haltingProvider); !errors.Is(err, tc.failure) {
				t.Fatalf("rotated pull = %v, want %v", err, tc.failure)
			}
			views, err := reg.Connections(ctx)
			if err != nil || len(views) != 1 {
				t.Fatalf("connections = %+v, %v", views, err)
			}
			view := views[0]
			if view.LastSuccessAt != nil || view.LastSyncedAt == nil || *view.LastSyncedAt != at || view.LastErrorClass == nil || *view.LastErrorClass != tc.class || view.NextSyncDueAt == nil {
				t.Fatalf("rotation lost failure scheduling: %+v", view)
			}
			if tc.class == "rate_limited" {
				owner, _ := setupCaptureDB(t)
				var seconds float64
				err := owner.QueryRow(ctx, `SELECT extract(epoch FROM (s.next_sync_at - l.occurred_at))
					FROM capture_sync_state s JOIN system_log l ON l.detail->>'connection_id'=@text
					WHERE s.connection_id=@id AND l.action='capture_sync_error'`,
					pgx.NamedArgs{"id": view.ID, "text": view.ID.String()}).Scan(&seconds)
				if err != nil || seconds != time.Hour.Seconds() {
					t.Fatalf("recorded Retry-After = %v seconds, %v", seconds, err)
				}
			}
			if tc.class == "auth" && view.Status != "reauth_required" {
				t.Fatalf("rotated rejected token was not parked: %s", view.Status)
			}
		})
	}
}
