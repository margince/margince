// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture_test

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestNativeBotRefreshRetiresOnlyTheSupersededCredential(t *testing.T) {
	f := newChannelFixture(t, nil)
	f.withPushProvider()
	connection := f.connectPush(t, pushSecret)
	prior := f.credentialRefOf(t, connection.ID)
	events := f.eventsRefOf(t, connection.ID)
	_, pool := setupCaptureDB(t)
	registry := capture.NewRegistry(database.BindTo(pool, f.workspaceKey()), nil, nil, f.vault)
	ctx := principal.WithActor(f.ctx, principal.Principal{Type: principal.PrincipalSystem, ID: "system:chat_links", Permissions: principal.Permissions{RowScope: principal.RowScopeAll}})
	bot, err := registry.LiveBotUsing(ctx, pushProvider, func(_ context.Context, old []byte) ([]byte, error) {
		if string(old) != pushToken {
			t.Fatalf("wrong refresh source %q", old)
		}
		return []byte("gwapp_refreshed_fixture"), nil
	})
	if err != nil || bot.Token != "gwapp_refreshed_fixture" || bot.ConnectionID != connection.ID || bot.TeamID != pushTeam {
		t.Fatalf("refresh changed installation identity or lost credential: %+v %v", bot, err)
	}
	current := f.credentialRefOf(t, connection.ID)
	if current == prior {
		t.Fatal("rotated token kept the prior vault reference")
	}
	if _, exists := f.sealed(t, prior); exists {
		t.Fatal("superseded token remained in the vault")
	}
	if secret, exists := f.sealed(t, current); !exists || secret != bot.Token {
		t.Fatalf("new credential missing: %q %v", secret, exists)
	}
	if secret, exists := f.sealed(t, events); !exists || secret != pushSecret || f.eventsRefOf(t, connection.ID) != events {
		t.Fatal("token refresh destroyed or replaced the signing secret")
	}
	if _, err := registry.LiveBotUsing(f.ctx, pushProvider, func(context.Context, []byte) ([]byte, error) {
		t.Fatal("ordinary request reached system credential refresh")
		return nil, nil
	}); err == nil {
		t.Fatal("ordinary request could rotate the workspace bot credential")
	}
	want := errors.New("provider unavailable")
	if _, err := registry.LiveBotUsing(ctx, pushProvider, func(context.Context, []byte) ([]byte, error) { return nil, want }); !errors.Is(err, want) {
		t.Fatalf("refresh hid provider failure: %v", err)
	}
	if f.credentialRefOf(t, connection.ID) != current {
		t.Fatal("failed refresh replaced a working credential")
	}
	unchanged, err := registry.LiveBotUsing(ctx, pushProvider, func(_ context.Context, old []byte) ([]byte, error) { return old, nil })
	if err != nil || unchanged.Token != bot.Token || f.credentialRefOf(t, connection.ID) != current {
		t.Fatalf("fresh credential was unnecessarily replaced: %+v %v", unchanged, err)
	}
}
