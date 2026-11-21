// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// A captured mail and its participants clear only the stamps at or after the
// mail's own time. The older settled mail on its thread keeps its stamps.
func TestACapturedMailKeepsTheOlderSettledStampsOfItsThread(t *testing.T) {
	owner, pool := setupCaptureDB(t)
	ctx := context.Background()
	ws := ids.NewV7()
	if _, err := owner.Exec(ctx, `INSERT INTO workspace (id) VALUES ($1)`, ws); err != nil {
		t.Fatalf("seeding workspace: %v", err)
	}
	sink := capture.NewSink(database.BindTo(pool, ids.From[ids.WorkspaceKind](ws)))
	sinkCtx := captureSinkContextFor(ctx, ws)
	thread := "gmail:settled-" + ids.NewV7().String()
	start := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)

	const older = 3
	for i := range older {
		if _, err := sink.Upsert(sinkCtx, threadMail(thread, i, start.Add(time.Duration(i)*time.Hour))); err != nil {
			t.Fatalf("capturing older mail %d: %v", i, err)
		}
	}
	if _, err := owner.Exec(ctx, `SELECT keep_reply_settled(array_agg(id), array_agg(occurred_at + interval '30 minutes'))
		FROM activity WHERE thread_key = $1`, thread); err != nil {
		t.Fatalf("stamping the older mail: %v", err)
	}

	if _, err := sink.Upsert(sinkCtx, threadMail(thread, older, start.Add(older*time.Hour))); err != nil {
		t.Fatalf("capturing the new mail: %v", err)
	}
	var participants, unstamped int
	if err := owner.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM activity_participant p JOIN activity a ON a.id = p.activity_id WHERE a.thread_key = $1 AND a.source_id = $2),
		(SELECT count(*) FROM activity WHERE thread_key = $1 AND reply_settled_at IS NULL)`,
		thread, "settled-3").Scan(&participants, &unstamped); err != nil {
		t.Fatalf("reading the thread: %v", err)
	}
	if participants < 2 {
		t.Fatalf("the new mail was captured with %d participant row(s); the case needs its recipients written", participants)
	}
	if unstamped != 1 {
		t.Errorf("%d mails on the thread are unstamped after one new mail, want only the new one", unstamped)
	}
}

// threadMail is inbound mail number n on thread, from one buyer to two of us.
func threadMail(thread string, n int, at time.Time) connector.NormalizedRecord {
	sourceID := fmt.Sprintf("settled-%d", n)
	return connector.NormalizedRecord{
		EntityType: datasource.EntityActivity,
		NaturalKey: connector.NaturalKey{SourceSystem: connector.EmailSourceSystem, SourceID: sourceID},
		Fields: capture.ActivityFields{
			Kind: "email", Subject: "the order", Body: "any news?",
			Direction: connector.DirectionInbound, OccurredAt: at,
		},
		Source:     "gmail:" + sourceID,
		CapturedBy: "connector:gmail",
		Counterparty: connector.Counterparty{
			Direction: connector.DirectionInbound,
			Email:     "buyer@settled.test",
		},
		Participants: []connector.MessageParticipant{
			{Email: "rep@ours.test", Role: "to"},
			{Email: "colleague@ours.test", Role: "cc"},
		},
		ThreadKey: thread,
	}
}
