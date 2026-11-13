// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package draftcore

import (
	"context"
	"slices"
	"time"

	"github.com/margince/margince/backend/internal/compose/activitywhen"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/convstate"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
)

// ClassifyWithLogged counts notes and past meetings as prior contact, even without mail.
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
// meeting is contact once it is past and nobody called it off
// (relstrength.HadContact); nobody has to report it held.
func IsLoggedTouch(activity crmcontracts.Activity, now time.Time, zone *time.Location) bool {
	at := activitywhen.MomentOf(activity)
	if !IsLoggedKind(activity.Kind) || !occurredday.Happened(string(activity.Kind), at, now, zone) {
		return false
	}
	var status string
	if activity.MeetingStatus != nil {
		status = string(*activity.MeetingStatus)
	}
	if activity.Kind == crmcontracts.ActivityKindMeeting {
		return relstrength.HadContact(string(activity.Kind), status, "", at, now, zone)
	}
	return relstrength.MeetingStatusNotCalledOff(status)
}

// LoggedOn names the record whose newest logged touch a draft reads: a
// contact, company or lead, narrowed to one project when the draft is.
type LoggedOn struct {
	RecordType string
	ID         ids.UUID
	ProjectID  *ids.ProjectID
}

// LoggedTouchReader reads the newest note or past meeting on one record that
// the caller may read. The timeline page a draft folds is bounded, and a note
// older than a page of newer activity is still contact.
type LoggedTouchReader interface {
	NewestLoggedTouch(ctx context.Context, on LoggedOn, now time.Time) ([]crmcontracts.Activity, error)
}

// WithNewestLogged is rows with the newest logged touch on the record added,
// for ClassifyWithLogged. A nil reader adds nothing.
func WithNewestLogged(ctx context.Context, reader LoggedTouchReader, on LoggedOn, now time.Time, rows []crmcontracts.Activity) ([]crmcontracts.Activity, error) {
	if reader == nil {
		return rows, nil
	}
	newest, err := reader.NewestLoggedTouch(ctx, on, now)
	if err != nil {
		return nil, err
	}
	return append(slices.Clone(rows), newest...), nil
}
