// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"strings"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func applyPickedTime(field string, wire *crmcontracts.IntentTime, target **time.Time) error {
	if wire == nil {
		return nil
	}
	instant, err := storekit.ResolveIntentTime(field, *wire)
	if err != nil {
		return err
	}
	*target = &instant
	return nil
}

func (in *LogActivityInput) admitPickedTimes() error {
	if err := checkMeetingIntent(in.Kind, in.OccurredPrecision, in.MeetingIntent); err != nil {
		return err
	}
	if err := applyPickedTime("meeting_intent", in.MeetingIntent, &in.OccurredAt); err != nil {
		return err
	}
	if err := applyPickedTime("due_intent", in.DueIntent, &in.DueAt); err != nil {
		return err
	}
	in.DueHasTime = in.DueHasTime || dueIntentHasTime(in.DueIntent)
	return applyPickedTime("remind_intent", in.RemindIntent, &in.RemindAt)
}

// dueIntentHasTime says whether a chosen due clock names a time of day. A
// date-only deadline is sent as its day's last second.
func dueIntentHasTime(intent *crmcontracts.IntentTime) bool {
	if intent == nil {
		return false
	}
	_, clock, _ := strings.Cut(intent.WallClock, "T")
	return strings.TrimRight(strings.TrimPrefix(clock, "23:59:59"), ".0") != ""
}

// admitPatchDueTime pairs a chosen due clock with due_has_time: the clock
// implies the flag, and a flag that says "no time" beside a time is refused.
func (in *UpdateActivityInput) admitPatchDueTime() error {
	if in.DueIntent == nil {
		return nil
	}
	timed := dueIntentHasTime(in.DueIntent)
	if in.DueHasTime != nil && !*in.DueHasTime && timed {
		return &values.ParseError{Field: fieldDueHasTime, Code: "invalid", Message: "a due_intent with a time of day is a timed deadline"}
	}
	if in.DueHasTime == nil {
		in.DueHasTime = &timed
	}
	return nil
}

// checkMeetingIntent admits a chosen local start only on a meeting that is
// held to an instant: a whole-day meeting has no clock to keep.
// fieldMeetingIntent names a meeting's chosen local start in refusals and audits.
const fieldMeetingIntent = "meeting_intent"

func checkMeetingIntent(kind, precision string, intent *crmcontracts.IntentTime) error {
	if intent != nil && kind != KindMeeting {
		return &values.ParseError{Field: fieldMeetingIntent, Code: "not_applicable", Message: "only a meeting has a chosen local start"}
	}
	if intent != nil && precision == PrecisionDay {
		return &values.ParseError{Field: fieldMeetingIntent, Code: "not_applicable", Message: "a whole-day meeting has no local start time"}
	}
	return nil
}

// admitPickedTimes takes the stored row's kind: a patch cannot change it.
func (in *UpdateActivityInput) admitPickedTimes(kind string) error {
	if err := checkMeetingIntent(kind, deref(in.OccurredPrecision), in.MeetingIntent); err != nil {
		return err
	}
	if err := applyPickedTime("meeting_intent", in.MeetingIntent, &in.OccurredAt); err != nil {
		return err
	}
	if err := applyPickedTime("due_intent", in.DueIntent, &in.DueAt); err != nil {
		return err
	}
	if err := in.admitPatchDueTime(); err != nil {
		return err
	}
	return applyPickedTime("remind_intent", in.RemindIntent, &in.RemindAt)
}

// admitBookedStart lets a chosen local start govern the booked instant, and
// the meeting keeps the length the request gave it.
func admitBookedStart(intent *crmcontracts.IntentTime, start, end *time.Time) error {
	if intent == nil {
		return nil
	}
	instant, err := storekit.ResolveIntentTime("start_intent", *intent)
	if err != nil {
		return err
	}
	length := end.Sub(*start)
	*start, *end = instant, instant.Add(length)
	return nil
}

// bookedStartIntent is the chosen local start an appointment carries.
func bookedStartIntent(appointment connector.CalendarAppointment) *crmcontracts.IntentTime {
	if appointment.StartWall == "" {
		return nil
	}
	return &crmcontracts.IntentTime{WallClock: appointment.StartWall, Timezone: appointment.StartZone}
}

func setBookedStartIntent(appointment *connector.CalendarAppointment, intent *crmcontracts.IntentTime) {
	appointment.StartWall, appointment.StartZone = "", ""
	if intent != nil {
		appointment.StartWall, appointment.StartZone = intent.WallClock, intent.Timezone
	}
}

func pickedWall(intent *crmcontracts.IntentTime) *string {
	if intent == nil {
		return nil
	}
	return &intent.WallClock
}

func pickedZone(intent *crmcontracts.IntentTime) *string {
	if intent == nil {
		return nil
	}
	return &intent.Timezone
}
