// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package chatsync

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// fileOnDeal files an activity on the deal through the automatic filing write
// and returns the relink envelope that write put in the outbox.
func (e *flushEnv) fileOnDeal(t *testing.T, activity ids.UUID) events.Envelope {
	t.Helper()
	ctx := principal.WithCorrelationID(context.Background(), ids.NewV7())
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalSystem, ID: "system:test",
		Permissions: principal.Permissions{RowScope: principal.RowScopeAll},
	})
	err := database.WithTx(ctx, e.pool, func(tx pgx.Tx) error {
		_, err := activities.FileOnDealIfUnfiledTx(ctx, tx, ids.From[ids.ActivityKind](activity), e.deal, nil, "")
		return err
	})
	if err != nil {
		t.Fatalf("filing the activity on the deal: %v", err)
	}
	var raw []byte
	if err := e.owner.QueryRow(context.Background(), `SELECT envelope FROM event_outbox
		WHERE envelope->>'type' = 'activity.updated' ORDER BY seq DESC LIMIT 1`).Scan(&raw); err != nil {
		t.Fatalf("reading the relink event: %v", err)
	}
	var env events.Envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("decoding the relink event: %v", err)
	}
	return env
}

func filedActivityPosts(t *testing.T, e *flushEnv) int {
	t.Helper()
	e.flush(t)
	posted := 0
	for _, text := range postTexts(e.chat) {
		posted += strings.Count(text, "Filed later")
	}
	return posted
}

func seedLateNote(t *testing.T, e *flushEnv) ids.UUID {
	t.Helper()
	late := ids.NewV7()
	e.exec(t, `INSERT INTO activity (id, kind, subject, audience, source, captured_by)
		VALUES ($1, 'note', 'Filed later', 'workspace', 'manual', 'human:x')`, late)
	return late
}

func TestAnActivityFiledAfterItsCaptureReachesTheChannelOnce(t *testing.T) {
	e := setupFlush(t)
	late := seedLateNote(t, e)
	e.handle(t, "activity.captured", "activity", late, `{"kind":"note"}`)
	relink := e.fileOnDeal(t, late)
	for range 2 {
		if err := e.sync.HandleEvent(context.Background(), relink); err != nil {
			t.Fatalf("handling the relink event: %v", err)
		}
	}
	if posted := filedActivityPosts(t, e); posted != 1 {
		t.Fatalf("the filed activity appears %d times in the channel, want once: %q", posted, postTexts(e.chat))
	}
}

func TestAnActivityCapturedAfterItsFilingIsNotPostedTwice(t *testing.T) {
	e := setupFlush(t)
	late := seedLateNote(t, e)
	relink := e.fileOnDeal(t, late)
	if err := e.sync.HandleEvent(context.Background(), relink); err != nil {
		t.Fatalf("handling the relink event: %v", err)
	}
	e.handle(t, "activity.captured", "activity", late, `{"kind":"note"}`)
	if posted := filedActivityPosts(t, e); posted != 1 {
		t.Fatalf("the activity appears %d times in the channel, want once: %q", posted, postTexts(e.chat))
	}
}
