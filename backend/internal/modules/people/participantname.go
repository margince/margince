// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// Naming a contact from the invitation that named them.
//
// A meeting has no counterparty — attendance is a list, so the calendar mapper
// leaves the field unset and the ensure ladder that names people never runs for
// one. The invitation itself names every attendee in full, which is the only
// full name an attendee-only contact ever gets: minted from a bare address, they
// are called by the local part of their own email and no later pass has better
// evidence to correct it with.
//
// Two orderings need this, and they arrive at different moments. An attendee who
// is already a contact is named as the meeting lands (capture calls the first
// function below). One who becomes a contact weeks later is named by the cohort
// repair, long after that sync (the second). Both feed completePersonName, whose
// guards decide whether the name is an improvement — they never overwrite what a
// human typed.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// FillParticipantNamesTx completes the names of every person this activity's
// participant rows resolved to, from the name the transport gave them.
//
// Package-level rather than a store method: it holds no state, and capture
// reaches it through a function-typed seam compose wires, the same shape the
// audience recompute already travels on. It reads activity_participant, which
// activities owns — a SELECT, so the table-ownership gate (which scans writes)
// is not in question, and PeopleOwedACohortRepair already reads it the same way.
func FillParticipantNamesTx(ctx context.Context, tx pgx.Tx, activityID ids.ActivityID) error {
	// The redirect is followed BEFORE liveness is judged, and liveness is judged
	// on the record the redirect LANDS on. A merge archives the source and
	// points it at the survivor in one write, so testing the source's own
	// archived_at drops an attendee whose record merely moved, and the survivor
	// keeps its local-part name forever. capture.meetingParticipantsWithPeople
	// resolves it the same way, for the same reason.
	//
	// Ordered by the row rather than left to the planner: one person can be on
	// a meeting under two addresses spelled two ways, and completePersonName is
	// one-way, so which spelling wins must not depend on the query plan.
	rows, err := tx.Query(ctx, `
		SELECT survivor.id, ap.display_name, ap.address
		  FROM activity_participant ap
		  JOIN person p ON p.id = ap.person_id
		  JOIN person survivor ON survivor.id = coalesce(p.merged_into_id, p.id)
		 WHERE ap.activity_id = $1
		   AND ap.person_id IS NOT NULL
		   AND coalesce(ap.display_name, '') <> ''
		   AND survivor.archived_at IS NULL
		 ORDER BY survivor.id, ap.address`, activityID)
	if err != nil {
		return fmt.Errorf("people: reading the names an invitation gave: %w", err)
	}
	named, err := scanNamedAttendees(rows)
	if err != nil {
		return err
	}
	for _, attendee := range named {
		// The name is written by whoever sent the invitation, and they are
		// outside this workspace. So the same probe the meeting FILING makes
		// (capture.linkResolvedMeetingParticipants) is made here, and for a
		// sharper reason: filing a meeting under a record puts a message on a
		// page, while this writes the record's own name. Without it an outside
		// organizer could put a colleague's contact on an invitation, type a
		// plausible name for them, and have it stick — on a record they can
		// neither see nor reach.
		//
		// Not-found is skipped rather than failed, exactly as the filing skips
		// it: the meeting happened, the attendee row already records who was
		// there, and refusing the whole capture over a name would throw away a
		// message that was read successfully.
		if err := auth.EnsureLinkTarget(ctx, tx, entityPerson, attendee.person.UUID); err != nil {
			if errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrPermissionDenied) {
				continue
			}
			return fmt.Errorf("people: checking the attendee %s a name was given for: %w", attendee.person, err)
		}
		if _, err := completePersonName(ctx, tx, attendee.person,
			ParsePersonName(attendee.displayName, attendee.address)); err != nil {
			return err
		}
	}
	return nil
}

// namedAttendee is one participant row that carried a name, resolved to the
// person it names.
type namedAttendee struct {
	person      ids.PersonID
	displayName string
	address     string
}

// scanNamedAttendees drains the query before any fill runs. completePersonName
// writes, and writing while a cursor over the same connection is open is what a
// pgx caller may not do.
func scanNamedAttendees(rows pgx.Rows) ([]namedAttendee, error) {
	defer rows.Close()
	var out []namedAttendee
	for rows.Next() {
		var found namedAttendee
		if err := rows.Scan(&found.person, &found.displayName, &found.address); err != nil {
			return nil, fmt.Errorf("people: reading the names an invitation gave: %w", err)
		}
		out = append(out, found)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("people: reading the names an invitation gave: %w", err)
	}
	return out, nil
}

// fillPersonNameFromAttendance names ONE person from the invitations they have
// been on, for the ordering where the meeting synced before they were a contact.
//
// The newest naming invitation wins. A person's name does not change, but the
// spelling an organizer types for them does, and the most recent is the one
// somebody most recently confirmed by sending it.
func fillPersonNameFromAttendance(ctx context.Context, tx pgx.Tx, personID ids.PersonID) error {
	var displayName, address string
	err := tx.QueryRow(ctx, `
		SELECT ap.display_name, coalesce(ap.address, '')
		  FROM activity_participant ap
		  JOIN activity a ON a.id = ap.activity_id
		 WHERE ap.person_id = $1
		   AND coalesce(ap.display_name, '') <> ''
		   AND a.archived_at IS NULL
		 ORDER BY a.occurred_at DESC, a.id DESC
		 LIMIT 1`, personID).Scan(&displayName, &address)
	if err != nil {
		if err == pgx.ErrNoRows {
			// No invitation ever named them: nothing to improve on, which is
			// the ordinary case for a contact who arrived through mail.
			return nil
		}
		return fmt.Errorf("people: reading the name an invitation gave person %s: %w", personID, err)
	}
	_, err = completePersonName(ctx, tx, personID, ParsePersonName(displayName, address))
	return err
}

// rederiveAudiences re-runs the audience derivation over the activities this
// store has just filed under a record.
//
// Nil recompute writes nothing, which is a real composition and not a broken
// one: a deployment assembling people without the timeline still repairs its
// cohorts, and every fixture is nil until it says otherwise. The ids are the
// ones the caller actually linked — never a scan — so a pass that filed nothing
// costs nothing.
// Ordered by id, and that is a LOCK order rather than tidiness. The link insert
// above took a key-share lock on each activity through the foreign key, and the
// recompute upgrades the same rows to FOR UPDATE — so two promotions running for
// two attendees of the same meetings would deadlock if they upgraded in
// different orders. Sorting makes that order the same for every caller.
func (s *Store) rederiveAudiences(ctx context.Context, tx pgx.Tx, activityIDs []ids.UUID) error {
	if s.recomputeAudience == nil {
		return nil
	}
	ordered := slices.Clone(activityIDs)
	slices.SortFunc(ordered, func(a, b ids.UUID) int { return bytes.Compare(a[:], b[:]) })
	for _, id := range ordered {
		if err := s.recomputeAudience(ctx, tx, ids.From[ids.ActivityKind](id)); err != nil {
			return fmt.Errorf("people: re-deriving the audience of %s after filing it: %w", id, err)
		}
	}
	return nil
}
