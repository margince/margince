// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestForecastCaptureFailureKeepsItsWrittenLanguage(t *testing.T) {
	e := setupForecast(t)
	store := newForecastStoreFor(e.Pool)
	config := settings.New(e.Pool, settings.NewRegistry(identity.BaseLanguage))
	writer := principal.WithCorrelationID(principal.SystemActing(principal.WithWorkspaceID(context.Background(), e.WS), "system:capture-language"), ids.NewV7())
	at := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	// Corrupt persisted configuration exercises the worker's failed-capture path.
	if _, err := e.owner.Exec(t.Context(), `UPDATE setting SET value = '"Margince/Nowhere"'::jsonb WHERE key = 'installation.timezone'`); err != nil {
		t.Fatal(err)
	}
	worker := &forecastSnapshotSweepWorker{pool: e.Pool, now: func() time.Time { return at }, log: slog.New(slog.DiscardHandler)}
	for _, tc := range []struct{ language, words string }{{"en", "Capture failed"}, {"de", "Erfassung ist fehlgeschlagen"}, {"vi", "Không thể ghi nhận"}} {
		if err := settings.Set(reportingActor(e), config, identity.BaseLanguage, tc.language); err != nil {
			t.Fatal(err)
		}
		if err := worker.freeze(writer, e.WS); err == nil {
			t.Fatal("invalid stored timezone must fail capture")
		}
		if err := settings.Set(reportingActor(e), config, identity.BaseLanguage, "en"); err != nil {
			t.Fatal(err)
		}
		if err := store.InTx(reportingActor(e), func(ctx context.Context, tx pgx.Tx) error {
			status, err := store.CaptureStatusTx(ctx, tx, forecasting.Scope{Kind: "workspace"}, nil)
			if err != nil {
				return err
			}
			if status == nil || status.Failure == nil || !strings.Contains(*status.Failure, tc.words) || !status.LastAttemptAt.Equal(at) || !status.NextCaptureAt.Equal(at.Add(24*time.Hour)) {
				t.Fatalf("%s changed stored failure or retry facts: %+v", tc.language, status)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
}
