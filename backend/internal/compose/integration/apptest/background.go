// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package apptest

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// AdminContext binds the installation's workspace and a seat holding perms, for
// the product writers a background job calls and no route does. The caller
// passes the admin grants, which live in package integration.
func AdminContext(t *testing.T, e *AppEnv, perms principal.Permissions) context.Context {
	t.Helper()
	wsID := InstallationWorkspaceUUID(context.Background(), t, e.Pool)
	user := ids.NewV7()
	ctx := principal.WithCorrelationID(principal.WithWorkspaceID(context.Background(), wsID), ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user, Permissions: perms,
	})
}

// FinishInputCheck records one completed nightly input check by a seat holding
// perms, through the store's three writers in the order the checker uses them.
func FinishInputCheck(t *testing.T, e *AppEnv, perms principal.Permissions) {
	t.Helper()
	admin := AdminContext(t, e, perms)
	store := assurance.NewStore(compose.InstallationDB(e.DB().Pool()))
	asOf := clocktest.Now(t).UTC()
	if err := store.InTx(admin, func(ctx context.Context, tx pgx.Tx) error {
		run, err := store.StartRun(ctx, tx, asOf, nil)
		if err != nil {
			return err
		}
		if err := store.RecordCoverage(ctx, tx, run, assurance.SourceCoverage{
			Source: "mail", State: assurance.CoverageChecked, CheckedThrough: &asOf,
		}); err != nil {
			return err
		}
		if err := store.RecordCoverage(ctx, tx, run, assurance.SourceCoverage{
			Source: "offers", State: assurance.CoverageUnavailable,
		}); err != nil {
			return err
		}
		return store.FinishRun(ctx, tx, run, 1, 0, 0, assurance.StatusComplete, "ready")
	}); err != nil {
		t.Fatalf("recording a finished input check: %v", err)
	}
}

// FreezeAt takes one workspace-wide forecast snapshot of the quarter holding
// `at`, by a seat holding perms, and answers its id.
func FreezeAt(t *testing.T, e *AppEnv, perms principal.Permissions, at time.Time) string {
	t.Helper()
	admin := AdminContext(t, e, perms)
	store := forecasting.NewStore(compose.InstallationDB(e.DB().Pool()))
	var id ids.UUID
	if err := InWorkspace(e, t, func(tx pgx.Tx) error {
		period, base, err := compose.ForecastPeriodAt(admin, tx, forecasting.PeriodQuarter, at)
		if err != nil {
			return err
		}
		id, err = store.TakeSnapshot(admin, tx, forecasting.NewSnapshot{
			Period: period, Scope: forecasting.Scope{Kind: forecasting.ScopeWorkspace},
			Trigger: forecasting.TriggerRecheck, BaseCurrency: base, TakenAt: at,
		})
		return err
	}); err != nil {
		t.Fatalf("freezing a forecast: %v", err)
	}
	return id.String()
}

// DriveNoticeCase runs the notice-case consumer for one contact, as the worker
// would.
func DriveNoticeCase(t *testing.T, e *AppEnv, contactID string) {
	t.Helper()
	id, err := ids.Parse(contactID)
	if err != nil {
		t.Fatalf("parsing the contact id: %v", err)
	}
	consumer := compose.NewNoticeCaseOpen(e.Pool, func() time.Time { return clocktest.Now(t) }, slog.New(slog.DiscardHandler))
	if err := consumer.HandleEvent(context.Background(), events.Envelope{
		Type:   "contact.created",
		Entity: events.EntityRef{Type: "contact", ID: id},
	}); err != nil {
		t.Fatalf("recording the duty: %v", err)
	}
}
