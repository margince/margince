// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture_test

import (
	"context"
	"errors"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func TestFailedAttemptsDoNotAdvanceSuccessfulMailCoverage(t *testing.T) {
	t.Parallel()
	ctx, reg, _, _ := newCaptureRegistryFixture(t)
	recovered := true
	reg.Register(haltingConnector{recovered: &recovered})
	at := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	good := at
	reg.SetClock(func() time.Time { return at })
	if err := syncOnceOf(ctx, t, reg, haltingProvider); err != nil {
		t.Fatal(err)
	}
	recovered = false
	at = at.Add(time.Hour)
	if err := reg.SyncOnce(ctx, connectionOf(ctx, t)); !errors.Is(err, connector.ErrUnreachable) {
		t.Fatalf("failed sync = %v", err)
	}
	coverage, err := reg.MailCoverage(ctx, []string{haltingProvider}, &good, at)
	if err != nil {
		t.Fatal(err)
	}
	if coverage.State != crmcontracts.MailCaptureCoverageStateUnavailable || coverage.LastSuccessAt == nil || !coverage.LastSuccessAt.Equal(good) {
		t.Fatalf("coverage = %+v", coverage)
	}
	concerns, err := reg.HealthConcerns(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(concerns) != 1 || concerns[0].LastSyncedAt == nil || !concerns[0].LastSyncedAt.Equal(good) {
		t.Fatalf("last good health sync = %+v", concerns)
	}
	views, err := reg.Connections(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(views) != 1 || views[0].LastSyncedAt == nil || !views[0].LastSyncedAt.Equal(at) {
		t.Fatalf("last attempt = %+v", views)
	}
}

type lifecycleSync struct {
	fixtureConnector
	during func() error
	result error
}

func (lifecycleSync) Descriptor() connector.Descriptor {
	return connector.Descriptor{Name: haltingProvider, Version: "fixture"}
}

func (c lifecycleSync) Sync(context.Context, connector.Auth, connector.Cursor, connector.Sink) (connector.Cursor, error) {
	if err := c.during(); err != nil {
		return nil, err
	}
	return connector.Cursor(`{"watermark":"old-grant"}`), c.result
}

func TestSupersededSyncDoesNotWriteSuccessOrFailureOntoANewGrant(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		disconnect bool
		result     error
	}{
		{"disconnected_success", true, nil},
		{"disconnected_failure", true, connector.ErrAuthRejected},
		{"reauthenticated_failure", false, connector.ErrAuthRejected},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, reg, _, _ := newCaptureRegistryFixture(t)
			reg.Register(lifecycleSync{result: tc.result, during: func() error {
				if tc.disconnect {
					if err := reg.Disconnect(ctx, capture.ProviderMailbox(haltingProvider)); err != nil {
						return err
					}
				}
				_, err := reg.Connect(ctx, haltingProvider, connector.Auth("replacement-token"))
				return err
			}})
			if err := syncOnceOf(ctx, t, reg, haltingProvider); !errors.Is(err, tc.result) {
				t.Fatalf("sync = %v, want %v", err, tc.result)
			}
			views, err := reg.Connections(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if len(views) != 1 || views[0].Status != "connected" || views[0].LastSuccessAt != nil || views[0].LastErrorClass != nil || len(views[0].Cursor) != 0 {
				t.Fatalf("old sync wrote onto new grant: %+v", views)
			}
		})
	}
}
