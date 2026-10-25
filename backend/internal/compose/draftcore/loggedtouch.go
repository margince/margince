// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package draftcore

import (
	"time"

	"github.com/margince/margince/backend/internal/compose/activitywhen"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/convstate"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
)

// ClassifyWithLogged is convstate.Classify with a logged note or meeting
// counted as prior contact: a rep who met somebody and wrote it down is not
// writing to a stranger. The touch counts as our own side's, because nobody
// on their side wrote anything.
//
// Here rather than in the 360 reads' last-touch stamps, which other readers
// take to mean mail only. The contact, lead and account drafts all classify
// through it, because the shared TIME rule tells each of them what
// silence_days counts from.
func ClassifyWithLogged(now time.Time, zone *time.Location, lastIn, lastOut time.Time, activities []crmcontracts.Activity) convstate.State {
	if logged := lastLoggedTouch(activities, now, zone); logged.After(lastOut) {
		lastOut = logged
	}
	return convstate.Classify(now, lastIn, lastOut)
}

// lastLoggedTouch is the newest logged touch in newest-first activities, zero
// when there is none.
func lastLoggedTouch(activities []crmcontracts.Activity, now time.Time, zone *time.Location) time.Time {
	for _, activity := range activities {
		if IsLoggedTouch(activity, now, zone) {
			return activity.OccurredAt.UTC()
		}
	}
	return time.Time{}
}

// IsLoggedKind reports the kinds a rep writes down about contact outside the
// mailbox.
func IsLoggedKind(kind crmcontracts.ActivityKind) bool {
	return kind == crmcontracts.ActivityKindNote || kind == crmcontracts.ActivityKindMeeting
}

// IsLoggedTouch reports a note or meeting that has happened by now in zone
// (occurredday.Happened). A canceled or missed meeting records no contact. A
// past meeting still marked booked counts: it most often took place, and reps
// rarely come back to mark it held.
func IsLoggedTouch(activity crmcontracts.Activity, now time.Time, zone *time.Location) bool {
	if !IsLoggedKind(activity.Kind) || !occurredday.Happened(string(activity.Kind), activitywhen.MomentOf(activity), now, zone) {
		return false
	}
	status := activity.MeetingStatus
	return status == nil || (*status != crmcontracts.ActivityMeetingStatusCanceled &&
		*status != crmcontracts.ActivityMeetingStatusNoShow)
}
