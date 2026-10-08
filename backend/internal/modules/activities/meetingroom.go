// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// Who is in a booked meeting's room and which deal it is about, as one reader
// may see them. The contact page and the Worklist both name the room of a next
// meeting, so both render it from here and cannot disagree about whose names
// a reader is shown.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// meetingAttendeeCap bounds who is named. This is "who is in the room" for
// prep, not the attendee list. Past a handful the reader is scanning names
// instead of noticing they are single-threaded.
const meetingAttendeeCap = 8

// MeetingAttendee is one name in the room. An alias, because the contract types
// the participant list as an anonymous struct and the decode has to land in
// that shape.
//
//nolint:staticcheck // ST1003: ContactId is the generated contract's spelling; renaming it here would not compile against the wire type
type MeetingAttendee = struct {
	ContactId openapi_types.UUID `json:"contact_id"`
	FullName  string             `json:"full_name"`
}

// MeetingRoom is a meeting's linked deal and attendees, each only as far as the
// reader may see it.
type MeetingRoom struct {
	LinkedDealID *ids.UUID
	Attendees    []MeetingAttendee
}

// MeetingRoomColumns renders two SELECT columns over `activity a`: the linked
// deal and a JSON list of attendees. Scan them into a *ids.UUID and a []byte,
// then hand both to ScanMeetingRoom.
//
// Each column carries its record's OBJECT grant as well as its row scope: a row
// scope narrows a set the grant already opened and renders nothing for an
// unbounded seat, so on its own it would name contacts to a seat that may not
// read contacts. A clause is built only when used, because it binds arguments.
func MeetingRoomColumns(ctx context.Context, arg func(any) int) (string, error) {
	dealScope, err := readableScope(ctx, linkEntityDeal, "d", arg)
	if err != nil {
		return "", err
	}
	contactScope, err := readableScope(ctx, linkEntityContact, "p", arg)
	if err != nil {
		return "", err
	}
	// DISTINCT because one contact holds several roles on one meeting (a
	// captured invite makes its sender `from` and `attendee`) and is in the room once.
	return fmt.Sprintf(`(SELECT dl.deal_id FROM activity_link dl
		          JOIN deal d ON d.id = dl.deal_id AND d.archived_at IS NULL
		         WHERE dl.activity_id = a.id AND (%s) LIMIT 1),
		       COALESCE((
		         SELECT json_agg(json_build_object('contact_id', p.id, 'full_name', p.full_name)
		                         ORDER BY p.full_name, p.id)
		         FROM (
		           SELECT DISTINCT ap.contact_id
		           FROM activity_participant ap
		           WHERE ap.activity_id = a.id
		         ) parts
		         JOIN contact p ON p.id = parts.contact_id AND p.archived_at IS NULL
		         WHERE %s
		       ), '[]'::json)`, dealScope, contactScope), nil
}

// readableScope is one record kind's visibility for MeetingRoomColumns: nothing
// without the object grant, its row scope with it.
func readableScope(ctx context.Context, object, alias string, arg func(any) int) (string, error) {
	if err := auth.Require(ctx, object, principal.ActionRead); err != nil {
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			return scopeNothing, nil
		}
		return "", err
	}
	clause, err := auth.ScopeClauseFor(ctx, object, alias, arg)
	if err != nil {
		return "", err
	}
	return orUnbounded(clause), nil
}

// ScanMeetingRoom turns the two columns MeetingRoomColumns rendered into a room,
// capped here rather than in SQL so the cap sits beside the type it bounds.
func ScanMeetingRoom(dealID *ids.UUID, attendees []byte) (MeetingRoom, error) {
	var decoded []MeetingAttendee
	if err := json.Unmarshal(attendees, &decoded); err != nil {
		return MeetingRoom{}, fmt.Errorf("activities: decoding the meeting's attendees: %w", err)
	}
	if len(decoded) > meetingAttendeeCap {
		decoded = decoded[:meetingAttendeeCap]
	}
	return MeetingRoom{LinkedDealID: dealID, Attendees: decoded}, nil
}

// Onto writes the room onto the wire's next-meeting shape.
func (r MeetingRoom) Onto(meeting *crmcontracts.Contact360NextMeeting) {
	if r.LinkedDealID != nil {
		linked := openapi_types.UUID(*r.LinkedDealID)
		meeting.LinkedDealId = &linked
	}
	attendees := r.Attendees
	meeting.Participants = &attendees
}

// MeetingRoom answers the room of one meeting this reader may read. found is
// false for a meeting they may not read, so the answer cannot confirm one exists.
func (s *Store) MeetingRoom(ctx context.Context, meetingID ids.UUID) (room MeetingRoom, found bool, err error) {
	if err := auth.Require(ctx, linkEntityActivity, principal.ActionRead); err != nil {
		return MeetingRoom{}, false, err
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	meeting := arg(meetingID)
	scope, err := auth.ActivityContentClause(ctx, "a", arg)
	if err != nil {
		return MeetingRoom{}, false, err
	}
	columns, err := MeetingRoomColumns(ctx, arg)
	if err != nil {
		return MeetingRoom{}, false, err
	}
	var dealID *ids.UUID
	var attendees []byte
	err = s.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, fmt.Sprintf(`SELECT %s FROM activity a
			WHERE a.id = $%d AND %s AND %s`, columns, meeting, activityLive, orUnbounded(scope)),
			args...).Scan(&dealID, &attendees)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return MeetingRoom{}, false, nil
	}
	if err != nil {
		return MeetingRoom{}, false, fmt.Errorf("activities: reading the meeting's room: %w", err)
	}
	room, err = ScanMeetingRoom(dealID, attendees)
	return room, err == nil, err
}
