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
