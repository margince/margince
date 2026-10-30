// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The activity a unit hands the core, and the kinds it may name.
//
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

// ActivityFields is a captured interaction bound for the timeline. It mirrors
// the core's own shape, and a fitness test outside this package compares the
// two so a field added there cannot go unnoticed here.
type ActivityFields struct {
	// Kind is the timeline kind. The core admits a closed set and refuses a
	// value outside it rather than coercing it.
	Kind string
	// ChannelProvider is the transport that carried this record. It is
	// meaningful for one kind only: ActivityKindMessage.
	//
	// The core enforces the pairing at the write door. A message names a
	// transport, nothing else does, and a unit may name only one it declared.
	// This package cannot see the core's kinds or the unit's channels. A rule
	// spelled here would be a second version that can disagree with the first.
	ChannelProvider string
	// Subject and Body are the message as a human reads it. Both are bounded
	// (see the Max* constants), because an unbounded field lets a remote
	// provider choose how much this installation stores.
	Subject string
	Body    string
	// OccurredAt is when the message happened at the provider, not when the
	// poll saw it. Ordering by discovery orders the timeline by our scheduling.
	OccurredAt      time.Time
	DateOnly        bool // OccurredAt names a date with no time of day, as an all-day event does.
	DurationSeconds *int
	Direction       string
	// AssigneeID and DueAt are meaningful for one kind only: ActivityKindTask.
	// The assignee is any live colleague the unit names. The core refuses an
	// agent seat or a retired one. A task with no due date is on nobody's
	// Worklist, so a unit that wants the assignee to see it sets DueAt too.
	AssigneeID UserID
	DueAt      time.Time
}
