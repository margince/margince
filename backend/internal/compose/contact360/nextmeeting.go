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
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// meetingParticipantCap bounds who is named. This is "who is in the room" for
// prep, not the attendee list — past a handful the reader is scanning names
// instead of noticing they are single-threaded.
const meetingParticipantCap = 8

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
	participantScope, err := auth.ScopeClauseFor(ctx, "contact", "p", arg)
	if err != nil {
		return err
	}
	if participantScope == "" {
		participantScope = scopeAll
	}
	// The linked deal is a REFERENCE, and it is served as a link the reader is
	// invited to open — so it carries the deal's own scope, exactly as the
	// participants beside it carry the contact's. Without it a rep who may see
	// the meeting is handed the id of a deal whose own read refuses them: a
	// well-formed answer naming a record they cannot open, which is the whole
	// failure this pairing exists to prevent.
	dealScope, err := auth.ScopeClauseFor(ctx, "deal", "d", arg)
	if err != nil {
		return err
	}
	if dealScope == "" {
		dealScope = scopeAll
	}

	var meeting crmcontracts.Contact360NextMeeting
	var activityID ids.UUID
	var subject *string
	var dealID *ids.UUID
	var participants []byte
	err = tx.QueryRow(ctx, fmt.Sprintf(`
		SELECT a.id, a.occurred_at, a.subject,
		       (SELECT dl.deal_id FROM activity_link dl
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
		       ), '[]'::json)
		FROM activity a
		WHERE a.kind = 'meeting' AND a.archived_at IS NULL
		  AND (a.meeting_status IS NULL OR a.meeting_status = 'booked')
		  AND a.occurred_at > $%d
		  AND `+fmt.Sprintf(contactReachesActivity, linkPos)+`
		  AND (%s)%s
		ORDER BY a.occurred_at, a.id
		LIMIT 1`, dealScope, participantScope, nowPos, scope, projectScope(opts, arg)), args...).
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
	if dealID != nil {
		linked := openapi_types.UUID(*dealID)
		meeting.LinkedDealId = &linked
	}
	attendees, err := decodeParticipants(participants)
	if err != nil {
		return err
	}
	meeting.Participants = &attendees
	out.NextMeeting = &meeting
	return nil
}

// attendee is one name in the room.
//
// It is a type ALIAS, not a definition: the contract types Participants as an
// anonymous struct, so the decode has to land in exactly that shape and the
// field spelling is the generator's rather than this package's.
//
//nolint:staticcheck // ST1003: ContactId is the generated contract's spelling; renaming it here would not compile against the wire type
type attendee = struct {
	ContactId openapi_types.UUID `json:"contact_id"`
	FullName  string             `json:"full_name"`
}

// decodeParticipants reads the lateral sub-select's JSON into the wire shape.
//
// It is capped here rather than in SQL so the cap is visible beside the type it
// bounds; the sub-select already carries the row-scope predicate that decides
// WHICH names may appear at all.
func decodeParticipants(raw []byte) ([]attendee, error) {
	var decoded []attendee
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, fmt.Errorf("decode the meeting participants: %w", err)
	}
	if len(decoded) > meetingParticipantCap {
		decoded = decoded[:meetingParticipantCap]
	}
	return decoded, nil
}
