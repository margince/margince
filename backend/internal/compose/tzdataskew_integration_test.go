// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
)

// Zones whose rules have moved, so the sentinel has teeth on an installation
// that applies UTC. Chatham and Lord Howe shift by part of an hour, Tehran
// abandoned its transitions, and Santiago has moved its dates more than once.
var skewProneZones = []string{
	"Europe/Berlin",
	"America/New_York",
	"Australia/Lord_Howe",
	"Pacific/Chatham",
	"Asia/Tehran",
	"America/Santiago",
}

// hourlyOffsetsRead is how many hourly steps fourteen months is, within a day
// either way. That span covers a transition in either hemisphere, and a
// reading far below it covered none, whatever it found.
const hourlyOffsetsRead = 10000

// TestGoAndPostgresBucketAnInstantAlike is the cross-engine tzdata sentinel.
//
// Which day an instant falls on is decided twice: in Go for the close-date
// sweep, and in SQL for the forecast report. Each engine reads its own tzdata
// copy, on its own update schedule.
//
// Where one copy moved a transition and the other did not, one instant falls
// on two days, and neither reader reports a fault.
func TestGoAndPostgresBucketAnInstantAlike(t *testing.T) {
	e := integration.Setup(t)
	from := time.Now().UTC().Truncate(time.Hour)
	transitioned := 0

	for _, zone := range skewProneZones {
		loc, err := time.LoadLocation(zone)
		if err != nil {
			t.Errorf("Go cannot load %s, which Postgres is about to be asked for: %v", zone, err)
			continue
		}
		if compareZoneOffsets(t, e, zone, loc, from) > 1 {
			transitioned++
		}
	}
	// Every zone could stop transitioning, or the window could land between
	// transitions, and the agreement above would then only ever compare one
	// offset per zone. The sentinel says so rather than reporting that pass.
	if transitioned == 0 {
		t.Error("no zone changed offset in the window, so no transition was compared")
	}
}

// compareZoneOffsets fails on the first instant the two engines place
// differently, naming it, because a count of disagreements says nothing about
// which transition moved. It answers how many offsets the zone held, which is
// above one where the window covered a transition.
func compareZoneOffsets(
	t *testing.T, e *integration.Env, zone string, loc *time.Location, from time.Time,
) int {
	t.Helper()
	ctx := e.Admin()
	seen := 0
	offsets := map[int64]struct{}{}
	err := e.DB().Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT ts, EXTRACT(EPOCH FROM (ts AT TIME ZONE $1) - (ts AT TIME ZONE 'UTC'))::bigint
			  FROM generate_series($2::timestamptz, $2::timestamptz + interval '14 months',
			                       interval '1 hour') ts`, zone, from)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var at time.Time
			var offset int64
			if err := rows.Scan(&at, &offset); err != nil {
				return err
			}
			seen++
			offsets[offset] = struct{}{}
			if _, want := at.In(loc).Zone(); int64(want) != offset {
				t.Fatalf("%s at %s: Go offset %ds, Postgres %ds. The tzdata copies disagree",
					zone, at.UTC().Format(time.RFC3339), want, offset)
			}
		}
		return rows.Err()
	})
	if err != nil {
		t.Fatalf("asking Postgres for %s offsets: %v", zone, err)
	}
	// A census that reads nothing reports a pass, so the hours are counted.
	if seen < hourlyOffsetsRead {
		t.Errorf("%s: read %d hourly offsets, too few to cover a transition", zone, seen)
	}
	return len(offsets)
}
