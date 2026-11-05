// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/roombrief"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/dealrooms"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type roomBriefHandlers = roombrief.Handlers

// roomBriefReads is the room's own gated reads, as the brief takes them: the
// rosters whole, without the page the list endpoints carry.
type roomBriefReads struct{ store *dealrooms.Store }

func (r roomBriefReads) GetRoom(ctx context.Context, id ids.DealRoomID) (crmcontracts.DealRoom, error) {
	return r.store.GetRoom(ctx, id)
}

func (r roomBriefReads) ListParticipants(
	ctx context.Context, roomID ids.DealRoomID, activeOnly bool,
) ([]crmcontracts.DealRoomParticipant, error) {
	rows, _, err := r.store.ListParticipants(ctx, roomID, activeOnly)
	return rows, err
}

func (r roomBriefReads) ListDocuments(ctx context.Context, roomID ids.DealRoomID) ([]crmcontracts.DealRoomDocument, error) {
	rows, _, err := r.store.ListDocuments(ctx, roomID)
	return rows, err
}

func (r roomBriefReads) ListThreads(
	ctx context.Context, roomID ids.DealRoomID, documentID *ids.UUID,
) ([]crmcontracts.DealRoomThread, error) {
	return r.store.ListThreads(ctx, roomID, documentID)
}

// newRoomBriefService builds the room brief over the room's own reads.
func newRoomBriefService(pool *pgxpool.Pool) *roombrief.Service {
	reads := roomBriefReads{store: dealrooms.NewStore(InstallationDB(pool))}
	return roombrief.NewService(pool, reads, time.Now)
}

// WithDealRoomBriefWriter binds the summarize lane that writes the room brief.
// No routing version: the brief is stored nowhere, so no text can outlive a
// re-pointed lane.
func WithDealRoomBriefWriter(brain completer) Option {
	return func(s *Server, _ *pgxpool.Pool) {
		if s.roomBriefSvc == nil {
			return
		}
		s.roomBriefHandlers = roombrief.NewHandlers(s.roomBriefSvc.WithLane(brain))
	}
}
