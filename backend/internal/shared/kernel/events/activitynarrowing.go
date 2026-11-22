// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package events

// NarrowsActivity reports whether an event may have narrowed who can read the
// activity it names. Each ActivityNarrowingTypes event names the activity it
// changed, and a consumer that keeps a copy of activity text rechecks it.
func NarrowsActivity(env Envelope) bool {
	return ActivityNarrowingTypes.Has(env.Type) && env.Entity.Type == "activity" && !env.Entity.ID.IsZero()
}
