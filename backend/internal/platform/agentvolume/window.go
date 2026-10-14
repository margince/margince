// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agentvolume

import "time"

// PolicyRefresh is the longest a process may judge calls against allowances it
// read earlier. A saved window change waits at least this long before it takes
// over, so every process holds the new plan by the moment it switches.
const PolicyRefresh = time.Minute

// WindowPlan is the window length in force and, when an admin saved another,
// the length that takes over at NextFrom.
//
// A length never changes mid-window. The bucket is the moment divided by the
// length. A new length now would move every counter to a key that reads zero,
// which hands every passport a full allowance. NextFrom is always a reset of the
// length in force, where every counter starts from zero anyway.
type WindowPlan struct {
	Length   time.Duration
	Next     time.Duration
	NextFrom time.Time
}

// At answers the length in force at now.
func (p WindowPlan) At(now time.Time) time.Duration {
	if p.Next > 0 && !now.Before(p.NextFrom) {
		return p.Next
	}
	return p.Length
}

// Pending reports whether a saved length is still waiting at now.
func (p WindowPlan) Pending(now time.Time) bool {
	return p.Next > 0 && now.Before(p.NextFrom)
}

// Reschedule is the plan after an admin asks for requested at now. Asking for
// the length in force cancels a change that is still waiting.
//
// The first window under the new length ends at that length's own boundary, so
// it can be shorter than the length and never longer.
func (p WindowPlan) Reschedule(now time.Time, requested time.Duration) WindowPlan {
	current := p.At(now)
	if requested == current {
		return WindowPlan{Length: current}
	}
	return WindowPlan{Length: current, Next: requested, NextFrom: ResetAfter(now.Add(PolicyRefresh), current)}
}

// ResetAfter is the first reset of a window of length that comes after now.
// Every window is a whole number of lengths since the Unix epoch, so every
// passport resets at the same moment.
//
// It divides in nanoseconds, as bucketAt does: a sub-second length would
// truncate to zero seconds and the division would panic.
func ResetAfter(now time.Time, length time.Duration) time.Time {
	next := (bucketAt(now, length) + 1) * int64(length)
	return time.Unix(0, next).UTC()
}

// bucketAt names the fixed window now falls in for a window of length.
func bucketAt(now time.Time, length time.Duration) int64 {
	return now.UTC().UnixNano() / int64(length)
}
