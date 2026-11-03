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
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
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
// Margince knows no contact for.
type MeetingOther struct {
	Kind string
	Name string
}

// MeetingRoom is a meeting's linked deal and attendees, each only as far as the
// reader may see it. Contacts are named first, then the others, within one cap.
// More counts the attendees past it.
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

// Attendee kinds the room column renders.
const (
	roomContact   = "contact"
	roomColleague = "colleague"
	roomAddress   = "address"
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
	return fmt.Sprintf(`(SELECT dl.deal_id FROM activity_link dl
		          JOIN deal d ON d.id = dl.deal_id AND d.archived_at IS NULL
		         WHERE dl.activity_id = a.id AND (%[1]s) LIMIT 1),
		       COALESCE((
		         SELECT json_agg(json_build_object('kind', room.kind, 'contact_id', room.contact_id, 'name', room.name)
		                         ORDER BY room.rank, room.name, room.sort_key)
		         FROM (
		           SELECT 0 AS rank, '`+roomContact+`' AS kind, p.id AS contact_id, p.full_name AS name, p.id::text AS sort_key
		             FROM (SELECT DISTINCT ap.contact_id FROM activity_participant ap WHERE ap.activity_id = a.id) parts
		             JOIN contact p ON p.id = parts.contact_id AND p.archived_at IS NULL
		            WHERE %[2]s
		           UNION ALL
		           SELECT 1, '`+roomColleague+`', NULL, u.display_name, u.id::text
		             FROM (SELECT DISTINCT ap.user_id FROM activity_participant ap
		                    WHERE ap.activity_id = a.id AND ap.contact_id IS NULL AND ap.user_id IS NOT NULL) seats
		             JOIN app_user u ON u.id = seats.user_id
		           UNION ALL
		           SELECT 2, '`+roomAddress+`', NULL, addressed.name, addressed.address
		             FROM (SELECT DISTINCT ON (lower(ap.address)) lower(ap.address) AS address,
		                          COALESCE(NULLIF(btrim(ap.display_name), ''), ap.address) AS name
		                     FROM activity_participant ap
		                    WHERE ap.activity_id = a.id AND ap.contact_id IS NULL AND ap.user_id IS NULL
		                      AND ap.role IN ('`+connector.ParticipantRoleOrganizer+`', '`+connector.ParticipantRoleAttendee+`')
		                      AND ap.address IS NOT NULL AND btrim(ap.address) <> ''
		                    ORDER BY lower(ap.address), ap.display_name NULLS LAST) addressed
		         ) room
		       ), '[]'::json)`, dealScope, contactScope), nil
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
	var decoded []roomEntry
	if err := json.Unmarshal(attendees, &decoded); err != nil {
		return MeetingRoom{}, fmt.Errorf("activities: decoding the meeting's attendees: %w", err)
	}
	room := MeetingRoom{LinkedDealID: dealID, Attendees: []MeetingAttendee{}}
	if len(decoded) > meetingAttendeeCap {
		room.More = len(decoded) - meetingAttendeeCap
		decoded = decoded[:meetingAttendeeCap]
	}
	for _, entry := range decoded {
		if entry.Kind == roomContact {
			room.Attendees = append(room.Attendees, MeetingAttendee{ContactId: entry.ContactID, FullName: entry.Name})
			continue
		}
		room.Others = append(room.Others, MeetingOther{Kind: entry.Kind, Name: entry.Name})
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
		out = append(out, crmcontracts.MeetingOtherParticipant{Kind: crmcontracts.MeetingOtherParticipantKind(other.Kind), Name: other.Name})
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
