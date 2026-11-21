// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/platform/dbmigrate"
	"github.com/margince/margince/backend/migrations"
)

func TestTheEmptyRenewalStarterIsSwitchedOffAndNothingElseIs(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	resetSchema(t, conn)
	ctx := context.Background()

	core, err := migrations.CoreHistory()
	if err != nil {
		t.Fatalf("loading core: %v", err)
	}
	before := namespaceThroughVersion(t, core, "1795246200")
	if _, err := dbmigrate.Up(ctx, conn, before); err != nil {
		t.Fatalf("up: %v", err)
	}
	for _, row := range []struct{ name, key, params string }{
		{"empty", "renewal_reminder", `{}`},
		{"configured", "renewal_reminder", `{"object":"deal","date_field":"renews_on"}`},
		{"stage", "stage_change_notify", `{}`},
	} {
		if _, err := conn.Exec(ctx, `INSERT INTO automation (key, name, origin, trigger, action, params, enabled, tier)
			VALUES ($1, $2, 'catalog', '{"event_type":"test"}', '{"kind":"test"}', $3::jsonb, true, 'auto_execute')`, row.key, row.name, row.params); err != nil {
			t.Fatalf("seeding %s: %v", row.name, err)
		}
	}

	if _, err := dbmigrate.Up(ctx, conn, core); err != nil {
		t.Fatalf("migrating up: %v", err)
	}
	for name, want := range map[string]string{"empty": "false", "configured": "true", "stage": "true"} {
		if got := seedValue(t, conn, `SELECT enabled::text FROM automation WHERE name = $1`, name); got != want {
			t.Errorf("%s: enabled=%s, want %s", name, got, want)
		}
	}
	if got := seedValue(t, conn, `SELECT count(*)::text FROM audit_log WHERE entity_type = 'automation' AND actor_id = 'migration' AND action = 'pause'`); got != "1" {
		t.Errorf("audit rows = %s, want 1", got)
	}
}
