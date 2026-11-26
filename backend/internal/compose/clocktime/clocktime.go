// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package clocktime is the River schedule of a job that runs at a local clock
// time, such as "weekdays at 07:30". The schedule book in compose places it
// and moves it when the installation's time zone changes.
package clocktime

import (
	"time"

	"github.com/margince/margince/backend/pkg/extension"
)

// Schedule is a River schedule for one extension.TimeOfDay.
type Schedule struct {
	At extension.TimeOfDay
	// Zone is read on every call, so a changed zone counts from the next one.
	Zone func() *time.Location
	Now  func() time.Time
}

// Next is the first slot after both current and now. After downtime River
// restores a stored slot and asks Next with that old slot. Counting from now
// keeps it from replaying every slot it missed.
func (s Schedule) Next(current time.Time) time.Time {
	now := s.Now()
	if now.After(current) {
		current = now
	}
	return s.SlotAfter(current)
}

// SlotAfter is the first slot after t. A run at a slot asks again with that
// slot, so a time the clock shows twice when it falls back runs once.
func (s Schedule) SlotAfter(t time.Time) time.Time {
	zone := s.Zone()
	y, m, d := t.In(zone).Date()
	// Today and the next seven days hold a slot of every valid TimeOfDay.
	for offset := range 8 {
		if !s.At.RunsOn(time.Date(y, m, d+offset, 12, 0, 0, 0, zone).Weekday()) {
			continue
		}
		if slot := clockTimeOn(y, m, d+offset, s.At.Minute, zone); slot.After(t) {
			return slot
		}
	}
	return t.AddDate(0, 0, 7)
}

// clockTimeOn is the moment the local clock shows minute on that day. On the
// day the clock springs past it, the slot moves to the end of the gap, the
// first minute after it.
func clockTimeOn(y int, m time.Month, d, minute int, zone *time.Location) time.Time {
	slot := time.Date(y, m, d, minute/60, minute%60, 0, 0, zone)
	if slot.Hour()*60+slot.Minute() == minute {
		return slot
	}
	start, end := slot.ZoneBounds()
	if end.Sub(slot) < slot.Sub(start) {
		return end
	}
	return start
}
