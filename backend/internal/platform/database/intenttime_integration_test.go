// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package database_test

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

func TestSQLAndTheValueObjectResolveTheSameLocalTime(t *testing.T) {
	raw, err := os.ReadFile("../../shared/kernel/values/testdata/intenttime.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct{ Wall, Zone, Instant string }
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) < 8 {
		t.Fatal("local-time cases are missing")
	}
	ctx, pool := laneEnv(t)
	onSnapshot(ctx, t, pool, func(ctx context.Context, tx pgx.Tx) {
		for _, c := range cases {
			zone, err := values.ParseTimezone(c.Zone)
			if err != nil {
				t.Fatal(err)
			}
			intent, err := values.ParseIntentTime(c.Wall, zone)
			if err != nil {
				t.Fatal(err)
			}
			fromGo, err := intent.Resolve()
			if err != nil {
				t.Fatal(err)
			}
			var fromSQL time.Time
			if err := tx.QueryRow(ctx, `SELECT public.resolve_intent_time(@wall::text::timestamp, @zone::text)`, pgx.NamedArgs{"wall": c.Wall, "zone": c.Zone}).Scan(&fromSQL); err != nil {
				t.Fatal(err)
			}
			want, err := time.Parse(time.RFC3339Nano, c.Instant)
			if err != nil {
				t.Fatal(err)
			}
			if !fromSQL.Equal(want) || !fromSQL.Equal(fromGo) {
				t.Errorf("%s in %s: SQL=%s Go=%s want=%s", c.Wall, c.Zone, fromSQL, fromGo, want)
			}
		}
	})
}

func TestTheIntentPairRefusesAMissingOrUnusableZone(t *testing.T) {
	ctx, pool := laneEnv(t)
	cases := []struct {
		Wall, Zone *string
		Valid      bool
	}{
		{nil, nil, true},
		{new("2026-03-29T02:30"), new("Europe/Berlin"), true},
		{new("2026-01-01T09:00"), nil, false},
		{nil, new("Europe/Berlin"), false},
		{new("2026-01-01T09:00"), new("Local"), false},
		{new("2026-01-01T09:00"), new("+01:00"), false},
		{new("2026-01-01T09:00"), new(" Europe/Berlin"), false},
		{new("infinity"), new("UTC"), false},
		{new("10000-01-01T09:00"), new("UTC"), false},
		{new("9999-12-31T23:59"), new("Etc/GMT+12"), false},
	}
	onSnapshot(ctx, t, pool, func(ctx context.Context, tx pgx.Tx) {
		for _, c := range cases {
			var valid bool
			if err := tx.QueryRow(ctx, `SELECT public.valid_intent_time(@wall::text::timestamp, @zone::text)`, pgx.NamedArgs{"wall": c.Wall, "zone": c.Zone}).Scan(&valid); err != nil {
				t.Fatal(err)
			}
			if valid != c.Valid {
				t.Errorf("pair wall=%v zone=%v: valid=%v want=%v", c.Wall, c.Zone, valid, c.Valid)
			}
		}
	})
}

func TestLegacyScheduleSavesKeepOnlyUnchangedLocalIntent(t *testing.T) {
	ctx, pool := laneEnv(t)
	cases := []struct{ NewWall, NewZone, NewInstant, Wall, Zone, Instant *string }{
		{nil, new("Europe/Berlin"), new("2026-07-01T08:00:00Z"), new("2026-07-01T09:00:00"), new("Europe/Berlin"), new("2026-07-01T07:00:00Z")},
		{nil, new("Europe/Berlin"), new("2026-07-01T07:00:00Z"), new("2026-07-01T09:00:00"), new("Europe/Berlin"), new("2026-07-01T07:00:00Z")},
		{nil, new("Europe/Berlin"), new("2026-07-01T09:00:00Z"), nil, nil, new("2026-07-01T09:00:00Z")},
		{new("2026-07-01T10:00:00"), new("Europe/Berlin"), nil, new("2026-07-01T10:00:00"), new("Europe/Berlin"), new("2026-07-01T08:00:00Z")},
		{nil, nil, nil, nil, nil, nil},
	}
	onSnapshot(ctx, t, pool, func(ctx context.Context, tx pgx.Tx) {
		for index, c := range cases {
			var wall, zone, instant *string
			err := tx.QueryRow(ctx, `SELECT to_char(wall_time, 'YYYY-MM-DD"T"HH24:MI:SS'), timezone,
     to_char(instant AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
     FROM public.update_intent_time('2026-07-01T09:00'::timestamp, 'Europe/Berlin', '2026-07-01T08:00Z'::timestamptz,
      @wall::text::timestamp, @zone::text, @instant::text::timestamptz)`, pgx.NamedArgs{"wall": c.NewWall, "zone": c.NewZone, "instant": c.NewInstant}).Scan(&wall, &zone, &instant)
			if err != nil {
				t.Fatal(err)
			}
			if !equalOptionalText(wall, c.Wall) || !equalOptionalText(zone, c.Zone) || !equalOptionalText(instant, c.Instant) {
				t.Errorf("schedule change %d lost or invented local intent", index)
			}
		}
	})
}

func equalOptionalText(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}
