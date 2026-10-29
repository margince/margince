// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// An attendee's own "I'm ready" for a meeting. It holds for that attendee
// alone and only at the preparation revision they confirmed. A moved start, a
// new agenda or a changed guest list asks them to look again.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// MeetingPreparations answers the signed-in attendee's preparation state for
// each of these meetings. A meeting is absent when the caller may not read its
// content or does not attend it: nobody else's readiness is theirs to show.
func (s *Store) MeetingPreparations(ctx context.Context, meetingIDs []ids.UUID) (map[ids.UUID]MeetingPreparation, error) {
	out := make(map[ids.UUID]MeetingPreparation, len(meetingIDs))
	if len(meetingIDs) == 0 {
		return out, nil
	}
	attendee, err := readinessAttendee(ctx)
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	if err := auth.Require(ctx, linkEntityActivity, principal.ActionRead); err != nil {
		return nil, err
	}
	err = s.tx(ctx, func(tx pgx.Tx) error {
		read, err := readPreparations(ctx, tx, attendee, meetingIDs, false)
		out = read
		return err
	})
	return out, err
}

// ConfirmMeetingReady records that the signed-in attendee, or the agent acting
// for them, is ready for the meeting at the revision they were shown. A
// revision the meeting has moved past is refused, so an old screen cannot
// confirm a meeting that changed under it.
func (s *Store) ConfirmMeetingReady(ctx context.Context, id ids.ActivityID, revision string) (MeetingPreparation, error) {
	return s.changeReadiness(ctx, id, revision, func(tx pgx.Tx, attendee ids.UUID, current meetingFacts, by string) (bool, error) {
		if current.revision != revision {
			return false, fmt.Errorf("activities: the meeting changed since its preparation was read: %w", apperrors.ErrVersionSkew)
		}
		tag, err := tx.Exec(ctx, `
			INSERT INTO meeting_readiness (attendee_id, activity_id, revision, starts_at, starts_on_day,
			                               agenda_fingerprint, participants_fingerprint, captured_by, confirmed_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			ON CONFLICT (attendee_id, activity_id) DO UPDATE
			   SET revision = EXCLUDED.revision,
			       starts_at = EXCLUDED.starts_at, starts_on_day = EXCLUDED.starts_on_day,
			       agenda_fingerprint = EXCLUDED.agenda_fingerprint,
			       participants_fingerprint = EXCLUDED.participants_fingerprint,
			       captured_by = EXCLUDED.captured_by, confirmed_at = EXCLUDED.confirmed_at
			 WHERE meeting_readiness.revision <> EXCLUDED.revision`,
			attendee, id, revision, current.facts.StartsAt, current.facts.StartsOnDay,
			current.facts.Agenda, current.facts.Participants, by, s.now().UTC())
		if err != nil {
			return false, fmt.Errorf("activities: recording that the attendee is ready: %w", err)
		}
		return tag.RowsAffected() > 0, nil
	}, readinessReady)
}

// WithdrawMeetingReady is the undo behind ConfirmMeetingReady. It removes the
// confirmation only while it is still the one given at revision. The undo of
// an old press cannot take back a newer confirmation.
func (s *Store) WithdrawMeetingReady(ctx context.Context, id ids.ActivityID, revision string) (MeetingPreparation, error) {
	return s.changeReadiness(ctx, id, revision, func(tx pgx.Tx, attendee ids.UUID, _ meetingFacts, _ string) (bool, error) {
		tag, err := tx.Exec(ctx,
			`DELETE FROM meeting_readiness WHERE attendee_id = $1 AND activity_id = $2 AND revision = $3`,
			attendee, id, revision)
		if err != nil {
			return false, fmt.Errorf("activities: withdrawing the attendee's readiness: %w", err)
		}
		return tag.RowsAffected() > 0, nil
	}, readinessWithdrawn)
}

// The two states the readiness event announces.
const (
	readinessReady     = "ready"
	readinessWithdrawn = "withdrawn"
)

// readinessWrite changes the attendee's row and reports whether it did.
type readinessWrite func(tx pgx.Tx, attendee ids.UUID, current meetingFacts, capturedBy string) (bool, error)

