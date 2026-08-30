// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

// Patch is the ONE spelling of the partial-UPDATE write shape, so a column it
// can render twice is a 42601 waiting in every store that sets one from two
// independent branches. These pin the property at the type rather than at the
// call sites: an assignment list is a set of columns, and the SQL it renders is
// the proof.

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

// Two branches of one request can legitimately want the same column — an
// `updated_at` bump next to whichever field the caller actually sent. The second
// write must land on the first's assignment, not beside it: Postgres rejects
// `SET updated_at = $1, updated_at = $2` with 42601, so an append would turn a
// legal request into a 500.
func TestSetTwiceOnOneColumnRendersOneAssignment(t *testing.T) {
	p := NewPatch()
	p.Set("display_name", "old name", "new name")
	p.Set("updated_at", "t0", "t1")
	p.Set("updated_at", "t0", "t2")

	sql := strings.Join(p.sets, ", ")
	if got := strings.Count(sql, "updated_at ="); got != 1 {
		t.Fatalf("updated_at assigned %d times in %q, want exactly 1 — Postgres rejects a duplicate column", got, sql)
	}
	// One bind value per assignment is what makes the slot index name the
	// placeholder it renders; the apply paths clone rather than append to keep it.
	if len(p.sets) != len(p.args) {
		t.Fatalf("%d assignments against %d bind values: the placeholders cannot all resolve", len(p.sets), len(p.args))
	}
}

// Last write wins on the value, and the placeholder still points at it. A repeat
// that updated the SQL but not the argument (or vice versa) would bind the wrong
// value into the right column, which is worse than the error it replaced.
func TestTheLastSetSuppliesTheBoundValue(t *testing.T) {
	p := NewPatch()
	p.Set("lifecycle", "prospect", "customer")
	p.Set("updated_at", "t0", "t1")
	p.Set("updated_at", "t0", "t2")

	if want := []any{"customer", "t2"}; !reflect.DeepEqual(p.args, want) {
		t.Errorf("args = %v, want %v", p.args, want)
	}
	// The exact fragments, not a suffix match: "$1" is a suffix of "$21" too, and
	// a repeat that rewrote the wrong slot is precisely a placeholder mix-up.
	if want := []string{"lifecycle = $1", "updated_at = $2"}; !reflect.DeepEqual(p.sets, want) {
		t.Errorf("sets = %v, want %v", p.sets, want)
	}
}

// The audit diff describes the whole change, so before stays the value the row
// actually held when the transaction read it. A second Set's oldVal is either the
// same or a re-read of a value this transaction already changed; letting it
// overwrite would make the diff claim the column started where the first write
// left it.
func TestARepeatKeepsTheOriginalBeforeImage(t *testing.T) {
	p := NewPatch()
	p.Set("updated_at", "the row's real prior value", "t1")
	p.Set("updated_at", "t1", "t2")

	if got := p.Before()["updated_at"]; got != "the row's real prior value" {
		t.Errorf("before image = %v, want the value read from the row", got)
	}
	if got := p.After()["updated_at"]; got != "t2" {
		t.Errorf("after image = %v, want the last value written", got)
	}
}

// A cf_ column comes through setQuoted, whose SET fragment is quoted while its
// audit key stays the wire name. The de-duplication has to key on that same wire
// name, or the two paths could each render an assignment for one column.
func TestSetQuotedDeduplicatesOnTheWireName(t *testing.T) {
	p := NewPatch()
	p.setQuoted("cf_employee_count", 10, 20)
	p.setQuoted("cf_employee_count", 10, 30)

	// The rendering matters as much as the count: the overwrite branch re-renders
	// the left-hand side, and a slip that passed the bare column there would
	// splice a catalog-derived identifier into the UPDATE unquoted — the one thing
	// setQuoted exists to prevent.
	if want := []string{`"cf_employee_count" = $1`}; !reflect.DeepEqual(p.sets, want) {
		t.Fatalf("sets = %v, want %v", p.sets, want)
	}
	if p.args[0] != 30 {
		t.Errorf("args[0] = %v, want 30 (the last write)", p.args[0])
	}
	if got := p.After()["cf_employee_count"]; got != 30 {
		t.Errorf("after image keys on the bare wire name: got %v for cf_employee_count", got)
	}
}

// Two writers that disagree about the identifier are not the same assignment.
// A core column and a catalog column sharing a name is a programming error, and
// merging them would either drop a validated value or decide the quoting — so
// they keep separate slots and the statement still fails loudly, as it did before
// the merge existed. Unreachable today (every catalog column is cf_-prefixed),
// which is exactly why it needs pinning rather than trusting.
func TestADisagreementAboutTheIdentifierIsNotMerged(t *testing.T) {
	p := NewPatch()
	p.Set("lifecycle", "prospect", "customer")
	p.setQuoted("lifecycle", "prospect", "from the catalog")

	if len(p.sets) != 2 {
		t.Fatalf("sets = %v, want the bare and quoted spellings kept apart", p.sets)
	}
	if p.args[0] != "customer" {
		t.Errorf("the core value was overwritten by the catalog one: args[0] = %v", p.args[0])
	}
}

