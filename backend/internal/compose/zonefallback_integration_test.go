// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// zoneOnlyPostgresReads is a POSIX zone rule, three hours east of UTC. Postgres
// reads one and Go's zone database holds none, as with a name newer than the binary.
const zoneOnlyPostgresReads = "XYZ-3"

// plantStoredZone stores a name past the write path, which refuses it, the way
// a row written under newer zone data holds one.
func plantStoredZone(t *testing.T, e *integration.Env, name string) {
	t.Helper()
	e.WsExec(t, `UPDATE setting SET value = to_jsonb($1::text) WHERE key = 'installation.timezone'`, name)
}

// The pass runs at 22:30 UTC, which is already the next day in the stored
// zone. Its run is dated the UTC day, and the walk records the fault once.
func TestASweepWhoseStoredZoneWillNotLoadRunsInUTCAndRecordsOneFault(t *testing.T) {
	t.Parallel()
	e := integration.Setup(t)
	plantStoredZone(t, e, zoneOnlyPostgresReads)
	at := time.Date(2026, 3, 10, 22, 30, 0, 0, time.UTC)
	corrector := NewCloseDateCorrector(e.Pool, slog.New(slog.DiscardHandler)).SetClock(func() time.Time { return at })

	for range 2 {
		err := runEach(context.Background(), e.Pool, []ids.UUID{e.WS}, func(ctx context.Context, ws ids.UUID) error {
			return corrector.SweepWorkspace(principal.SystemActing(principal.WithWorkspaceID(ctx, ws), closeDateSweepActor))
		})
		if err != nil {
			t.Fatalf("the close-date pass stopped on a zone that will not load: %v", err)
		}
	}

	if got := e.WsScalar(t, `SELECT string_agg(DISTINCT as_of::text, ',') FROM close_date_run`); got != "2026-03-10" {
		t.Errorf("the passes are dated %q, want the UTC day 2026-03-10", got)
	}
	faults := e.WsCount(t, `SELECT count(*) FROM system_log
		WHERE action = $1 AND actor_id = 'system:installation-zone'
		  AND detail->'timezones' = to_jsonb(ARRAY[$2::text]) AND detail->>'applied' = 'UTC'`,
		storekit.ZoneUnresolvedAction, zoneOnlyPostgresReads)
	if faults != 2 {
		t.Errorf("two passes wrote %d fault rows naming the stored zone, want one per pass", faults)
	}
}

func TestAPassWhoseZoneLoadsRecordsNoFault(t *testing.T) {
	t.Parallel()
	e := integration.Setup(t)
	corrector := NewCloseDateCorrector(e.Pool, slog.New(slog.DiscardHandler))
	err := runEach(context.Background(), e.Pool, []ids.UUID{e.WS}, func(ctx context.Context, ws ids.UUID) error {
		return corrector.SweepWorkspace(principal.SystemActing(principal.WithWorkspaceID(ctx, ws), closeDateSweepActor))
	})
	if err != nil {
		t.Fatalf("the close-date pass failed: %v", err)
	}
	if faults := e.WsCount(t, `SELECT count(*) FROM system_log WHERE action = $1`, storekit.ZoneUnresolvedAction); faults != 0 {
		t.Errorf("a pass whose zone loaded wrote %d fault rows, want none", faults)
	}
}

// The admin's health page raises the finding while the stored zone will not
// load, and drops it once an admin saves a loadable zone.
func TestSystemHealthTellsTheAdminTheStoredZoneWillNotLoad(t *testing.T) {
	t.Parallel()
	e := integration.Setup(t)
	server := Server{jobHealthHandlers: jobHealthHandlers{pool: e.Pool}}
	raised := func() bool {
		t.Helper()
		_, findings, err := server.jobsSummary(e.Admin())
		if err != nil {
			t.Fatalf("reading background-work health: %v", err)
		}
		for _, finding := range findings {
			if finding.Code == "zone_unresolved" {
				return true
			}
		}
		return false
	}
	if raised() {
		t.Fatal("the finding is raised for a zone that loads")
	}
	plantStoredZone(t, e, zoneOnlyPostgresReads)
	if !raised() {
		t.Error("the stored zone will not load, and the health page raised no finding")
	}
	berlin := "Europe/Berlin"
	if _, err := NewInstallationSettingsFor(e.Pool).UpdateInstallation(e.Admin(), identity.InstallationPatch{Timezone: &berlin}); err != nil {
		t.Fatalf("an admin could not choose a zone again: %v", err)
	}
	if raised() {
		t.Error("the finding stayed after a loadable zone was stored")
	}
}

// Inside a watched pass the zone reader answers UTC for the unloadable name.
// Outside one it answers the stored name, which an interactive read refuses.
func TestAZoneReaderAnswersUTCOnlyInsideAWatchedPass(t *testing.T) {
	t.Parallel()
	e := integration.Setup(t)
	plantStoredZone(t, e, zoneOnlyPostgresReads)
	watched, _ := storekit.WatchZones(e.Admin())
	for want, ctx := range map[string]context.Context{storekit.FallbackZone: watched, zoneOnlyPostgresReads: e.Admin()} {
		if err := e.DB().Tx(ctx, func(tx pgx.Tx) error {
			applied, err := identity.TimezoneAppliedTx(ctx, tx)
			if applied != want {
				t.Errorf("the reader answers %q, want %q", applied, want)
			}
			return err
		}); err != nil {
			t.Fatalf("reading the installation zone: %v", err)
		}
	}
}
