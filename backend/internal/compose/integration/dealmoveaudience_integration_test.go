// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func dealReaderWithoutActivities(e *SearchEnv, userID ids.UUID) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + ids.NewV7().String(), UserID: userID,
		SeatType: principal.SeatFull,
		Permissions: principal.Permissions{
			Objects:  map[string]principal.ObjectGrant{"deal": {Read: true}},
			RowScope: principal.RowScopeAll,
		},
	})
}

func dealReader(e *SearchEnv) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + ids.NewV7().String(), UserID: ids.NewV7(),
		SeatType: principal.SeatFull,
		Permissions: principal.Permissions{
			Objects: map[string]principal.ObjectGrant{
				"deal":     {Read: true},
				"activity": {Read: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
}

func readerOf(ctx context.Context, t *testing.T) ids.UUID {
	t.Helper()
	actor, ok := principal.Actor(ctx)
	if !ok {
		t.Fatal("the reader context carries no principal")
	}
	return actor.UserID
}
