// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package draftcore

import (
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/convstate"
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
func ClassifyWithLogged(now, lastIn, lastOut time.Time, activities []crmcontracts.Activity) convstate.State {
	if logged := lastLoggedTouch(activities, now); logged.After(lastOut) {
		lastOut = logged
	}
	return convstate.Classify(now, lastIn, lastOut)
}

// lastLoggedTouch is the newest logged touch in newest-first activities, zero
// when there is none.
func lastLoggedTouch(activities []crmcontracts.Activity, now time.Time) time.Time {
	for _, activity := range activities {
		if IsLoggedTouch(activity, now) {
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

// IsLoggedTouch reports a note or meeting that records contact which really
// happened: dated at or before now, and not a meeting that was canceled or
// that the other side missed. A past meeting still marked booked counts: it
// most often took place, and reps rarely come back to mark it held.
func IsLoggedTouch(activity crmcontracts.Activity, now time.Time) bool {
	if !IsLoggedKind(activity.Kind) || activity.OccurredAt.After(now) {
		return false
	}
	status := activity.MeetingStatus
	return status == nil || (*status != crmcontracts.ActivityMeetingStatusCanceled &&
		*status != crmcontracts.ActivityMeetingStatusNoShow)
}
