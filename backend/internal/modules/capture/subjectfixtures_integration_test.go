// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture_test

import (
	"context"
	"time"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// aProjectSubjectRecord is one inbound email whose subject carries a bracketed
// key. The same record every time, which is what a mailbox replay delivers.
func aProjectSubjectRecord() connector.NormalizedRecord {
	return connector.NormalizedRecord{
		EntityType: datasource.EntityActivity,
		NaturalKey: connector.NaturalKey{SourceSystem: connector.EmailSourceSystem, SourceID: "retry-1"},
		Fields: capture.ActivityFields{
			Kind: "email", Subject: "[retry] the rollout plan", Body: "as discussed",
			Direction:  connector.DirectionInbound,
			OccurredAt: time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC),
		},
		Source:     "gmail:retry-1",
		CapturedBy: "connector:gmail",
		Counterparty: connector.Counterparty{
			Direction: connector.DirectionInbound,
			Email:     "sender@retry.test",
		},
		ThreadKey: "gmail:retry-thread",
	}
}

// captureSinkContextFor is the connector principal a mail sync captures under.
func captureSinkContextFor(ctx context.Context, ws ids.UUID) context.Context {
	ctx = principal.WithWorkspaceID(ctx, ws)
	ctx = principal.WithActor(ctx, principal.Principal{
		Type:     principal.PrincipalConnector,
		ID:       "connector:gmail",
		UserID:   ids.NewV7(),
		SeatType: principal.SeatFull,
		Permissions: principal.Permissions{
			RoleKeys: []string{"connector"},
			Objects: map[string]principal.ObjectGrant{
				"activity": {Create: true, Read: true, Update: true},
				"contact":  {Create: true, Read: true},
				"project":  {Read: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
	return principal.WithCorrelationID(ctx, ids.NewV7())
}
