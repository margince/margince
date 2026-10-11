// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// What this installation can say about the floor it applies.
//
// An operator needs to tell four cases apart. No jurisdiction is selected, and
// the strictest compiled-in floor holds. One is selected whose pack declares no
// floor. One is selected with a floor. And beside any of these, a carried floor
// from an earlier jurisdiction may still bind older mail.

import (
	"time"

	"github.com/margince/margince/backend/internal/shared/ports/jurisdiction"
)

// FloorPosture is the evaluated floor, in the terms a screen or a log states.
type FloorPosture struct {
	// Selected is the installation's jurisdiction, empty when none is set.
	Selected string
	// Known reports that a correspondence floor binds new mail. With nothing
	// selected that is the strictest compiled-in pack's; with a selection it is
	// that pack's own.
	Known bool
	// Class and Keep describe that floor, and are empty when Known is false. A
	// reader is never shown a period beside a false Known.
	Class string
	Keep  jurisdiction.Period
	// FromYearEnd says the period counts from the end of the calendar year.
	FromYearEnd bool
	// Packs names every compiled-in jurisdiction, floor or no floor.
	Packs []string
	// Carried is the floor an earlier jurisdiction left on older mail, or nil.
	Carried *CarriedPosture
}

// CarriedPosture describes a carried floor and how long it keeps binding.
type CarriedPosture struct {
	Class       string
	Keep        jurisdiction.Period
	FromYearEnd bool
	// Before is the change time: mail that occurred earlier is bound.
	Before time.Time
	// BindsUntil is when the youngest bound message leaves the carried window,
	// after which the carried floor binds nothing.
	BindsUntil time.Time
}

// StatutoryFloorPosture reports the floor a resolved policy applies.
func StatutoryFloorPosture(p FloorPolicy) FloorPosture {
	posture := FloorPosture{Selected: string(p.Selected)}
	for _, pack := range jurisdiction.Applicable() {
		posture.Packs = append(posture.Packs, string(pack.Code()))
	}
	if p.Current.Name != "" {
		posture.Known = true
		posture.Class = string(p.Current.Name)
		posture.Keep = p.Current.Keep
		posture.FromYearEnd = p.Current.Anchor == jurisdiction.AnchorCalendarYearEnd
	}
	if c := p.Carried; c != nil {
		posture.Carried = &CarriedPosture{
			Class: string(c.Class.Name), Keep: c.Class.Keep,
			FromYearEnd: c.Class.Anchor == jurisdiction.AnchorCalendarYearEnd,
			Before:      c.Before, BindsUntil: windowEndAt(c.Class, c.Before),
		}
	}
	return posture
}

// windowEndAt is when the window over a message that occurred at occurred
// closes, the same instant floorWindowEnd computes in SQL.
func windowEndAt(class jurisdiction.RetentionClass, occurred time.Time) time.Time {
	start := occurred
	if class.Anchor == jurisdiction.AnchorCalendarYearEnd {
		start = time.Date(occurred.Year()+1, time.January, 1, 0, 0, 0, 0, occurred.Location())
	}
	return start.AddDate(class.Keep.Years, class.Keep.Months, class.Keep.Days)
}
