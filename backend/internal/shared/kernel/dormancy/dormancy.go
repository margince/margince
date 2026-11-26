// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package dormancy is the one ladder that says how cold an account has gone:
// active, cooling, dormant and stale, read from the company's stored
// last_activity_at. The company page, the company list filter and the agent
// tools all read this ladder. Nothing stores the rung, so it moves when time
// passes and needs no sweep.
package dormancy

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
)

// Rung is one step of the ladder.
type Rung string

// The four rungs, warmest first.
const (
	Active  Rung = "active"
	Cooling Rung = "cooling"
	Dormant Rung = "dormant"
	Stale   Rung = "stale"
)

// Rungs answers the ladder from the warmest rung to the coldest.
func Rungs() []Rung { return []Rung{Active, Cooling, Dormant, Stale} }

// Cold reports whether the account has gone quiet enough to win back: dormant
// or stale.
func (r Rung) Cold() bool { return r == Dormant || r == Stale }

// Ladder holds the silent day on which each rung after active begins.
type Ladder struct {
	Cooling, Dormant, Stale int
}

// Default is the ladder an installation gets when it chose none.
var Default = Ladder{Cooling: 30, Dormant: 60, Stale: 90}

// DefaultDays is Default in the form the installation setting stores.
func DefaultDays() []int { return Default.Days() }

// maxDay bounds each day of the ladder at ten years. A larger day is a typo,
// and the list filter subtracts it from today's date.
const maxDay = 3650

var errLadderShape = errors.New("name three whole days from 1 to 3650, each larger than the one before it, such as 30, 60 and 90")

// FromDays reads the stored setting: three positive days that ascend.
func FromDays(days []int) (Ladder, error) {
	if len(days) != 3 || days[0] < 1 || days[0] >= days[1] || days[1] >= days[2] || days[2] > maxDay {
		return Ladder{}, errLadderShape
	}
	return Ladder{Cooling: days[0], Dormant: days[1], Stale: days[2]}, nil
}

// Days is the ladder in the form the installation setting stores.
func (l Ladder) Days() []int { return []int{l.Cooling, l.Dormant, l.Stale} }

// Reading is where one account stands on the ladder, with the evidence behind it.
type Reading struct {
	Rung Rung
	// SilentDays counts calendar days in the installation zone from the last
	// activity to today. Zero when NoActivity.
	SilentDays int
	// NoActivity is an account with no activity recorded at all. It reads as
	// stale, because nobody has spoken to it, and says why.
	NoActivity bool
}

// Read places an account whose last activity was last. A nil last is an
// account with no activity at all. A date-only last activity counts its silence
// from its own date.
func (l Ladder) Read(last *occurredday.Moment, now time.Time, zone *time.Location) Reading {
	if last == nil {
		return Reading{Rung: Stale, NoActivity: true}
	}
	days := max(occurredday.DaysSince(*last, now, zone), 0)
	return Reading{Rung: l.rungAt(days), SilentDays: days}
}

func (l Ladder) rungAt(days int) Rung {
	switch {
	case days >= l.Stale:
		return Stale
	case days >= l.Dormant:
		return Dormant
	case days >= l.Cooling:
		return Cooling
	}
	return Active
}

// firstDay is the silent day on which r begins.
func (l Ladder) firstDay(r Rung) int {
	switch r {
	case Cooling:
		return l.Cooling
	case Dormant:
		return l.Dormant
	case Stale:
		return l.Stale
	}
	return 0
}

// next is the rung after r, and false for the last one.
func next(r Rung) (Rung, bool) {
	rungs := Rungs()
	for i, rung := range rungs[:len(rungs)-1] {
		if rung == r {
			return rungs[i+1], true
		}
	}
	return "", false
}

// Window is Read turned into bounds on last_activity_at, for a list filter.
// Each bound is the start of a day so many days before today, in the
// installation zone. A row is on the rung when it is at or after the From day
// and before the Before day. A nil bound is open. IncludesNone says an account
// with no activity is on the rung too.
type Window struct {
	FromDaysBack, BeforeDaysBack *int
	IncludesNone                 bool
}

// WindowOf answers the window of rung r. A last activity dated d days before
// today is silent d days. So a rung that begins on silent day n takes the day
// n days back and earlier, and stops where day n-1 back starts.
func (l Ladder) WindowOf(r Rung) Window {
	out := Window{IncludesNone: r == Stale}
	if r != Active {
		out.BeforeDaysBack = new(l.firstDay(r) - 1)
	}
	if after, ok := next(r); ok {
		out.FromDaysBack = new(l.firstDay(after) - 1)
	}
	return out
}

// SQL is the window as a predicate on column at asOf, cut in the installation
// zone. column and asOf are SQL the caller renders, never request text.
func (w Window) SQL(column, asOf string) string {
	dayStart := func(daysBack int) string {
		return occurredday.DayStartSQL(fmt.Sprintf("%s - %d", occurredday.TodaySQL(asOf), daysBack))
	}
	bounds := []string{"true"}
	if w.FromDaysBack != nil {
		bounds = append(bounds, column+" >= "+dayStart(*w.FromDaysBack))
	}
	if w.BeforeDaysBack != nil {
		bounds = append(bounds, column+" < "+dayStart(*w.BeforeDaysBack))
	}
	inside := "(" + strings.Join(bounds, " AND ") + ")"
	if w.IncludesNone {
		return "(" + column + " IS NULL OR " + inside + ")"
	}
	return "(" + column + " IS NOT NULL AND " + inside + ")"
}
