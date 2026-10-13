// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/workforce"
)

const fieldAttendeeUserIDs = "attendee_user_ids"

// roleAttendee is the role a captured calendar meeting gives a colleague the
// provider listed as invited (connector.ParticipantRoleAttendee). A logged
// colleague gets the same role, so the relationship fold cannot tell the two
// sources apart.
const roleAttendee = "attendee"

// AttendeeColleagueError refuses a colleague list the store cannot stamp: too
// long, or naming somebody who is not an active member of this workspace.
type AttendeeColleagueError struct {
	Code    string
	Message string
}

func (e *AttendeeColleagueError) Error() string { return e.Message }

// FieldFault makes the refusal a 422 against attendee_user_ids on both the REST
// and the MCP surface.
func (e *AttendeeColleagueError) FieldFault() (field, code, message string) {
	return fieldAttendeeUserIDs, e.Code, e.Message
}

// stampLoggedColleagues writes one our-side participant row for each colleague
// named as having attended, next to the row for whoever logged it.
//
// The list is checked before any row is written. A seat that is invited,
// suspended, archived or an agent did not attend, and a guessed id must not
// become relationship evidence for anybody.
func stampLoggedColleagues(ctx context.Context, tx pgx.Tx, activityID ids.ActivityID, attendees []ids.UserID, logger ids.UUID) error {
	if len(attendees) == 0 {
		return nil
	}
	if len(attendees) > maxActivityLinks {
		return &AttendeeColleagueError{Code: "too_many_attendees", Message: fmt.Sprintf(
			"a logged activity may name at most %d colleagues who attended; this one names %d",
			maxActivityLinks, len(attendees))}
	}
	distinct := make([]ids.UUID, 0, len(attendees))
	seen := make(map[ids.UUID]bool, len(attendees))
	for _, attendee := range attendees {
		if !seen[attendee.UUID] {
			seen[attendee.UUID] = true
			distinct = append(distinct, attendee.UUID)
		}
	}
	var members int
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FROM app_user
		 WHERE id = ANY($1::uuid[]) AND NOT is_agent AND `+workforce.LiveMemberSQL(""),
		distinct).Scan(&members); err != nil {
		return fmt.Errorf("activities: checking the colleagues who attended: %w", err)
	}
	if members != len(distinct) {
		return &AttendeeColleagueError{Code: "not_a_colleague", Message: "attendee_user_ids names somebody " +
			"who is not an active member of this workspace; name colleagues from the member list"}
	}
	for _, attendee := range distinct {
		// The logger already has their own row; a second one under another role
		// would make them two parties to one meeting.
		if attendee == logger {
			continue
		}
		if err := insertLoggedParticipant(ctx, tx, activityID, roleAttendee, &attendee, nil); err != nil {
			return err
		}
	}
	return nil
}
