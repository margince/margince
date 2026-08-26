// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package idlebase_test

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/idlebase"
)

func TestSinceReadsTheActivityWhenThereIsOne(t *testing.T) {
	created := time.Date(2026, 1, 5, 9, 0, 0, 0, time.UTC)
	touched := time.Date(2026, 3, 2, 14, 30, 0, 0, time.UTC)
	if got := idlebase.Since(created, &touched); !got.Equal(touched) {
		t.Errorf("Since(created, touched) = %v, want the activity %v", got, touched)
	}
}

// The case the fallback exists for. A record nobody has ever touched is not
// "no data" — it has been silent since the day somebody wrote it down, and
// reading it as unknown hides the oldest untouched records from every surface
// built on this.
func TestSinceFallsBackToTheCreationOfAnUntouchedRecord(t *testing.T) {
	created := time.Date(2026, 1, 5, 9, 0, 0, 0, time.UTC)
	if got := idlebase.Since(created, nil); !got.Equal(created) {
		t.Errorf("Since(created, nil) = %v, want the creation %v", got, created)
	}
}

// The column order is load-bearing and invisible: created_at is NOT NULL, so a
// coalesce written the other way round never falls back and quietly answers
// "created" for every record. The assertion is on the rendered text because
// that reversal is what a reader cannot see.
func TestSQLPrefersTheActivityOverTheCreation(t *testing.T) {
	if got := idlebase.SQL(""); got != "coalesce(last_activity_at, created_at)" {
		t.Errorf("SQL(%q) = %q, want the activity first", "", got)
	}
}

func TestSQLQualifiesEveryColumnWithTheAlias(t *testing.T) {
	// Both columns, not just the first: a query that joins another table
	// carrying created_at needs the qualifier on the fallback too, or the
	// reference is ambiguous SQL rather than merely pointing at the wrong row.
	if got := idlebase.SQL("d"); got != "coalesce(d.last_activity_at, d.created_at)" {
		t.Errorf("SQL(%q) = %q, want both columns qualified", "d", got)
	}
}
