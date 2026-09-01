// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// Filing a captured meeting under the people who were in it.
//
// Mail reaches its records through the counterparty: one address, one ensure,
// one link. A meeting has no counterparty — attendance is a LIST, and the
// calendar mapper says so by leaving Counterparty unset. That is honest about
// the shape and it had a consequence nobody had noticed: with no counterparty
// the tiered gate returns "captured, named nobody", the ensure never runs, and
// a synced meeting was written with participant rows and NO activity_link at
// all. Every calendar meeting in the dev workspace — 517 of them — was
// unreachable from the company and person pages that find activity through
// links, so "Next meeting" was empty on an account you were seeing that day.
//
// The people are already known here: StampFurtherParticipants has just resolved
// each attendee address against person_email in this same transaction. So the
// links are derived from those rows rather than re-resolved, and a meeting whose
// attendee is a contact is filed under that contact.
//
// PERSON links only. A meeting may never link straight to a company
// (activities.refuseACompanyMeeting, and a trigger behind it): the account is
// reached through the attendee's employment, which is exactly what the org
// reach walk's third arm does. Filing the meeting under the person is therefore
// enough to put it on the company page too.
//
// Nothing is CREATED from an invite. An attendee with no person record stays an
// address here; an invitation is not correspondence, and one all-hands with
// forty unknown externals would otherwise buy forty creation decisions nobody
// asked for. Those meetings are picked up later by the cohort repair, in two
// steps: it promotes the attendee's address-only participant row to name the
// new person, then files the meeting under them (people.linkAttendedMeetings).
// The second step is a MEETING-shaped arm on purpose — the repair's other arm
// finds work by counterparty_email, and a meeting carries none, so for a while
// this comment described a pickup that could not happen and every synced
// meeting stayed unlinked.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// meetingKind is the activity kind a calendar connector writes. Spelled here,
// beside the only capture path that treats meetings differently from mail.
const meetingKind = "meeting"

// maxDerivedMeetingLinks bounds how many people one captured meeting is filed
// under.
//
// The ceiling is activities.maxActivityLinks — an activity may be filed under
// at most 25 records, for every transport including a human's own. It is spelled
// again here rather than shared because a module never imports a sibling; what
// must not drift is the NUMBER, and a meeting that reached the cap has stopped
// being a meeting and become a broadcast, where the attendee list is the record
// and the filing is not.
const maxDerivedMeetingLinks = 25

// linkResolvedMeetingParticipants files a captured meeting under each attendee
// the workspace already has a person for, and answers how many links it wrote.
//
// The count is what the audience limiter reads: a meeting that lands with links
// is not a link-less record and must not be held to its participants. It counts
// links actually WRITTEN — a conflict, a cap or an invisible target all mean no
// link — because the limiter's write is deterministic, and claiming a link that
// does not exist would leave the meeting workspace-readable with nothing filing
// it anywhere.
func (s *Sink) linkResolvedMeetingParticipants(
	ctx context.Context, tx pgx.Tx, activityID ids.ActivityID, existing int,
) (int, error) {
	people, err := meetingParticipantsWithPeople(ctx, tx, activityID)
	if err != nil {
		return 0, err
	}
	// The budget is what the row can still take, not a fresh 25: a connector
	// record may already carry links of its own, and the ceiling is on the
	// activity rather than on this step.
	budget := maxDerivedMeetingLinks - existing
	written := 0
	for _, person := range people {
		if written >= budget {
			break
		}
		// A connector may not plant a link to a record its granting human could
		// not see (H1), the same probe linkActivity makes. Not-found is skipped
		// rather than failed: the meeting happened, the participant row already
		// records that this person was in it, and refusing the capture over a
		// filing decision would throw away a message we read successfully.
		if err := auth.EnsureLinkTarget(ctx, tx, "person", person.UUID); err != nil {
			if errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrPermissionDenied) {
				continue
			}
			return 0, fmt.Errorf("capture: meeting link target %s: %w", person, err)
		}
		tag, err := tx.Exec(ctx, `
			INSERT INTO activity_link (activity_id, entity_type, person_id)
			VALUES ($1, 'person', $2)
			ON CONFLICT DO NOTHING`, activityID, person)
		if err != nil {
			return 0, fmt.Errorf("capture: filing a meeting under its attendee: %w", err)
		}
		written += int(tag.RowsAffected())
	}
	return written, nil
}

// meetingParticipantsWithPeople answers the people this meeting's participant
// rows resolved to, organizer first and then by id.
//
// Ordered because the cap can cut the list: whoever called the meeting is the
// most useful filing, and a deterministic order means a replay of the same
// meeting files it the same way rather than under whichever rows came back
// first.
//
// The id is settled against a merge here, for the reason every writer of
// activity_link settles it: no reader walks merged_into_id, so a link written
// to a retired person leaves the meeting on a record nobody opens.
func meetingParticipantsWithPeople(
	ctx context.Context, tx pgx.Tx, activityID ids.ActivityID,
) ([]ids.PersonID, error) {
	// The redirect is followed BEFORE liveness is judged, and the liveness test
	// is on the record the redirect lands on. A merge archives the source and
	// points it at the survivor in one write, so testing the source's own
	// archived_at would drop an attendee whose record simply moved — the meeting
	// would silently lose a link to a person the workspace still has.
	rows, err := tx.Query(ctx, `
		SELECT survivor.id AS person_id,
		       bool_or(ap.role = 'organizer') AS organized
		  FROM activity_participant ap
		  JOIN person p ON p.id = ap.person_id
		  JOIN person survivor ON survivor.id = coalesce(p.merged_into_id, p.id)
		 WHERE ap.activity_id = $1 AND ap.person_id IS NOT NULL
		   AND survivor.archived_at IS NULL
		 GROUP BY survivor.id
		 ORDER BY organized DESC, person_id`, activityID)
	if err != nil {
		return nil, fmt.Errorf("capture: reading a meeting's resolved attendees: %w", err)
	}
	defer rows.Close()
	var out []ids.PersonID
	for rows.Next() {
		var person ids.PersonID
		var organized bool
		if err := rows.Scan(&person, &organized); err != nil {
			return nil, fmt.Errorf("capture: reading a meeting's resolved attendees: %w", err)
		}
		out = append(out, person)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("capture: reading a meeting's resolved attendees: %w", err)
	}
	return out, nil
}
