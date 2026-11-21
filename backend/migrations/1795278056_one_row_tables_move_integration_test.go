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

// The upgrade keeps what the five one-row tables and the bar table held.
// A rollback returns each row to its table.
func TestTheOneRowTablesAndTheBarsMoveAndComeBack(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	resetSchema(t, conn)
	ctx := context.Background()

	core, err := migrations.Core()
	if err != nil {
		t.Fatalf("loading core: %v", err)
	}
	if _, err := dbmigrate.Up(ctx, conn, namespaceThroughVersion(t, core, "1795276613")); err != nil {
		t.Fatalf("up to the migration before: %v", err)
	}
	for _, seed := range []string{
		`INSERT INTO activity_clock_refold_mark (passed_up_to, passed_id)
		 VALUES ('2026-05-04T09:30:00.123456Z', '00000000-0000-7000-8000-000000000001')`,
		`INSERT INTO activity_waiting_horizon (horizon_days, measured_at) VALUES (12, '2026-05-04T09:30:00Z')`,
		`INSERT INTO capture_answered_sweep_cursor (after_contact_id) VALUES (NULL)`,
		`INSERT INTO capture_party_admission_cursor (after_activity, after_user, ceiling)
		 VALUES ('00000000-0000-7000-8000-000000000002', '00000000-0000-7000-8000-000000000003', '00000000-0000-7000-8000-000000000004')`,
		`INSERT INTO deal_authorship_sweep_cursor (after_evidence_id) VALUES ('00000000-0000-7000-8000-000000000005')`,
		`INSERT INTO app_user (id, email, display_name) VALUES ('00000000-0000-7000-8000-00000000000a', 'rep@bars.test', 'Rep')`,
		`INSERT INTO weekly_review (id, user_id, local_week_start, as_of) VALUES
		 ('00000000-0000-7000-8000-00000000000b', '00000000-0000-7000-8000-00000000000a', '2026-05-04', '2026-05-11T06:00:00Z'),
		 ('00000000-0000-7000-8000-00000000000c', '00000000-0000-7000-8000-00000000000a', '2026-05-11', '2026-05-18T06:00:00Z')`,
		`INSERT INTO weekly_review_movement (weekly_review_id, period_kind, bar, delta_minor) VALUES
		 ('00000000-0000-7000-8000-00000000000b', 'quarter', 'won', 6000),
		 ('00000000-0000-7000-8000-00000000000b', 'month', 'not_checked', -9007199254740991)`,
	} {
		if _, err := conn.Exec(ctx, seed); err != nil {
			t.Fatalf("seeding %s: %v", seed, err)
		}
	}

	moved := namespaceThroughVersion(t, core, "1795278059")
	if _, err := dbmigrate.Up(ctx, conn, moved); err != nil {
		t.Fatalf("migrating up: %v", err)
	}
	state := `SELECT value::text FROM installation_state WHERE key = `
	for _, c := range []struct{ name, query, want string }{
		// The two instants are compared as instants: their text follows the session's zone.
		{"the refold mark", `SELECT (value->>'passed_id') || ' ' || extract(epoch FROM (value->>'passed_up_to')::timestamptz)::text
			FROM installation_state WHERE key = 'activity_clock_refold_mark'`, "00000000-0000-7000-8000-000000000001 1777887000.123456"},
		{"the waiting horizon", `SELECT (value->>'horizon_days') || ' ' || extract(epoch FROM (value->>'measured_at')::timestamptz)::text
			FROM installation_state WHERE key = 'activity_waiting_horizon'`, "12 1777887000.000000"},
		{"the answered-contact place", state + `'capture_answered_sweep_cursor'`, `{"after_contact_id": null}`},
		{
			"the party recovery place", state + `'capture_party_admission_cursor'`,
			`{"ceiling": "00000000-0000-7000-8000-000000000004", "after_user": "00000000-0000-7000-8000-000000000003", "after_activity": "00000000-0000-7000-8000-000000000002"}`,
		},
		{
			"the authorship place", state + `'deal_authorship_sweep_cursor'`,
			`{"after_evidence_id": "00000000-0000-7000-8000-000000000005"}`,
		},
		{
			"the bars of the week that froze some", `SELECT outlook_movement::text FROM weekly_review WHERE local_week_start = '2026-05-04'`,
			`[{"bar": "not_checked", "delta_minor": -9007199254740991, "period_kind": "month"}, {"bar": "won", "delta_minor": 6000, "period_kind": "quarter"}]`,
		},
		{"the week that froze none", `SELECT coalesce(outlook_movement::text, 'none') FROM weekly_review WHERE local_week_start = '2026-05-11'`, "none"},
	} {
		if got := scalar(t, conn, c.query); got != c.want {
			t.Errorf("%s moved as %s, want %s", c.name, got, c.want)
		}
	}

	if _, err := dbmigrate.Down(ctx, conn, moved, 2); err != nil {
		t.Fatalf("rolling back: %v", err)
	}
	for _, c := range []struct{ name, query, want string }{
		{
			"the refold mark", `SELECT passed_id::text || ' ' || extract(epoch FROM passed_up_to)::text FROM activity_clock_refold_mark`,
			"00000000-0000-7000-8000-000000000001 1777887000.123456",
		},
		{"the waiting horizon", `SELECT horizon_days::text FROM activity_waiting_horizon`, "12"},
		{"the answered-contact place", `SELECT coalesce(after_contact_id::text, 'start') FROM capture_answered_sweep_cursor`, "start"},
		{
			"the party recovery place", `SELECT after_activity::text || ' ' || after_user::text || ' ' || ceiling::text FROM capture_party_admission_cursor`,
			"00000000-0000-7000-8000-000000000002 00000000-0000-7000-8000-000000000003 00000000-0000-7000-8000-000000000004",
		},
		{"the authorship place", `SELECT after_evidence_id::text FROM deal_authorship_sweep_cursor`, "00000000-0000-7000-8000-000000000005"},
		{
			"the bars", `SELECT string_agg(period_kind || ':' || bar || ':' || delta_minor, ',' ORDER BY period_kind) FROM weekly_review_movement`,
			"month:not_checked:-9007199254740991,quarter:won:6000",
		},
	} {
		if got := scalar(t, conn, c.query); got != c.want {
			t.Errorf("after the rollback %s reads %s, want %s", c.name, got, c.want)
		}
	}
}
