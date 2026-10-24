// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// Filing a captured meeting under the contacts who were in it.
//
// Mail reaches its records through the counterparty: one address, one ensure,
// one link. A meeting has no counterparty — attendance is a LIST, and the
// calendar mapper says so by leaving Counterparty unset. That is honest about
// the shape and it had a consequence nobody had noticed: with no counterparty
// the tiered gate returns "captured, named nobody", the ensure never runs, and
// a synced meeting was written with participant rows and NO activity_link at
// all. Every calendar meeting in the dev workspace — 517 of them — was
// unreachable from the company and contact pages that find activity through
// links, so "Next meeting" was empty on an account you were seeing that day.
//
// The contacts are already known here: StampFurtherParticipants has just resolved
// each attendee address against contact_email in this same transaction. So the
// links are derived from those rows rather than re-resolved, and a meeting whose
// attendee is a contact is filed under that contact.
//
// Contact and lead links only. A lead attendee is matched by exact address.
// A meeting may never link straight to a company
// (activities.refuseACompanyMeeting, and a trigger behind it). The company
// reach walk finds the account through the attendee's employment, so the
// contact link puts the meeting on the company page too.
//
// Nothing is CREATED from an invite. An attendee with no contact record stays an
// address here; an invitation is not correspondence, and one all-hands with
// forty unknown externals would otherwise buy forty creation decisions nobody
// asked for. Those meetings are picked up later by the cohort repair, in two
// steps: it promotes the attendee's address-only participant row to name the
// new contact, then files the meeting under them (contacts.linkAttendedMeetings).
// The second step is a MEETING-shaped arm on purpose — the repair's other arm
// finds work by counterparty_email, and a meeting carries none, so for a while
// this comment described a pickup that could not happen and every synced
// meeting stayed unlinked.

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// meetingKind is the activity kind a calendar connector writes. Spelled here,
// beside the only capture path that treats meetings differently from mail.
const meetingKind = "meeting"

// maxDerivedMeetingLinks bounds how many records one captured activity is filed
// under through its participants.
//
// It mirrors activities.maxActivityLinks, because a module never imports a
// sibling. An activity is filed under at most 25 records on every transport.
//
// Held by: TestTheLinkCeilingHasOneValue (backend/gates/linkceilingparity_test.go)
const maxDerivedMeetingLinks = 25

// linkResolvedMeetingParticipants files a captured meeting under each attendee
// who is a live contact or a live lead (participantfiling.go), organizer first.
//
// It answers the links written, which the audience limiter reads. A meeting
// with links is not held to its participants. A claimed link that was never
// written would open a meeting that nothing files.
func (s *Sink) linkResolvedMeetingParticipants(
	ctx context.Context, tx pgx.Tx, activityID ids.ActivityID, existing int,
) (int, error) {
	// The ceiling is on the activity, so a connector's own links count.
	return fileUnderParticipants(tx, activityID, participantFiling{
		decider: ctx,
		budget:  maxDerivedMeetingLinks - existing,
		write:   insertParticipantLink(ctx, tx, activityID),
	})
}
