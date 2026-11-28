// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package chatsync

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/chatlinks"
	"github.com/margince/margince/backend/internal/modules/chatlinks/teams"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestDisconnectedWatchCleanupRetainsLostResponseLeaseAndDestroysExpiredSecret(t *testing.T) {
	e := setupFlush(t)
	var connection ids.UUID
	if err := e.owner.QueryRow(context.Background(), `SELECT connection_id FROM chat_link WHERE id=$1`, e.link).Scan(&connection); err != nil {
		t.Fatal(err)
	}
	e.exec(t, `UPDATE channel_connection SET provider='teams',channel_id='tenant/team',address=NULL WHERE id=$1`, connection)
	e.exec(t, `UPDATE chat_link SET provider='teams',channel_slug='team/channel' WHERE id=$1`, e.link)
	ctx := principal.WithActor(context.Background(), chatLinksPrincipal)
	vault := keyvault.NewMemory()
	e.store.WithVault(func() keyvault.Vault { return vault })
	links, err := e.store.PullLinks(ctx, connection)
	if err != nil || len(links) != 1 {
		t.Fatalf("links=%v error=%v", links, err)
	}
	now := time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)
	watch, err := e.store.PrepareChannelWatch(ctx, links[0], "app", "tenant", "https://crm.example.test/v1/channel-events/teams/graph", now)
	if err != nil {
		t.Fatal(err)
	}
	until := now.Add(teams.WatchMaxLifetime)
	attempted, err := e.store.BeginChannelWatchAttempt(ctx, watch, until)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.store.CompleteChannelWatch(ctx, attempted, "", time.Time{}, "source_incomplete"); err != nil {
		t.Fatal(err)
	}
	e.exec(t, `UPDATE channel_connection SET status='disconnected',archived_at=now() WHERE id=$1`, connection)
	cleanup, err := chatlinks.DueChannelWatchCleanup(context.Background(), e.pool)
	if err != nil || len(cleanup) != 1 || cleanup[0].Connection != connection || cleanup[0].Workspace != e.ws {
		t.Fatalf("disconnected watch disappeared from cleanup: %v %v", cleanup, err)
	}
	unused, err := e.store.UnusedChannelWatches(ctx, connection)
	if err != nil || len(unused) != 1 {
		t.Fatalf("disconnected link still uses watch: %v %v", unused, err)
	}
	if unused[0].LeaseUntil == nil || !unused[0].LeaseUntil.Equal(until) {
		t.Fatal("lost response discarded lease bound")
	}
	if err := e.store.RetireChannelWatch(ctx, unused[0], until.Add(-time.Second), false); !errors.Is(err, apperrors.ErrInvalidArgument) {
		t.Fatalf("retired an uncertain live subscription: %v", err)
	}
	if _, err := e.store.ChannelWatchSecret(ctx, unused[0]); err != nil {
		t.Fatalf("destroyed live lease secret: %v", err)
	}
	if err := e.store.RetireChannelWatch(ctx, unused[0], until, false); err != nil {
		t.Fatal(err)
	}
	if _, err := e.store.ChannelWatchSecret(ctx, unused[0]); err == nil {
		t.Fatal("expired credential retained")
	}
	var retired, verified bool
	if err := e.owner.QueryRow(context.Background(), `SELECT retired_at IS NOT NULL,removal_verified FROM chat_channel_watch WHERE id=$1`, watch.ID).Scan(&retired, &verified); err != nil || !retired || verified {
		t.Fatalf("expiry claimed confirmed removal: retired=%v verified=%v error=%v", retired, verified, err)
	}
	cleanup, err = chatlinks.DueChannelWatchCleanup(context.Background(), e.pool)
	if err != nil || len(cleanup) != 0 {
		t.Fatalf("retired watch rescheduled: %v %v", cleanup, err)
	}
}
