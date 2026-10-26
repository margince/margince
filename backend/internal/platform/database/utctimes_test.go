// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package database

import (
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// A timestamptz scans in UTC, not the process zone, alone or in an array.
// Location UTC is a pointer the default codec never answers, whatever time.Local is.
func TestATimestamptzScansInUTCWhateverTheProcessZone(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 10, 22, 19, 0, 0, 0, time.FixedZone("ICT", 7*3600))
	scan := func(m *pgtype.Map, format int16) time.Time {
		t.Helper()
		raw, err := m.Encode(pgtype.TimestamptzOID, format, at, nil)
		if err != nil {
			t.Fatal(err)
		}
		var got time.Time
		if err := m.Scan(pgtype.TimestamptzOID, format, raw, &got); err != nil {
			t.Fatal(err)
		}
		return got
	}
	if got := scan(pgtype.NewMap(), pgtype.BinaryFormatCode); got.Location() == time.UTC {
		t.Fatal("the default codec already scans in UTC, so this test proves nothing")
	}
	utc := pgtype.NewMap()
	scanTimesInUTC(utc)
	for _, format := range []int16{pgtype.BinaryFormatCode, pgtype.TextFormatCode} {
		if got := scan(utc, format); got.Location() != time.UTC || !got.Equal(at) {
			t.Errorf("format %d scans %v in %v, want %v in UTC", format, got, got.Location(), at)
		}
	}
	raw, err := utc.Encode(pgtype.TimestamptzArrayOID, pgtype.BinaryFormatCode, []time.Time{at}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var got []time.Time
	if err := utc.Scan(pgtype.TimestamptzArrayOID, pgtype.BinaryFormatCode, raw, &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Location() != time.UTC {
		t.Errorf("an array scans %v, want one instant in UTC", got)
	}
}

// Every session reads dates in UTC, the zone the codec scans in, and a DSN
// naming another zone does not move it.
func TestEverySessionReadsDatesInUTC(t *testing.T) {
	t.Parallel()
	for _, dsn := range []string{"postgres://h/db", "postgres://h/db?timezone=Europe/Berlin", "postgres://h/db?TimeZone=Asia/Tokyo"} {
		cfg, err := PoolConfig(dsn)
		if err != nil {
			t.Fatal(err)
		}
		for name, zone := range cfg.ConnConfig.RuntimeParams {
			if strings.EqualFold(name, "timezone") && (name != "timezone" || zone != "UTC") {
				t.Errorf("%s: the session sends %s=%s, want only timezone=UTC", dsn, name, zone)
			}
		}
		if got := cfg.ConnConfig.RuntimeParams["timezone"]; got != "UTC" {
			t.Errorf("%s: the session zone is %q, want UTC", dsn, got)
		}
	}
}
