// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package roombrief

import (
	"context"
	"errors"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// countingRooms answers GetRoom with roomErr and counts every list read.
type countingRooms struct {
	roomErr error
	reads   int
}

func (r *countingRooms) GetRoom(context.Context, ids.DealRoomID) (crmcontracts.DealRoom, error) {
	r.reads++
	return crmcontracts.DealRoom{}, r.roomErr
}

func (r *countingRooms) ListParticipants(context.Context, ids.DealRoomID, bool) ([]crmcontracts.DealRoomParticipant, error) {
	r.reads++
	return nil, nil
}

func (r *countingRooms) ListDocuments(context.Context, ids.DealRoomID) ([]crmcontracts.DealRoomDocument, error) {
	r.reads++
	return nil, nil
}

func (r *countingRooms) ListThreads(context.Context, ids.DealRoomID, *ids.UUID) ([]crmcontracts.DealRoomThread, error) {
	r.reads++
	return nil, nil
}

func actor(kind principal.PrincipalType) context.Context {
	return principal.WithActor(context.Background(), principal.Principal{
		Type: kind, UserID: ids.NewV7(),
		Permissions: principal.Permissions{RowScope: principal.RowScopeAll},
	})
}

func TestAnAgentIsRefusedBeforeTheRoomIsRead(t *testing.T) {
	rooms := &countingRooms{}
	_, err := NewService(nil, rooms, func() time.Time { return clocktest.Now(t) }).Get(actor(principal.PrincipalAgent), ids.From[ids.DealRoomKind](ids.NewV7()))
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("an agent's read = %v, want permission denied", err)
	}
	if rooms.reads != 0 {
		t.Errorf("an agent's read reached %d room reads, want none", rooms.reads)
	}
}

func TestARoomTheCallerCannotSeeIsNotReadFurther(t *testing.T) {
	rooms := &countingRooms{roomErr: apperrors.ErrNotFound}
	_, err := NewService(nil, rooms, func() time.Time { return clocktest.Now(t) }).Get(actor(principal.PrincipalHuman), ids.From[ids.DealRoomKind](ids.NewV7()))
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("an unseen room = %v, want not found", err)
	}
	if rooms.reads != 1 {
		t.Errorf("an unseen room made %d reads, want the room read alone", rooms.reads)
	}
}
