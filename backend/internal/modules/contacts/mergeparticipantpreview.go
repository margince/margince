// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

const mergeParticipantTwin = `SELECT 1 FROM activity_participant twin
		        WHERE twin.activity_id = ap.activity_id
		          AND twin.role = ap.role
		          AND twin.contact_id = @target
		          AND twin.user_id IS NOT DISTINCT FROM ap.user_id
		          AND twin.address IS NOT DISTINCT FROM ap.address
		          AND twin.channel_user_id IS NOT DISTINCT FROM ap.channel_user_id`

type MergeParticipant struct {
	ID            ids.UUID
	ActivityID    ids.UUID
	Role          string
	UserID        *ids.UUID
	Address       *string
	ChannelUserID *string
	Collapses     bool
}

type ParticipantRosterReader func(context.Context, pgx.Tx, ids.ActivityID, bool) error

func (s *Store) WithParticipantRosterReader(read ParticipantRosterReader) *Store {
	s.participantRosterReader = read
	return s
}

func (s *Store) previewMergeParticipants(ctx context.Context, tx pgx.Tx, source, target ids.ContactID) ([]MergeParticipant, error) {
	// Admission must not reveal whether the source has conversation participants.
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT ap.id, ap.activity_id, ap.role, ap.user_id, ap.address,
 ap.channel_user_id, EXISTS (`+mergeParticipantTwin+`)
 FROM activity_participant ap WHERE ap.contact_id = @source ORDER BY ap.activity_id, ap.id LIMIT 501`, pgx.StrictNamedArgs{"source": source, "target": target})
	if err != nil {
		return nil, err
	}
	participants, err := pgx.CollectRows(rows, pgx.RowToStructByPos[MergeParticipant])
	if err != nil {
		return nil, err
	}
	if len(participants) > 500 {
		return nil, apperrors.ErrInvalidArgument
	}
	if len(participants) == 0 {
		return participants, nil
	}
	if s.participantRosterReader == nil {
		return nil, apperrors.ErrInvalidArgument
	}
	rosters := map[ids.UUID]bool{}
	for _, participant := range participants {
		rosters[participant.ActivityID] = rosters[participant.ActivityID] || participant.Role == connector.ParticipantRoleBCC
	}
	for activity, hasBCC := range rosters {
		if err := s.participantRosterReader(ctx, tx, ids.From[ids.ActivityKind](activity), hasBCC); err != nil {
			return nil, err
		}
	}
	return participants, nil
}

func (h Handlers) WithParticipantRosterReader(read ParticipantRosterReader) Handlers {
	h.store = h.store.WithParticipantRosterReader(read)
	return h
}
