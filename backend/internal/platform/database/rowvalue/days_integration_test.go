// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package rowvalue

import (
	"context"
	"testing"
	"time"
)

// A date column and a timestamp at the same midnight UTC: only the date is a
// calendar day, so only it leaves as YYYY-MM-DD.
func TestADayReaderSpellsADateAsItsDayAndLeavesAnInstantAlone(t *testing.T) {
	ctx := context.Background()
	pool := censusPool(ctx, t)
	rows, err := pool.Query(ctx,
		`SELECT '2025-12-01'::date, '2026-03-01 00:00:00+00'::timestamptz, NULL::date`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if !rows.Next() {
		t.Fatalf("the query returned no row: %v", rows.Err())
	}
	values, err := NewDayReader().Values(rows)
	if err != nil {
		t.Fatal(err)
	}
	if values[0] != "2025-12-01" {
		t.Errorf("a date read as %#v, want the day 2025-12-01", values[0])
	}
	instant, ok := values[1].(time.Time)
	if !ok || !instant.Equal(time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("a timestamp read as %#v, want the instant it holds", values[1])
	}
	if values[2] != nil {
		t.Errorf("a NULL date read as %#v, want nil", values[2])
	}
}
