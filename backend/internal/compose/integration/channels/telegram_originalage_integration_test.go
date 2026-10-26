// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package channels

// The provider original's own retention stage, over a channel message.
//
// Only activity.raw_capture_id joins a channel original to its activity. These
// cases run the real poll, ingest and retention pass. They sit here because
// this package arms the statutory floor (statutoryfloor_test.go).

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

func TestAChannelOriginalAgesOutOnItsOwnClock(t *testing.T) {
	c := setupTelegramConnected(t)
	u := telegramUpdate{updateID: 6101, messageID: 61, senderID: 776101, username: "aged", firstName: "Ada", text: "an old message"}
	c.ingestOne(t, u, compose.JobRunnerConfig{})

	c.ageTheOriginalAndSweep(t, u, nil)

	if n := c.rawCaptures(t, u.updateID); n != 0 {
		t.Fatalf("%d channel original(s) outlived the raw_capture policy — the stage joined on a key the poller never writes", n)
	}
	if n := c.telegramActivities(t, u); n != 1 {
		t.Fatalf("%d activities left, want the message itself: the stage governs the stored original, not the record", n)
	}
}

func TestAShieldedChannelMessageKeepsItsOriginal(t *testing.T) {
	c := setupTelegramConnected(t)
	u := telegramUpdate{updateID: 6102, messageID: 62, senderID: 776102, username: "held", firstName: "Hal", text: "the signed order"}
	c.ingestOne(t, u, compose.JobRunnerConfig{})
	commercial := "commercial_correspondence"

	c.ageTheOriginalAndSweep(t, u, &commercial)

	if n := c.rawCaptures(t, u.updateID); n != 1 {
		t.Fatalf("%d original(s) left, want the shielded message's one — §257 HGB obliges keeping the correspondence, "+
			"and the original is part of it", n)
	}
}

// ageTheOriginalAndSweep moves the message and its original behind a 100-day
// raw_capture rule, stamps the class when given, and runs the real pass.
func (c *telegramEnv) ageTheOriginalAndSweep(t *testing.T, u telegramUpdate, class *string) {
	t.Helper()
	if err := apptest.InWorkspace(c.AppEnv, t, func(tx pgx.Tx) error {
		ctx := context.Background()
		rule, err := tx.Exec(ctx, `
			UPDATE retention_policy SET retain_days = 100, enabled = true
			 WHERE object_type = 'raw_capture' AND category IS NULL AND action = 'erase'`)
		if err != nil {
			return err
		}
		if rule.RowsAffected() != 1 {
			return fmt.Errorf("the installation seeded %d raw_capture erase rule(s), want 1", rule.RowsAffected())
		}
		tag, err := tx.Exec(ctx, `
			UPDATE raw_capture rc SET received_at = now() - interval '400 days'
			  FROM activity a
			 WHERE a.raw_capture_id = rc.id AND a.source_system = 'telegram' AND a.source_id = $1`, u.naturalKey())
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return fmt.Errorf("aged %d linked original(s), want the one the activity names", tag.RowsAffected())
		}
		_, err = tx.Exec(ctx, `
			UPDATE activity SET occurred_at = now() - interval '400 days',
			       retention_class = $2,
			       retention_class_at = CASE WHEN $2::text IS NULL THEN NULL ELSE now() END
			 WHERE source_system = 'telegram' AND source_id = $1`, u.naturalKey(), class)
		return err
	}); err != nil {
		t.Fatalf("ageing the original out: %v", err)
	}
	svc := compose.NewRetentionServiceFor(c.DB(), nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := svc.EvaluateInstallation(integration.RetentionPassCtx(c.workspaceID(t))); err != nil {
		t.Fatalf("running the retention pass: %v", err)
	}
}
