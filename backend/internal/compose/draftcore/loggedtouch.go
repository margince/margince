// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package draftcore

import (
	"time"

	"github.com/margince/margince/backend/internal/compose/activitywhen"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/convstate"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
)

// A logged note or meeting is prior contact even when neither side sent mail.
// It counts as our side: nothing claims the contact wrote.
func ClassifyWithLogged(now time.Time, zone *time.Location, lastIn, lastOut time.Time, activities []crmcontracts.Activity) convstate.State {
	if logged := lastLoggedTouch(activities, now, zone); logged.After(lastOut) {
		lastOut = logged
	}
	return convstate.Classify(now, lastIn, lastOut, zone)
}

// Date-only anchors can sort differently once read in the record calendar.
func lastLoggedTouch(activities []crmcontracts.Activity, now time.Time, zone *time.Location) time.Time {
	var latest time.Time
	for _, activity := range activities {
		if !IsLoggedTouch(activity, now, zone) {
			continue
		}
		moment := activitywhen.MomentOf(activity)
		at := moment.At
		if moment.DateOnly {
			at = occurredday.Begins(at, zone)
		}
		if at.After(latest) {
			latest = at
		}
	}
	return latest
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
	return status == nil || relstrength.MeetingStatusCounts(string(*status))
}
