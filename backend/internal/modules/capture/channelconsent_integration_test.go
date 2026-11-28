// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type consentBotBoundary struct{ workspace string }

func (consentBotBoundary) NeedsAddress() bool         { return false }
func (consentBotBoundary) FixedIdentity() bool        { return true }
func (consentBotBoundary) ValidateToken(string) error { return nil }
func (c consentBotBoundary) Confirm(context.Context, string, string) (capture.ChannelBot, error) {
	return capture.ChannelBot{ID: c.workspace, Label: "Synthetic workspace"}, nil
}

func TestNativeChannelConsentReconnectsOnlyTheSameWorkspace(t *testing.T) {
	for _, provider := range []string{"slack", "teams"} {
		t.Run(provider, func(t *testing.T) {
			f := newChannelFixture(t, nil)
			store := f.store.WithBotCheck(provider, consentBotBoundary{"workspace-one"})
			handler := capture.NewChannelHandlers(store)
			first, err := handler.ConnectBot(f.ctx, capture.ConnectRequest{Provider: provider, BotToken: "first-synthetic"})
			if err != nil {
				t.Fatal(err)
			}
			ref := f.credentialRefOf(t, first.ID)
			second, err := handler.ConnectBot(f.ctx, capture.ConnectRequest{Provider: provider, BotToken: "renewed-synthetic"})
			if err != nil {
				t.Fatal(err)
			}
			if second.ID != first.ID || second.Version <= first.Version || f.rowCount(t) != 1 {
				t.Fatalf("reconsent replaced the connection: %#v %#v", first, second)
			}
			if _, err := f.vault.Get(f.ctx, f.workspaceKey(), ref); err == nil {
				t.Fatal("superseded token retained")
			}
			foreign := capture.NewChannelHandlers(store.WithBotCheck(provider, consentBotBoundary{"workspace-two"}))
			if _, err := foreign.ConnectBot(f.ctx, capture.ConnectRequest{Provider: provider, BotToken: "foreign-synthetic"}); !errors.Is(err, apperrors.ErrConflict) {
				t.Fatalf("foreign consent retargeted links: %v", err)
			}
			if live := f.liveConnections(t); len(live) != 1 || live[0].ChannelID != "workspace-one" {
				t.Fatalf("existing destination changed: %#v", live)
			}
		})
	}
}

func TestChannelConsentRechecksAuthorityBeforeCommitting(t *testing.T) {
	for _, reconnect := range []bool{false, true} {
		t.Run(map[bool]string{false: "install", true: "reconnect"}[reconnect], func(t *testing.T) {
			f := newChannelFixture(t, nil)
			store := f.store.WithBotCheck("slack", consentBotBoundary{"workspace-one"})
			handler := capture.NewChannelHandlers(store)
			var prior capture.ChannelConnection
			if reconnect {
				var err error
				prior, err = handler.ConnectBot(f.ctx, capture.ConnectRequest{Provider: "slack", BotToken: "existing-synthetic"})
				if err != nil {
					t.Fatal(err)
				}
			}
			narrowed := store.WithAuthority(func(ctx context.Context, _ pgx.Tx) (context.Context, error) {
				actor, _ := principal.Actor(ctx)
				actor.Permissions.Objects = map[string]principal.ObjectGrant{}
				return principal.WithActor(ctx, actor), nil
			})
			handler = capture.NewChannelHandlers(narrowed)
			_, err := handler.ConnectBot(f.ctx, capture.ConnectRequest{Provider: "slack", BotToken: "revoked-synthetic"})
			if !errors.Is(err, apperrors.ErrPermissionDenied) {
				t.Fatalf("stale authority committed consent: %v", err)
			}
			for _, ref := range f.vault.mintedRefs() {
				secret, err := f.vault.Get(f.ctx, f.workspaceKey(), ref)
				if err == nil && string(secret) == "revoked-synthetic" {
					t.Fatal("refused consent left an orphan token")
				}
			}
			live := f.liveConnections(t)
			if !reconnect && len(live) != 0 {
				t.Fatalf("refused consent persisted %d connections", len(live))
			}
			if reconnect && (len(live) != 1 || live[0].ID != prior.ID || live[0].Version != prior.Version) {
				t.Fatalf("refused rotation changed existing connection: %#v", live)
			}
		})
	}
}
