// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package chatlinks

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestConcurrentMemberReadsShareOneRotatingRefreshWithoutChangingAudienceVersion(t *testing.T) {
	e := setupSlackMemberLinks(t)
	vault := keyvault.NewMemory()
	e.store.WithMessenger(func() keyvault.Vault { return vault }, func(context.Context, MemberGrant, []byte) (messaging.Client, error) {
		return identityClient{identity: messaging.Identity{Workspace: "TEAM", Member: messaging.Member{ID: "REP", Active: true}}}, nil
	})
	ctx := e.seat(e.rep)
	grant, err := e.store.ConnectMember(ctx, MemberGrant{Provider: "slack", Workspace: "TEAM", Member: "REP"}, []byte("old-token"))
	if err != nil {
		t.Fatal(err)
	}
	var refreshes atomic.Int64
	e.store.WithMemberRefresh(func(_ context.Context, _ MemberGrant, raw []byte) ([]byte, error) {
		if string(raw) == "old-token" {
			refreshes.Add(1)
			return []byte("new-token"), nil
		}
		return raw, nil
	})
	answers := make(chan error, 2)
	for range 2 {
		go func() { _, _, err := e.store.OpenMember(ctx, "slack"); answers <- err }()
	}
	for range 2 {
		if err := <-answers; err != nil {
			t.Fatal(err)
		}
	}
	if refreshes.Load() != 1 {
		t.Fatalf("rotating token spent %d times", refreshes.Load())
	}
	current, err := e.store.MemberGrant(ctx, "slack")
	if err != nil || current.Version != grant.Version || current.credential == grant.credential {
		t.Fatalf("refresh invalidated reviewed conversation or kept old secret: %v", err)
	}
	ws := ids.From[ids.WorkspaceKind](e.ws)
	if _, err := vault.Get(ctx, ws, keyvault.Ref(grant.credential)); !errors.Is(err, keyvault.ErrNotFound) {
		t.Fatalf("superseded refresh token survived: %v", err)
	}
	raw, err := vault.Get(ctx, ws, keyvault.Ref(current.credential))
	if err != nil || string(raw) != "new-token" {
		t.Fatal("rotated credential was not committed")
	}
}
