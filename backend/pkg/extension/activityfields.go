// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//margince:extension-surface

package extension

import "time"

// ActivityKindMessage is the timeline kind a message on a messaging channel
// lands as. It is published because a unit cannot reach the core's own
// vocabulary and would otherwise spell the string blind.
//
// A message is the one kind whose transport is separable from it: it names
// what carried it on ActivityFields.ChannelProvider, and no other kind does.
// A fitness test outside this package holds this constant equal to the core's.
const ActivityKindMessage = "message"

// ActivityKindTask is the one kind that may carry an assignee and a due date.
const ActivityKindTask = "task"

// ActivityFields preserves the provider's timeline content and privacy hold.
type ActivityFields struct {
	ProviderPrivate bool
	Kind            string
	ChannelProvider string
	Subject         string
	Body            string
	OccurredAt      time.Time
	DateOnly        bool
	DurationSeconds *int
	Direction       string
	// AssigneeID and DueAt are meaningful for one kind only: ActivityKindTask.
	// The assignee is any live colleague the unit names. The core refuses an
	// agent seat or a retired one. A task with no due date is on nobody's
	// Worklist, so a unit that wants the assignee to see it sets DueAt too.
	AssigneeID UserID
	DueAt      time.Time
}