// changeReadiness is the gate and the write shape both readiness verbs share.
// The caller must read the meeting's content and attend it. A change lands
// with its audit row and event in one transaction.
func (s *Store) changeReadiness(
	ctx context.Context, id ids.ActivityID, revision string, write readinessWrite, state string,
) (MeetingPreparation, error) {
	if revision == "" {
		return MeetingPreparation{}, &values.ParseError{
			Field: "preparation_revision", Code: codeRequired,
			Message: "send the preparation revision the meeting was read at",
		}
	}
	if err := auth.Require(ctx, linkEntityActivity, principal.ActionRead); err != nil {
		return MeetingPreparation{}, err
	}
	attendee, err := readinessAttendee(ctx)
	if err != nil {
		return MeetingPreparation{}, err
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return MeetingPreparation{}, err
	}
	var out MeetingPreparation
	err = s.tx(ctx, func(tx pgx.Tx) error {
		if err := auth.EnsureActivityContentVisibleLive(ctx, tx, id.UUID); err != nil {
			return err
		}
		current, err := lockMeetingFacts(ctx, tx, attendee, id.UUID)
		if err != nil {
			return err
		}
		changed, err := write(tx, attendee, current, by)
		if err != nil {
			return err
		}
		if changed {
			if err := recordReadiness(ctx, tx, id.UUID, revision, state); err != nil {
				return err
			}
		}
		read, err := readPreparations(ctx, tx, attendee, []ids.UUID{id.UUID}, true)
		out = read[id.UUID]
		return err
	})
	return out, err
}

// recordReadiness is the audit row and the event, in the readiness write's
// transaction. The attendee is the audit row's actor, or the user an agent
// acted for; the event names nobody.
func recordReadiness(ctx context.Context, tx pgx.Tx, id ids.UUID, revision, state string) error {
	after := map[string]any{"readiness": state}
	evidence := map[string]any{"preparation_revision": revision}
	auditID, err := storekit.AuditEventWithEvidence(ctx, tx, "update", linkEntityActivity, id, after, evidence)
	if err != nil {
		return err
	}
	return storekit.EmitEvent(ctx, tx, auditID, id, crmcontracts.PublicEventActivityReadinessRecorded{
		ActivityId:          openapi_types.UUID(id),
		State:               crmcontracts.PublicEventActivityReadinessRecordedState(state),
		PreparationRevision: revision,
	})
}

// readinessAttendee is whose readiness a call reads or writes: the signed-in
// user, or the user an agent acts for. Nobody else has a meeting to be
// ready for.
func readinessAttendee(ctx context.Context) (ids.UUID, error) {
	actor, ok := principal.Actor(ctx)
	switch {
	case !ok:
		return ids.UUID{}, apperrors.ErrPermissionDenied
	case actor.Type == principal.PrincipalHuman && !actor.UserID.IsZero():
		return actor.UserID, nil
	case actor.ActsAsItsUser():
		return actor.OnBehalfOf, nil
	}
	return ids.UUID{}, apperrors.ErrPermissionDenied
}

// fieldMeetingID is the request field naming the meeting a readiness verb acts on.
const fieldMeetingID = "activity_id"

// meetingFacts is a meeting's current facts as the write reads them.
type meetingFacts struct {
	facts    PreparationFacts
	revision string
}

