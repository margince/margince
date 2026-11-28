// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package chatsync

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/chatlinks/chatwire"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestCompletedForwardScanRetainsItsSequenceAcrossQuietPolls(t *testing.T) {
	e := setupFlush(t)
	var connection ids.UUID
	if err := e.owner.QueryRow(context.Background(), `SELECT connection_id FROM chat_link WHERE id=$1`, e.link).Scan(&connection); err != nil {
		t.Fatal(err)
	}
	ctx := principal.WithCorrelationID(principal.WithActor(context.Background(), chatLinksPrincipal), ids.NewV7())
	links, err := e.store.PullLinks(ctx, connection)
	if err != nil || len(links) != 1 {
		t.Fatalf("links: %+v %v", links, err)
	}
	link := links[0]
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	first, err := e.store.StartRootScan(ctx, link, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.store.RecordRootPage(ctx, link, first, chatwire.RootPage{Roots: []chatwire.RootRef{{ID: "root-one", Revision: "first"}}, Next: "42"}); err != nil {
		t.Fatal(err)
	}
	second, err := e.store.StartRootScan(ctx, link, now.Add(time.Minute))
	if err != nil || second.Cursor != "42" {
		t.Fatalf("lost page cursor: %+v %v", second, err)
	}
	if err := e.store.RecordRootPage(ctx, link, second, chatwire.RootPage{Checkpoint: "42"}); err != nil {
		t.Fatal(err)
	}
	third, err := e.store.StartRootScan(ctx, link, now.Add(time.Hour))
	if err != nil || third.Cursor != "42" || third.Coverage == nil || !third.Coverage.Equal(now) {
		t.Fatalf("lost committed coverage or sequence: %+v %v", third, err)
	}
	if err := e.store.RecordRootPage(ctx, link, third, chatwire.RootPage{Checkpoint: "42"}); err != nil {
		t.Fatalf("a quiet poll stopped root refresh: %v", err)
	}
	roots, err := e.store.RootsToRefresh(ctx, link, now.Add(2*time.Hour))
	if err != nil || len(roots) != 1 || roots[0].Root != "root-one" {
		t.Fatalf("quiet scan lost the watched root: %+v %v", roots, err)
	}
}
