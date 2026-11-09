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
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/workforce"
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

// MeetingOther is an attendee who is not a contact: a colleague, or an address
// no contact or seat on the meeting holds.
type MeetingOther struct {
	Kind crmcontracts.MeetingOtherParticipantKind
	Name string
}

// MeetingRoom is a meeting's linked deal and attendees, each only as far as the
// reader may see it. Contacts are named first, then the others, within one cap.
// More counts the attendees past it, and the guests capture left off a crowd.
type MeetingRoom struct {
	LinkedDealID *ids.UUID
	Attendees    []MeetingAttendee
	Others       []MeetingOther
	More         int
}

// roomEntry is one attendee as the room column renders it.
type roomEntry struct {
	Kind      string             `json:"kind"`
	ContactID openapi_types.UUID `json:"contact_id"`
	Name      string             `json:"name"`
}

// roomColumn is the room column: the attendees on file, and how many guests a
// crowded meeting stores no row for.
type roomColumn struct {
	Attendees []roomEntry `json:"attendees"`
	LeftOut   int         `json:"left_out"`
}

// Attendee kinds the room column renders.
const (
	roomContact   = "contact"
	roomColleague = string(crmcontracts.MeetingOtherParticipantKindColleague)
	roomAddress   = string(crmcontracts.MeetingOtherParticipantKindAddress)
)

// MeetingRoomColumns renders two SELECT columns over `activity a`: the linked
// deal and a JSON list of attendees. Scan them into a *ids.UUID and a []byte,
// then hand both to ScanMeetingRoom.
//
// Each column carries its record's OBJECT grant as well as its row scope: a row
// scope narrows a set the grant already opened and renders nothing for an
// unbounded seat, so on its own it would name contacts to a seat that may not
// read contacts. A clause is built only when used, because it binds arguments.
func MeetingRoomColumns(ctx context.Context, arg func(any) int) (string, error) {
	dealScope, err := readableScope(ctx, auth.ReadGranted(ctx, "deal"), linkEntityDeal, "d", arg)
	if err != nil {
		return "", err
	}
	contactScope, err := readableScope(ctx, auth.ReadGranted(ctx, "contact"), linkEntityContact, "p", arg)
	if err != nil {
		return "", err
	}
	// DISTINCT because one attendee holds several roles on one meeting (a
	// captured invite makes its sender `from` and `attendee`) and is in the room
	// once. A row naming a contact is that contact; one naming only a seat is a
	// colleague; one naming only an address is that address.
	//
	// This renders in the card's own query, so it cannot call ColleagueSeats or
	// the guest read. Their rule holds here. A seat attends through a standing
	// organizer or attendee row, since owning a calendar is not attending. Only a
	// live human is a colleague. An address also held by a contact or a seat on
	// this meeting is that attendee, not a second one. A contact whose email is
	// the address a listed colleague attends by is that colleague, never a
	// second entry; names alone never merge two attendees.
	return fmt.Sprintf(`(SELECT dl.deal_id FROM activity_link dl
		          JOIN deal d ON d.id = dl.deal_id AND d.archived_at IS NULL
		         WHERE dl.activity_id = a.id AND (%[1]s) LIMIT 1),
		       json_build_object('left_out', a.guests_left_out, 'attendees', COALESCE((
		         SELECT json_agg(json_build_object('kind', room.kind, 'contact_id', room.contact_id, 'name', room.name)
		                         ORDER BY room.rank, room.name, room.sort_key)
		         FROM (
		           SELECT 0 AS rank, '`+roomContact+`' AS kind, p.id AS contact_id, p.full_name AS name, p.id::text AS sort_key
		             FROM (SELECT DISTINCT ap.contact_id FROM activity_participant ap WHERE ap.activity_id = a.id) parts
		             JOIN contact p ON p.id = parts.contact_id AND p.archived_at IS NULL
		            WHERE %[2]s
		              AND NOT EXISTS (SELECT 1 FROM activity_participant seat
		                               JOIN contact_email ce ON ce.contact_id = p.id AND ce.archived_at IS NULL
		                                AND ce.email = lower(seat.address)
		                               JOIN app_user su ON su.id = seat.user_id AND %[7]s
		                              WHERE seat.activity_id = a.id AND seat.contact_id IS NULL
		                                AND seat.role IN (%[3]s) AND %[6]s)
		           UNION ALL
		           SELECT 1, '`+roomColleague+`', NULL, u.display_name, u.id::text
		             FROM (SELECT DISTINCT ap.user_id FROM activity_participant ap
		                    WHERE ap.activity_id = a.id AND ap.contact_id IS NULL AND ap.user_id IS NOT NULL
		                      AND ap.role IN (%[3]s) AND %[4]s) seats
		             JOIN app_user u ON u.id = seats.user_id AND %[5]s
		           UNION ALL
		           SELECT 2, '`+roomAddress+`', NULL, addressed.name, addressed.address
		             FROM (SELECT DISTINCT ON (lower(ap.address)) lower(ap.address) AS address,
		                          COALESCE(NULLIF(btrim(ap.display_name), ''), ap.address) AS name
		                     FROM activity_participant ap
		                    WHERE ap.activity_id = a.id AND ap.contact_id IS NULL AND ap.user_id IS NULL
		                      AND ap.role IN (%[3]s)
		                      AND ap.address IS NOT NULL AND btrim(ap.address) <> ''
		                      AND NOT EXISTS (SELECT 1 FROM activity_participant held
		                                       WHERE held.activity_id = a.id AND lower(held.address) = lower(ap.address)
		                                         AND (held.contact_id IS NOT NULL OR held.user_id IS NOT NULL))
		                    ORDER BY lower(ap.address), NULLIF(btrim(ap.display_name), '') NULLS LAST) addressed
		         ) room
		       ), '[]'::json))`, dealScope, contactScope, storekit.MeetingAttendingRolesSQL,
		auth.ParticipantStandsSQL("ap", "a"), workforce.LiveHumanSQL("u"),
		auth.ParticipantStandsSQL("seat", "a"), workforce.LiveHumanSQL("su")), nil
}

