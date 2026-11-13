// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// A transcript logged as the record of an open meeting. The transcript stays
// its own entry, so it is stored and read as one. The meeting it records takes
// the outcome, so a held count counts that meeting once.

import (
	"context"
	"slices"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/meetingoutcome"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const fieldRecordsMeetingID = "records_meeting_id"

// filingKinds are the records a meeting is filed on, beside the contacts it
// was with. A company is not one: a meeting is refused a company link.
var filingKinds = []string{linkEntityDeal, linkEntityProject, linkEntityLead}

// recordedMeetingFrom admits records_meeting_id. It also keeps
// meetingoutcome.RecordsMeetingSQL exact: a meeting transcript answering a task
// must carry its own outcome.
func recordedMeetingFrom(req crmcontracts.CreateActivityRequest, in *LogActivityInput) error {
	meetingTranscript := req.Kind == crmcontracts.CreateActivityRequestKindCreateActivityRequestKindMeeting &&
		req.SourceSystem != nil && *req.SourceSystem == transcriptSourceSystem
	if req.RecordsMeetingId == nil {
		if meetingTranscript && req.SourceActivityId != nil && req.MeetingStatus == nil {
			return &followUpFieldError{
				Field:   auditFieldMeetingStatus,
				Message: "A meeting transcript that answers a task needs its meeting_status.",
			}
		}
		return nil
	}
	if !meetingTranscript || req.SourceActivityId != nil || req.RequestActivityId != nil {
		return &followUpFieldError{
			Field:   fieldRecordsMeetingID,
			Message: "Only a meeting transcript can record a meeting, and it names no other source.",
		}
	}
	in.recordsMeeting = new(ids.From[ids.ActivityKind](ids.UUID(*req.RecordsMeetingId)))
	return nil
}

// sourceActivity is the stored source_activity_id: the meeting a transcript
// records, else the source the caller named.
func (in LogActivityInput) sourceActivity() *ids.UUID {
	if in.recordsMeeting != nil {
		return &in.recordsMeeting.UUID
	}
	return in.SourceActivityID
}

// admitRecordedMeeting locks the meeting the transcript records and dates the
// transcript at it. The lock holds until the outcome lands, so a second answer
// to the same meeting waits and is then refused.
func admitRecordedMeeting(ctx context.Context, tx pgx.Tx, in *LogActivityInput) (crmcontracts.Activity, error) {
	if err := auth.Require(ctx, "activity", principal.ActionUpdate); err != nil {
		return crmcontracts.Activity{}, err
	}
	id := *in.recordsMeeting
	if _, err := storekit.LockRow(ctx, tx, "activity", id.UUID, storekit.LiveOnly); err != nil {
		return crmcontracts.Activity{}, err
	}
	if err := auth.EnsureActivityWritable(ctx, tx, id.UUID); err != nil {
		return crmcontracts.Activity{}, err
	}
	meeting, err := readActivityContent(ctx, tx, id, storekit.LiveOnly)
	if err != nil {
		return crmcontracts.Activity{}, err
	}
	if !owesOutcome(meeting) {
		return crmcontracts.Activity{}, &followUpFieldError{
			Field:   fieldRecordsMeetingID,
			Message: "Choose a meeting that still waits for its outcome.",
		}
	}
	in.OccurredAt, in.OccurredPrecision, in.MeetingIntent = &meeting.OccurredAt, string(meeting.OccurredPrecision), nil
	// The outcome is the meeting's; the transcript only records it.
	in.MeetingStatus = nil
	in.audienceOf = &id
	in.Links = append(in.Links, linksOfKinds(meeting, filingKinds)...)
	return meeting, nil
}

// owesOutcome is OpenMeetingStatusSQL for a meeting read into Go.
func owesOutcome(meeting crmcontracts.Activity) bool {
	open := meeting.MeetingStatus == nil || *meeting.MeetingStatus == crmcontracts.ActivityMeetingStatusBooked
	transcript := meeting.SourceSystem != nil && *meeting.SourceSystem == transcriptSourceSystem
	return meeting.Kind == crmcontracts.ActivityKindMeeting && open && !transcript
}

// markRecordedMeetingHeld writes the outcome on the meeting and files it on
// the records the transcript is filed on, for each kind it has none of. A
// meeting already on a deal keeps that deal.
func markRecordedMeetingHeld(
	ctx context.Context, tx pgx.Tx, meeting, transcript crmcontracts.Activity, in LogActivityInput,
) error {
	held := meetingoutcome.Held
	id := ids.From[ids.ActivityKind](ids.UUID(meeting.Id))
	if _, err := updateActivityInTx(ctx, tx, id, UpdateActivityInput{MeetingStatus: &held, recordedAt: in.recordedAt}); err != nil {
		return err
	}
	missing := slices.DeleteFunc(slices.Clone(filingKinds), func(kind string) bool {
		return len(linksOfKinds(meeting, []string{kind})) > 0
	})
	for _, link := range linksOfKinds(transcript, missing) {
		if _, err := RelinkActivityInTx(ctx, tx, id, RelinkActivityInput{EntityType: link.EntityType, EntityID: link.EntityID}); err != nil {
			return err
		}
	}
	return nil
}

func linksOfKinds(activity crmcontracts.Activity, kinds []string) []ActivityLinkInput {
	var links []ActivityLinkInput
	for _, link := range filedUnder(activity) {
		if slices.Contains(kinds, link.EntityType) {
			links = append(links, link)
		}
	}
	return links
}
