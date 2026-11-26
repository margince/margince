// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package connector

import (
	"context"
	"time"
)

// MeetingReplayer updates an existing calendar meeting through the capture
// writer while leaving an uncaptured event outside the capture window.
type MeetingReplayer interface {
	ReplayMeeting(ctx context.Context, rec NormalizedRecord) error
}

// CalendarAnswer is the authenticated calendar account's own invitation answer.
type CalendarAnswer struct {
	Address  string
	Response string
}

// MeetingRemover handles a removed calendar copy without assuming that the
// organizer cancelled the shared meeting.
type MeetingRemover interface {
	RemoveCalendarMeeting(ctx context.Context, key NaturalKey, identity CrossDoorIdentity, at time.Time) error
}

// UnlistedMeetingRetirer retires the stored meetings of the acting seat's
// calendar that a complete listing no longer holds. listed is every event id
// the listing returned, and only meetings starting after from and up to until
// are judged. A connector calls it only after the whole listing was settled.
type UnlistedMeetingRetirer interface {
	RetireUnlistedMeetings(ctx context.Context, sourceSystem string, listed []string, from, until time.Time) error
}