// readableScope is one record kind's visibility for MeetingRoomColumns: nothing
// without the object grant, its row scope with it. The grant is asked by the
// caller, where the object is a literal the reader censuses can see.
func readableScope(ctx context.Context, granted bool, object, alias string, arg func(any) int) (string, error) {
	if !granted {
		return scopeNothing, nil
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
	var column roomColumn
	if err := json.Unmarshal(attendees, &column); err != nil {
		return MeetingRoom{}, fmt.Errorf("activities: decoding the meeting's attendees: %w", err)
	}
	room := MeetingRoom{LinkedDealID: dealID, Attendees: []MeetingAttendee{}, More: column.LeftOut}
	decoded := column.Attendees
	if len(decoded) > meetingAttendeeCap {
		room.More += len(decoded) - meetingAttendeeCap
		decoded = decoded[:meetingAttendeeCap]
	}
	for _, entry := range decoded {
		if entry.Kind == roomContact {
			room.Attendees = append(room.Attendees, MeetingAttendee{ContactId: entry.ContactID, FullName: entry.Name})
			continue
		}
		room.Others = append(room.Others, MeetingOther{Kind: crmcontracts.MeetingOtherParticipantKind(entry.Kind), Name: entry.Name})
	}
	return room, nil
}

// Onto writes the room onto the wire's next-meeting shape.
func (r MeetingRoom) Onto(meeting *crmcontracts.Contact360NextMeeting) {
	if r.LinkedDealID != nil {
		linked := openapi_types.UUID(*r.LinkedDealID)
		meeting.LinkedDealId = &linked
	}
	attendees := r.Attendees
	meeting.Participants = &attendees
	meeting.OtherParticipants, meeting.ParticipantsMore = r.OtherParticipants(), r.MoreCount()
}

// OtherParticipants is the room's non-contact attendees on the wire, absent when
// there are none.
func (r MeetingRoom) OtherParticipants() *[]crmcontracts.MeetingOtherParticipant {
	if len(r.Others) == 0 {
		return nil
	}
	out := make([]crmcontracts.MeetingOtherParticipant, 0, len(r.Others))
	for _, other := range r.Others {
		out = append(out, crmcontracts.MeetingOtherParticipant{Kind: other.Kind, Name: other.Name})
	}
	return &out
}

// MoreCount is how many attendees the room does not name, absent when it names
// them all.
func (r MeetingRoom) MoreCount() *int {
	if r.More == 0 {
		return nil
	}
	more := r.More
	return &more
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
