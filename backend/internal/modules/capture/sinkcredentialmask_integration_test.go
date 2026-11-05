// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture_test

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// The sink masks what every connector stores, not only what the mail mapper
// built: a chat message lands with its PIN masked too.
func TestTheSinkMasksCredentialsInEveryCapturedRecord(t *testing.T) {
	owner, pool := setupCaptureDB(t)
	ctx := context.Background()
	ws := ids.NewV7()
	if _, err := owner.Exec(ctx, `INSERT INTO workspace (id) VALUES ($1)`, ws); err != nil {
		t.Fatalf("seeding workspace: %v", err)
	}
	sink := capture.NewSink(database.BindTo(pool, ids.From[ids.WorkspaceKind](ws))).
		WithChannelEnsurer(&recordingChannelEnsurer{})

	rec := connector.NormalizedRecord{
		EntityType: datasource.EntityActivity,
		NaturalKey: connector.NaturalKey{SourceSystem: "telegram", SourceID: "8100:4242:9"},
		Fields: capture.ActivityFields{
			Kind: "message", ChannelProvider: "telegram", Direction: connector.DirectionInbound,
			Body:       "the door code, PIN: 4711",
			OccurredAt: time.Date(2026, 7, 30, 9, 0, 0, 0, time.UTC),
		},
		Source:     "telegram:8100:4242:9",
		CapturedBy: "connector:telegram",
		Counterparty: connector.Counterparty{
			Direction:       connector.DirectionInbound,
			ChannelIdentity: connector.ChannelIdentity{Provider: "telegram", ChannelUserID: "990102"},
		},
		ThreadKey: "telegram:8100:4242",
	}
	ref, err := sink.Upsert(channelSinkContext(ctx, ws, "connector:telegram"), rec)
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	var body string
	if err := owner.QueryRow(ctx, `SELECT body FROM activity WHERE id = $1`, ref.ID).Scan(&body); err != nil {
		t.Fatalf("reading the captured message: %v", err)
	}
	if body != "the door code, PIN: [masked]" {
		t.Errorf("body = %q, want the PIN masked behind its label", body)
	}
}
