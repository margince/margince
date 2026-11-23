// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package briefs

// A brief is the person's own list, so its local_day follows the person's zone
// and falls back to the installation's. The SQL spelling the overnight pass
// uses must name the same day as the Go one the store stamps runs with.

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
)

func TestABriefIsFiledUnderThePersonsOwnDay(t *testing.T) {
	b := setupBrief(t)
	b.WsExec(t, `UPDATE setting SET value = '"Europe/Berlin"'::jsonb WHERE key = 'installation.timezone'`)
	b.WsExec(t, `UPDATE app_user SET timezone = 'Asia/Ho_Chi_Minh' WHERE id = $1`, b.Rep1)
	// 20:30 on the 15th in Berlin, 01:30 on the 16th in Ho Chi Minh City.
	at := time.Date(2026, 6, 15, 18, 30, 0, 0, time.UTC)

	run, err := b.engine.SnapshotRun(b.repCtx, at)
	if err != nil {
		t.Fatal(err)
	}
	if got := run.LocalDay.Format(time.DateOnly); got != "2026-06-16" {
		t.Errorf("run local_day = %s, want the person's own day 2026-06-16", got)
	}
	sql := b.WsScalar(t, `SELECT `+occurredday.DaySQL(occurredday.Personal, "$1::timestamptz", "$2::uuid")+`::text`, at, b.Rep1)
	if sql != "2026-06-16" {
		t.Errorf("the SQL day for the person = %s, want 2026-06-16", sql)
	}
	sql = b.WsScalar(t, `SELECT `+occurredday.DaySQL(occurredday.Combined, "$1::timestamptz", "$2::uuid")+`::text`, at, b.Rep1)
	if sql != "2026-06-15" {
		t.Errorf("the SQL day for a combined screen = %s, want the installation's 2026-06-15", sql)
	}
	b.WsExec(t, `UPDATE app_user SET timezone = NULL WHERE id = $1`, b.Rep1)
	sql = b.WsScalar(t, `SELECT `+occurredday.DaySQL(occurredday.Personal, "$1::timestamptz", "$2::uuid")+`::text`, at, b.Rep1)
	if sql != "2026-06-15" {
		t.Errorf("a person with no zone reads the installation's day, got %s", sql)
	}
}

func (b *briefEnv) runsFiled(t *testing.T) int {
	t.Helper()
	return b.WsCount(t, `SELECT count(*) FROM brief_run WHERE user_id = $1`, b.Rep1)
}

// A zone change moves the date label without moving the real day. West to east
// would file tomorrow's run early, east to west a second run for today. Either
// way one edition covers the real day, and a later day still gets its own.
func TestAZoneChangeFilesOneBriefPerRealDayInBothDirections(t *testing.T) {
	for name, c := range map[string]struct{ from, to string }{
		"moving east": {"America/Los_Angeles", "Asia/Ho_Chi_Minh"},
		"moving west": {"Asia/Ho_Chi_Minh", "America/Los_Angeles"},
	} {
		t.Run(name, func(t *testing.T) {
			b := setupBrief(t)
			b.WsExec(t, `UPDATE app_user SET timezone = $2 WHERE id = $1`, b.Rep1, c.from)
			first := time.Date(2026, 6, 15, 20, 0, 0, 0, time.UTC)
			run, err := b.engine.SnapshotRun(b.repCtx, first)
			if err != nil {
				t.Fatal(err)
			}

			b.WsExec(t, `UPDATE app_user SET timezone = $2 WHERE id = $1`, b.Rep1, c.to)
			again, err := b.engine.SnapshotRun(b.repCtx, first.Add(time.Hour))
			if err != nil {
				t.Fatal(err)
			}
			if again.ID != run.ID || b.runsFiled(t) != 1 {
				t.Fatalf("a zone change an hour later filed another run: %d runs, want the one edition", b.runsFiled(t))
			}

			next, err := b.engine.SnapshotRun(b.repCtx, first.Add(48*time.Hour))
			if err != nil {
				t.Fatal(err)
			}
			if next.ID == run.ID || b.runsFiled(t) != 2 {
				t.Fatalf("two days later no new run was filed: %d runs, want one per real day", b.runsFiled(t))
			}
		})
	}
}