// The archive predicate is rendered once and shared by a lock and the update it
// authorizes, so the two can never resolve different row sets. Which filter
// yields which predicate is the whole contract, and it is invisible in SQL that
// runs fine either way — a missing liveness clause does not fail, it just
// silently widens what a write can reach.
func TestLiveClauseGuardsOnlyTheTablesThatCanArchive(t *testing.T) {
	const predicate = " AND archived_at IS NULL"
	for _, tc := range []struct {
		name    string
		filter  ArchivedFilter
		want    string
		because string
	}{
		{
			"LiveOnly", LiveOnly, predicate,
			"the default write posture must not reach a retired record",
		},
		{
			"IncludeArchived", IncludeArchived, "",
			"a flow that deliberately touches an archived row asks for it explicitly",
		},
		{
			"NoArchiveColumn", NoArchiveColumn, "",
			"a table with no archived_at cannot be filtered on one — the predicate is a SQL error, not a narrower write",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := liveClause(tc.filter); got != tc.want {
				t.Errorf("liveClause(%s) = %q, want %q — %s", tc.name, got, tc.want, tc.because)
			}
		})
	}
}

// A lock and the update it authorizes must agree, so the lock carries the
// filter it was taken under rather than letting ApplyLocked assume one.
func TestARowLockRemembersTheFilterItWasTakenUnder(t *testing.T) {
	for _, filter := range []ArchivedFilter{LiveOnly, IncludeArchived, NoArchiveColumn} {
		lock := RowLock{archived: filter}
		if got := liveClause(lock.archived); got != liveClause(filter) {
			t.Errorf("a lock taken under %v applies through %q, want %q", filter, got, liveClause(filter))
		}
	}
}

// TestMovedIsWhatActuallyChanged is the difference between what a write TOUCHED
// and what it CHANGED.
//
// After holds every column the caller Set, because Set records an assignment
// and does not compare — right for the audit diff, wrong for a reader asking
// whether a field moved. Several were asking exactly that by testing a key's
// presence, and the cost was a provenance stamp: a display name re-sent
// unchanged promoted name_source to 'human', after which no automated source
// may correct it.
func TestMovedIsWhatActuallyChanged(t *testing.T) {
	same := "Acme GmbH"
	renamed := "Acme AG"
	p := NewPatch()
	p.Set("display_name", same, same)
	p.Set("legal_name", same, renamed)

	after := p.After()
	if len(after) != 2 {
		t.Fatalf("After() = %v, want both columns — it reports what the write touched", after)
	}
	moved := p.Moved()
	if _, unchanged := moved["display_name"]; unchanged {
		t.Error("a column re-set to its own value is reported as moved; re-sending a name is " +
			"not re-authoring it")
	}
	if got, ok := moved["legal_name"]; !ok || got != renamed {
		t.Errorf("moved[legal_name] = %v (present %t), want the new value", got, ok)
	}
}

// A before and after of different TYPES counts as moved, which is the safe
// direction: it is what the presence test already did, so a caller switching to
// Moved cannot start missing a change it used to see.
func TestAChangeOfShapeCountsAsMoved(t *testing.T) {
	value := "Acme GmbH"
	p := NewPatch()
	p.Set("display_name", nil, &value)
	if _, moved := p.Moved()["display_name"]; !moved {
		t.Error("setting a value where there was none is not reported as moved")
	}
}

// A `date` column's images are recorded the way Postgres renders one.
//
// `to_jsonb(row)` gives "2026-12-01" for a date column, while a Go time.Time
// marshals as "2026-12-01T00:00:00Z". The undo path decides whether a field has
// moved by comparing the audit image against the live row as JSON, so an image
// in the second spelling reads as moved the instant it is written — and undo
// refuses a change nobody has touched.
func TestSetDateRecordsBothImagesTheWayPostgresRendersADate(t *testing.T) {
	was := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
	now := time.Date(2027, 3, 15, 0, 0, 0, 0, time.UTC)

	p := NewPatch()
	p.SetDate("expected_close_date", &was, &now)
	if got := p.Before()["expected_close_date"]; got != "2026-12-01" {
		t.Errorf("before = %#v, want the day as Postgres renders it", got)
	}
	if got := p.After()["expected_close_date"]; got != "2027-03-15" {
		t.Errorf("after = %#v, want the day as Postgres renders it", got)
	}
	// And what is BOUND is that same text, so the value written and the value
	// recorded cannot disagree.
	if got := p.args[0]; got != "2027-03-15" {
		t.Errorf("the bind value is %#v, want the same text the image carries", got)
	}
}

// Absence stays absence. A cleared date must be a JSON null on both sides —
// rendered as a zero day it would read as a change to the first of January.
func TestSetDateKeepsAnAbsentDateAbsent(t *testing.T) {
	held := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
	p := NewPatch()
	p.SetDate("wait_until", &held, nil)
	if got := p.After()["wait_until"]; got != nil {
		t.Errorf("clearing a date recorded %#v, want a null", got)
	}
	p2 := NewPatch()
	p2.SetDate("wait_until", nil, &held)
	if got := p2.Before()["wait_until"]; got != nil {
		t.Errorf("a date that was absent recorded %#v as its before, want a null", got)
	}
}
