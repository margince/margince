// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package draftcore

import (
	"time"

	"github.com/margince/margince/backend/internal/compose/activitywhen"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/convstate"
	"github.com/margince/margince/backend/internal/shared/kernel/meetingoutcome"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
)

// ClassifyWithLogged counts notes and held meetings as prior contact, even without mail.
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
// (occurredday.Happened), the Go form of relstrength.HappenedByNowSQL. A
// meeting is contact only when someone recorded it held. A past meeting still
// booked or with no status is awaiting its outcome.
func IsLoggedTouch(activity crmcontracts.Activity, now time.Time, zone *time.Location) bool {
	if !IsLoggedKind(activity.Kind) || !occurredday.Happened(string(activity.Kind), activitywhen.MomentOf(activity), now, zone) {
		return false
	}
	var status string
	if activity.MeetingStatus != nil {
		status = string(*activity.MeetingStatus)
	}
	if activity.Kind == crmcontracts.ActivityKindMeeting {
		return meetingoutcome.IsHeld(status)
	}
	return relstrength.MeetingStatusNotCalledOff(status)
}
