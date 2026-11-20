// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The SQL day start is the first instant of the day, as storekit.StartOfDay
// answers it. That holds at a fall-back midnight, where a bare AT TIME ZONE
// picks the later of two instants. It also holds at a spring-forward midnight
// that does not exist.
func TestTheSQLDayStartIsTheFirstInstantOfTheDay(t *testing.T) {
	e := Setup(t)
	for _, tc := range []struct{ zone, day string }{
		{"America/Havana", "2026-11-01"},
		{"America/Havana", "2026-03-08"},
		{"America/Santiago", "2026-09-06"},
		{"Australia/Lord_Howe", "2026-04-05"},
		{"America/New_York", "2026-11-01"},
		{"UTC", "2026-11-01"},
	} {
		e.WsExec(t, `UPDATE setting SET value = to_jsonb($1::text) WHERE key = 'installation.timezone'`, tc.zone)
		loc, err := time.LoadLocation(tc.zone)
		if err != nil {
			t.Fatalf("%s: %v", tc.zone, err)
		}
		day, err := time.Parse(time.DateOnly, tc.day)
		if err != nil {
			t.Fatalf("%s: %v", tc.day, err)
		}
		want := storekit.StartOfDay(time.Date(day.Year(), day.Month(), day.Day(), 12, 0, 0, 0, loc), loc)
		var got time.Time
		ctx := principal.WithWorkspaceID(context.Background(), e.WS)
		if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT `+occurredday.DayStartSQL(`$1::date`), tc.day).Scan(&got)
		}); err != nil {
			t.Fatalf("%s %s: %v", tc.zone, tc.day, err)
		}
		if !got.Equal(want) {
			t.Errorf("%s on %s: SQL day start %v, want %v", tc.zone, tc.day, got.UTC(), want.UTC())
		}
	}
}
