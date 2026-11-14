// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package events

// activityNarrowingEvents are the events after which fewer members may read an
// activity than before: archived, restricted, or its audience changed.
// Each names the activity it changed. A consumer that keeps a copy of activity
// text rechecks the activity on each of them.
var activityNarrowingEvents = map[string]bool{
	"activity.updated":     true,
	"activity.archived":    true,
	"retention.restricted": true,
}

// NarrowsActivity reports whether an event may have narrowed who can read the
// activity it names.
func NarrowsActivity(env Envelope) bool {
	return activityNarrowingEvents[env.Type] && env.Entity.Type == "activity" && !env.Entity.ID.IsZero()
}
