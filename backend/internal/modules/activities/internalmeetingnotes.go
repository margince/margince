// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"fmt"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/meetingoutcome"
)

const fieldMeetingNotes = "meeting_notes"

func (in *UpdateActivityInput) spellClearedMeetingNotes() {
	if in.MeetingNotes == nil && in.clears(fieldMeetingNotes) {
		in.MeetingNotes = new("")
	}
}

func (in UpdateActivityInput) meetingNotesAssignment(arg func(any) int) string {
	if in.MeetingNotes == nil {
		return ""
	}
	return fmt.Sprintf(", meeting_notes = $%d", arg(in.MeetingNotes))
}

func (in *UpdateActivityInput) alignMeetingNotes(current crmcontracts.Activity) error {
	if in.MeetingNotes != nil && current.Kind != crmcontracts.ActivityKindMeeting {
		return &KindFieldError{Field: fieldMeetingNotes, Only: "a meeting"}
	}
	if current.Kind != crmcontracts.ActivityKindMeeting || !meetingoutcome.LegacyManualNotes(current.SourceSystem, current.InvitationStatus != nil, current.CapturedBy) {
		return nil
	}
	// A manual meeting's body is its legacy notes field. Old clients must see the same notes.
	if in.MeetingNotes != nil {
		if in.Body != nil && *in.Body != *in.MeetingNotes {
			return &followUpFieldError{Field: fieldMeetingNotes, Message: "Send the notes in one field, or the same notes in both fields."}
		}
		in.Body = in.MeetingNotes
	} else if in.Body != nil && current.MeetingNotes != nil {
		in.MeetingNotes = in.Body
	}
	return nil
}

// MeetingRecordBody selects the rep's notes when they exist, including an explicit clear.
// The read projection resolves proved legacy calendar edits; manual entries retain their body.
func MeetingRecordBody(activity crmcontracts.Activity) *string {
	if activity.Kind == crmcontracts.ActivityKindMeeting {
		if activity.MeetingNotes != nil {
			return activity.MeetingNotes
		}
		if !meetingoutcome.LegacyManualNotes(activity.SourceSystem, activity.InvitationStatus != nil, activity.CapturedBy) {
			return nil
		}
	}
	return activity.Body
}

func meetingNotesText(meeting crmcontracts.Activity) string {
	return deref(MeetingRecordBody(meeting))
}
