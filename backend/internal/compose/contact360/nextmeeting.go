// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contact360

// The next booked meeting with this contact.
//
// It reads through the CONTACT's own activity-link predicate rather than the
// account's. The two answer different questions and would give different
// answers: the soonest meeting at a company is often one this contact is not
// in, and a page that named it would tell a rep to prepare for a room they are
// not walking into.
//
// Absent means either nothing is booked or the caller cannot read meetings.
// Only `sections_omitted` separates the two, because the wire field is an
// omitempty pointer and nil and absent are the same bytes.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// nextMeetingSection reads the soonest booked meeting and who is in it.
//
// ONE query, not two. Participants come back as JSON from a lateral sub-select
// carrying its own row-scope predicate: a section that read the row and then
// its children is how a composite read starts costing per record.
func (s *Service) nextMeetingSection(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, now time.Time, opts AssembleOptions, out *crmcontracts.Contact360) error {
	if err := requireRead(ctx, "activity"); err != nil {
		return err
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	linkPos := arg(contactID)
	nowPos := arg(now)
	scope, err := activityScope(ctx, arg)
	if err != nil {
		return err
	}
	// Who is in the room and which deal it is about, under the same grants
	// the Worklist names a next meeting's room with.
	room, err := activities.MeetingRoomColumns(ctx, arg)
	if err != nil {
		return err
	}

	var meeting crmcontracts.Contact360NextMeeting
	var activityID ids.UUID
	var subject *string
	var dealID *ids.UUID
	var participants []byte
	err = tx.QueryRow(ctx, fmt.Sprintf(`
		SELECT a.id, a.occurred_at, a.subject, %s
		FROM activity a
		WHERE a.kind = 'meeting' AND a.archived_at IS NULL
		  AND (a.meeting_status IS NULL OR a.meeting_status = 'booked')
		  AND a.occurred_at > $%d
		  AND `+fmt.Sprintf(contactReachesActivity, bind(linkPos))+`
		  AND (%s)%s
		ORDER BY a.occurred_at, a.id
		LIMIT 1`, room, nowPos, scope, projectScope(opts, arg)), args...).
		Scan(&activityID, &meeting.StartsAt, &subject, &dealID, &participants)
	if errors.Is(err, pgx.ErrNoRows) {
		// Nothing booked. That IS the answer, and the strip renders "None".
		return nil
	}
	if err != nil {
		return fmt.Errorf("read the next meeting: %w", err)
	}

	meeting.ActivityId = openapi_types.UUID(activityID)
	meeting.Subject = subject
	inRoom, err := activities.ScanMeetingRoom(dealID, participants)
	if err != nil {
		return err
	}
	inRoom.Onto(&meeting)
	out.NextMeeting = &meeting
	return nil
}