// lockMeetingFacts reads the meeting the attendee confirms, holding the row so
// its start and agenda cannot move between the compare and the write. A row
// that is not a meeting, or not one the attendee attends, answers 422.
func lockMeetingFacts(ctx context.Context, tx pgx.Tx, attendee, id ids.UUID) (meetingFacts, error) {
	var (
		kind, precision, agenda string
		startsAt                time.Time
		attends                 bool
		guests                  []string
		current                 meetingFacts
	)
	query := storekit.SQLf(`
		SELECT a.kind, a.occurred_at, a.occurred_precision, COALESCE(a.agenda_fingerprint, ''), %s, %s
		  FROM activity a WHERE a.id = $1 AND `+activityLive+`
		   FOR SHARE OF a`, participantsSQL, storekit.MeetingResponsibilitySQL("a", "$2"))
	if err := tx.QueryRow(ctx, query, id, attendee).Scan(
		&kind, &startsAt, &precision, &agenda, &guests, &attends); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return meetingFacts{}, apperrors.ErrNotFound
		}
		return meetingFacts{}, fmt.Errorf("activities: reading the meeting's preparation facts: %w", err)
	}
	if kind != KindMeeting {
		return meetingFacts{}, &values.ParseError{
			Field: fieldMeetingID, Code: "not_a_meeting", Message: "only a meeting has a preparation to be ready for",
		}
	}
	if !attends {
		return meetingFacts{}, &values.ParseError{
			Field: fieldMeetingID, Code: "not_an_attendee", Message: "only an attendee of the meeting can say they are ready for it",
		}
	}
	current.facts = preparationFactsOf(startsAt, precision, agenda, guests)
	current.revision = current.facts.Revision()
	return current, nil
}

// readPreparations reads the attendee's state for each meeting they attend
// and may read. trusted skips the content clause for a meeting the caller's
// transaction has already checked.
func readPreparations(
	ctx context.Context, tx pgx.Tx, attendee ids.UUID, meetingIDs []ids.UUID, trusted bool,
) (map[ids.UUID]MeetingPreparation, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	content := scopeUnbounded
	if !trusted {
		clause, err := auth.ActivityContentClause(ctx, "a", arg)
		if err != nil {
			return nil, err
		}
		content = orUnbounded(clause)
	}
	seat := sprintf("$%d", arg(attendee))
	query := storekit.SQLf(preparationsQuery, participantsSQL, seat, arg(meetingIDs), content,
		storekit.MeetingResponsibilitySQL("a", seat))
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("activities: reading meeting preparations: %w", err)
	}
	defer rows.Close()
	out := make(map[ids.UUID]MeetingPreparation, len(meetingIDs))
	for rows.Next() {
		var (
			id                       ids.UUID
			startsAt                 time.Time
			precision, invite        string
			roster                   []string
			revision, agenda, guests *string
			confirmedAt              *time.Time
			confirmedOnDay           *bool
		)
		if err := rows.Scan(&id, &startsAt, &precision, &invite, &roster,
			&revision, &confirmedAt, &confirmedOnDay, &agenda, &guests); err != nil {
			return nil, fmt.Errorf("activities: reading a meeting preparation: %w", err)
		}
		var stored *storedReadiness
		if revision != nil && confirmedAt != nil && confirmedOnDay != nil && agenda != nil && guests != nil {
			stored = &storedReadiness{revision: *revision, facts: PreparationFacts{
				StartsAt: confirmedAt.UTC(), StartsOnDay: *confirmedOnDay, Agenda: *agenda, Participants: *guests,
			}}
		}
		out[id] = preparationAt(id, preparationFactsOf(startsAt, precision, invite, roster), stored)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("activities: reading meeting preparations: %w", err)
	}
	return out, nil
}

// preparationsQuery reads each meeting's facts beside the attendee's stored
// confirmation, for meetings the attendee attends.
const preparationsQuery = `
	SELECT a.id, a.occurred_at, a.occurred_precision, COALESCE(a.agenda_fingerprint, ''), %s,
	       r.revision, r.starts_at, r.starts_on_day, r.agenda_fingerprint, r.participants_fingerprint
	  FROM activity a
	  LEFT JOIN meeting_readiness r ON r.activity_id = a.id AND r.attendee_id = %s
	 WHERE a.id = ANY($%d) AND a.kind = 'meeting' AND ` + activityLive + ` AND %s AND %s`

// participantsSQL names each participant by its most stable identity. A
// calendar's address stays when capture later resolves it to a contact. So
// resolving a guest does not read as a changed guest list.
const participantsSQL = `COALESCE((SELECT array_agg(COALESCE('a:' || lower(p.address), 'u:' || p.user_id::text,
	                                  'c:' || p.contact_id::text, 'h:' || p.channel_user_id))
	           FROM activity_participant p WHERE p.activity_id = a.id), '{}')`
