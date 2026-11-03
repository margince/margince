// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// organizerSeatSQL picks, per activity among the ids at the placeholder, the
// seat its organizer participant is bound to. Capture binds an organizer's
// address to a seat only when that seat alone proved it, and a meeting two
// calendars disagree on names nobody.
const organizerSeatSQL = `SELECT op.activity_id, (array_agg(DISTINCT op.user_id))[1] AS seat
	  FROM activity_participant op
	 WHERE op.activity_id = ANY($%d) AND op.role = 'organizer' AND op.user_id IS NOT NULL
	 GROUP BY op.activity_id
	HAVING count(DISTINCT op.user_id) = 1`

// meetingOrganizerQuery is that seat for each live meeting the caller may read.
const meetingOrganizerQuery = `
	SELECT o.activity_id, o.seat FROM (` + organizerSeatSQL + `) o
	  JOIN activity a ON a.id = o.activity_id AND a.kind = 'meeting' AND ` + activityLive + ` AND %s`

// MeetingOrganizers answers which colleague organized each of these meetings,
// for this caller. Outside organizers and unreadable meetings are absent. It
// can differ from host_user_id, which names whose calendar synced the meeting
// first, unless ClaimOrganizerHostTx moved it to the organizer.
func (s *Store) MeetingOrganizers(ctx context.Context, meetingIDs []ids.UUID) (map[ids.UUID]ids.UUID, error) {
	out := make(map[ids.UUID]ids.UUID, len(meetingIDs))
	if len(meetingIDs) == 0 {
		return out, nil
	}
	if err := auth.Require(ctx, linkEntityActivity, principal.ActionRead); err != nil {
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			return out, nil
		}
		return nil, err
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	activityScope, err := auth.ActivityContentClause(ctx, "a", arg)
	if err != nil {
		return nil, err
	}
	meetings := arg(meetingIDs)
	query := storekit.SQLf(meetingOrganizerQuery, meetings, orUnbounded(activityScope))
	err = s.tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return fmt.Errorf("activities: reading who organized the meetings: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var meeting, organizer ids.UUID
			if err := rows.Scan(&meeting, &organizer); err != nil {
				return fmt.Errorf("activities: reading who organized a meeting: %w", err)
			}
			out[meeting] = organizer
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// organizerHostReason is the audit reason a host moved to its organizer records.
const organizerHostReason = "meeting_organizer_host"

// organizerHostQuery is the organizer seat a calendar meeting should be hosted
// by, beside the host it has. A row qualifies only when a sync created it and
// the app did not book it. On a logged or booked meeting the host is evidence
// of who held it. The organizer's own calendar must hold the meeting too.
const organizerHostQuery = `
	SELECT a.host_user_id, o.seat FROM (` + organizerSeatSQL + `) o
	  JOIN activity a ON a.id = o.activity_id
	 WHERE a.kind = 'meeting' AND ` + activityLive + ` AND a.captured_by LIKE 'connector:%%'
	   AND NOT EXISTS (SELECT 1 FROM meeting_invitation mi WHERE mi.activity_id = a.id)
	   AND EXISTS (SELECT 1 FROM capture_import ci WHERE ci.activity_id = a.id AND ci.user_id = o.seat)
	   AND a.host_user_id IS DISTINCT FROM o.seat`

// ClaimOrganizerHostTx names a calendar meeting's organizer as its host, when
// the organizer is a colleague whose own calendar holds it. It answers whether
// the host moved. Otherwise the host is whose calendar synced the meeting
// first, often an invited colleague's, and the Worklist names them as owner.
// Credit on a synced row reads attendance, not the host, so no credit moves.
func (s *Store) ClaimOrganizerHostTx(ctx context.Context, tx pgx.Tx, id ids.ActivityID) (bool, error) {
	if err := requireFoldActor(ctx, "names a meeting's organizer as its host"); err != nil {
		return false, err
	}
	lock, err := storekit.LockRow(ctx, tx, "activity", id.UUID, storekit.LiveOnly)
	if errors.Is(err, apperrors.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("activities: locking a meeting to name its host: %w", err)
	}
	var host *ids.UUID
	var organizer ids.UUID
	err = tx.QueryRow(ctx, storekit.SQLf(organizerHostQuery, 1), []ids.UUID{id.UUID}).Scan(&host, &organizer)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("activities: reading who organized a meeting: %w", err)
	}
	p := storekit.NewPatch()
	p.Set("host_user_id", host, organizer)
	if err := p.ApplyLocked(ctx, tx, lock); err != nil {
		return false, fmt.Errorf("activities: naming a meeting's organizer as its host: %w", err)
	}
	evidence := map[string]any{"reason": organizerHostReason}
	auditID, err := storekit.AuditWithEvidence(ctx, tx, "update", "activity", id.UUID, p.Before(), p.After(), evidence)
	if err != nil {
		return false, err
	}
	hostID := openapi_types.UUID(organizer)
	return true, storekit.EmitEvent(ctx, tx, auditID, id.UUID, crmcontracts.PublicEventActivityUpdated{
		ChangedFields: crmcontracts.PublicEventActivityChangedFields{HostUserId: &hostID},
	})
}
