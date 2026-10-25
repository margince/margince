// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package company360

// The next meeting with this account, and who is in it.
//
// It is a FACT and it sits with the other facts, not with the suggestions: a
// meeting is on the calendar or it is not. The page's `meeting_ahead` moment
// says a meeting is coming and is worth preparing for; this says which one and
// with whom, so the "Prepare" action has something to prefill from.
//
// The section is ABSENT for two opposite reasons, and `sections_omitted` is the
// only thing that separates them: named there, the caller has no activity grant;
// not named, the grant is held and nothing is scheduled. "No meeting booked" is
// a fact a rep acts on; "you cannot see the calendar" is not the same statement,
// and a client that reads the missing field alone tells someone with no calendar
// access to book a meeting that already exists.
//
// The field cannot carry an explicit null to make this easier: the generated
// wire type is an omitempty pointer, so nil and absent are the same bytes. The
// omission list is the mechanism, which is also the convention every other
// section on this payload already follows.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// nextMeetingSection reads the soonest meeting on the account that has not
// started yet, together with the attendees this caller may read.
//
// STARTS_AT, not created_at or due_at: a meeting's identity in time is when it
// happens. Ordering by anything else would put a long-booked meeting behind one
// entered this morning for next month.
//
// A day meeting is next for its whole date (activities.MeetingAheadSQL).
//
// One query, not two. The 360 has a fixed query budget
// (TestCompany360CostDoesNotGrowWithTheAccount). A section that reads the row
// and then its children costs more per account than per page. The deal and
// the attendees come back as columns
// from activities.MeetingRoomColumns, carrying their own grants and row scope.
func nextMeetingSection(
	ctx context.Context, tx pgx.Tx, companyID ids.CompanyID, now time.Time, opts AssembleOptions,
) (*crmcontracts.Company360NextMeeting, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	companyPos := arg(companyID)
	nowPos := arg(now)
	activityScope, err := auth.ActivityContentClause(ctx, "a", arg)
	if err != nil {
		return nil, err
	}
	if activityScope == "" {
		activityScope = scopeAll
	}
	// The room through the one owner Contact360 and the activity detail read,
	// so the company page names the same attendees for the same meeting.
	room, err := activities.MeetingRoomColumns(ctx, arg)
	if err != nil {
		return nil, err
	}

	var (
		meeting   crmcontracts.Company360NextMeeting
		id        ids.UUID
		dealID    *ids.UUID
		attendees []byte
		occurred  *time.Time
	)
	err = tx.QueryRow(ctx, fmt.Sprintf(`
		SELECT a.id, a.occurred_at, a.occurred_precision, coalesce(a.subject, ''), %[3]s
		FROM activity a
		WHERE a.kind = 'meeting' AND a.archived_at IS NULL
		  -- A canceled meeting is still a future row. Offering it as the next
		  -- meeting sends a rep to prepare for something that will not happen,
		  -- and hides the real next one behind it. A NULL status is a manual
		  -- booking nobody has tracked a status on, which is a real meeting.
		  AND (a.meeting_status IS NULL OR a.meeting_status = 'booked')
		  AND %[4]s
		  AND %[1]s AND %[2]s%[5]s
		ORDER BY a.occurred_at, a.id
		LIMIT 1`,
		activityScope, activities.CompanyLinkedActivityExists(companyPos), room,
		activities.MeetingAheadSQL("a", fmt.Sprintf("$%d", nowPos)), opts.projectScope(arg)),
		args...,
	).Scan(&id, &occurred, &meeting.OccurredPrecision, &meeting.Subject, &dealID, &attendees)
	if errors.Is(err, pgx.ErrNoRows) {
		// Nothing scheduled. The caller holds the grant, so this is the account's
		// own state and the page says so.
		return nil, nil //nolint:nilnil // "no meeting booked" is the answer, not an absence of one
	}
	if err != nil {
		return nil, fmt.Errorf("company360: reading the next meeting: %w", err)
	}
	scanned, err := activities.ScanMeetingRoom(dealID, attendees)
	if err != nil {
		return nil, err
	}
	meeting.ActivityId = openapi_types.UUID(id)
	meeting.StartsAt = *occurred
	meeting.LinkedDealId = uuidPtr(scanned.LinkedDealID)
	meeting.Participants = make([]crmcontracts.Company360MeetingParticipant, 0, len(scanned.Attendees))
	for _, attendee := range scanned.Attendees {
		meeting.Participants = append(meeting.Participants, crmcontracts.Company360MeetingParticipant{
			ContactId: attendee.ContactId, DisplayName: attendee.FullName,
		})
	}
	return &meeting, nil
}
