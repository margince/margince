// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture_test

// The events secret on a channel connection. It is sealed on connect or on a
// token replacement, and never shown. Setting it is in the audit, and the
// disconnect destroys it.

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const (
	pushProvider = "dispact"
	pushTeam     = "team-push"
	pushToken    = "gwapp_push"
	pushSecret   = "whsec_Zmlyc3Qtc2VjcmV0"
)

// pushBotCheck is a provider that pushes signed notices: one team, any token
// with its prefix, and a secret with the whsec_ prefix.
type pushBotCheck struct{}

func (pushBotCheck) ValidateToken(token string) error {
	if !strings.HasPrefix(token, "gwapp_") {
		return capture.ErrChannelTokenRejected
	}
	return nil
}

func (pushBotCheck) Confirm(context.Context, string, string) (capture.ChannelBot, error) {
	return capture.ChannelBot{ID: pushTeam, Label: "Margince"}, nil
}

func (pushBotCheck) NeedsAddress() bool  { return true }
func (pushBotCheck) FixedIdentity() bool { return true }

func (pushBotCheck) ValidateEventsSecret(secret string) error {
	if !strings.HasPrefix(secret, "whsec_") {
		return errors.New("no whsec_ prefix")
	}
	return nil
}

func (f *channelFixture) withPushProvider() {
	f.store = f.store.WithBotCheck(pushProvider, pushBotCheck{})
}

func (f *channelFixture) connectPush(t *testing.T, secret string) capture.ChannelConnection {
	t.Helper()
	conn, err := f.store.Connect(f.ctx, capture.ConnectRequest{
		Provider: pushProvider, BotToken: pushToken, Address: "https://chat.example.test", EventsSecret: secret,
	})
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	return conn
}

// eventsRefOf reads the sealed events secret's ref, empty when none.
func (f *channelFixture) eventsRefOf(t *testing.T, id ids.UUID) keyvault.Ref {
	t.Helper()
	owner, _ := setupCaptureDB(t)
	var ref *string
	if err := owner.QueryRow(context.Background(),
		`SELECT events_credential_ref FROM channel_connection WHERE id = $1`, id).Scan(&ref); err != nil {
		t.Fatalf("reading the events ref of %s: %v", id, err)
	}
	if ref == nil {
		return ""
	}
	return keyvault.Ref(*ref)
}

func (f *channelFixture) sealed(t *testing.T, ref keyvault.Ref) (string, bool) {
	t.Helper()
	secret, err := f.vault.Get(context.Background(), f.workspaceKey(), ref)
	if errors.Is(err, keyvault.ErrNotFound) {
		return "", false
	}
	if err != nil {
		t.Fatalf("reading ref %s: %v", ref, err)
	}
	return string(secret), true
}

// lastAuditAfter reads the after image of the newest audit row of id.
func (f *channelFixture) lastAuditAfter(t *testing.T, id ids.UUID) map[string]any {
	t.Helper()
	owner, _ := setupCaptureDB(t)
	var raw []byte
	if err := owner.QueryRow(context.Background(), `SELECT after FROM audit_log
		WHERE entity_type = 'channel_connection' AND entity_id = $1 ORDER BY occurred_at DESC, id DESC LIMIT 1`, id).Scan(&raw); err != nil {
		t.Fatalf("reading the audit of %s: %v", id, err)
	}
	var after map[string]any
	if err := json.Unmarshal(raw, &after); err != nil {
		t.Fatal(err)
	}
	return after
}

func TestAnEventsSecretIsSealedShownAsSetAndAudited(t *testing.T) {
	f := newChannelFixture(t, nil)
	f.withPushProvider()
	conn := f.connectPush(t, pushSecret)

	if !conn.EventsSecretSet {
		t.Error("the connection does not report its events secret as set")
	}
	if secret, ok := f.sealed(t, f.eventsRefOf(t, conn.ID)); !ok || secret != pushSecret {
		t.Errorf("the vault holds %q under the events ref; want the secret the admin pasted", secret)
	}
	if after := f.lastAuditAfter(t, conn.ID); after["events_secret_set"] != true {
		t.Errorf("the connect's audit shows %v; want events_secret_set true", after)
	}
	_, pool := setupCaptureDB(t)
	registry := capture.NewRegistry(database.BindTo(pool, f.workspaceKey()), nil, nil, f.vault)
	bot, secret, err := registry.LiveBotEvents(f.ctx, pushProvider)
	if err != nil || secret != pushSecret || bot.TeamID != pushTeam {
		t.Errorf("LiveBotEvents = %+v, %q, %v; want team %s and the sealed secret", bot, secret, err, pushTeam)
	}
}

func TestABotWithNoEventsSecretAnswersSo(t *testing.T) {
	f := newChannelFixture(t, nil)
	f.withPushProvider()
	conn := f.connectPush(t, "")
	if conn.EventsSecretSet || f.eventsRefOf(t, conn.ID) != "" {
		t.Error("a connect with no events secret sealed one")
	}
	_, pool := setupCaptureDB(t)
	registry := capture.NewRegistry(database.BindTo(pool, f.workspaceKey()), nil, nil, f.vault)
	if _, _, err := registry.LiveBotEvents(f.ctx, pushProvider); !errors.Is(err, capture.ErrNoEventsSecret) {
		t.Errorf("LiveBotEvents with no secret: %v; want ErrNoEventsSecret", err)
	}
}

func TestAProviderThatPushesNothingRefusesAnEventsSecret(t *testing.T) {
	f := newChannelFixture(t, nil)
	f.withPushProvider()
	token, _ := f.api.withNewBot("refusing_bot")
	for name, req := range map[string]capture.ConnectRequest{
		"telegram":         {Provider: capture.ProviderTelegram, BotToken: token, EventsSecret: pushSecret},
		"a secret no chat": {Provider: pushProvider, BotToken: pushToken, Address: "https://chat.example.test", EventsSecret: "not-a-secret"},
	} {
		if _, err := f.store.Connect(f.ctx, req); !errors.Is(err, apperrors.ErrInvalidArgument) {
			t.Errorf("%s: Connect = %v; want ErrInvalidArgument", name, err)
		}
	}
	if n, puts := f.rowCount(t), f.vault.putCount(); n != 0 || puts != 0 {
		t.Errorf("a refused events secret left %d row(s) and %d sealed secret(s)", n, puts)
	}
}

func TestReplacingTheTokenReplacesOrKeepsTheEventsSecret(t *testing.T) {
	f := newChannelFixture(t, nil)
	f.withPushProvider()
	conn := f.connectPush(t, pushSecret)
	first := f.eventsRefOf(t, conn.ID)

	if err := f.store.ReplaceToken(f.ctx, conn.ID, pushToken+"2", ""); err != nil {
		t.Fatalf("ReplaceToken without a secret: %v", err)
	}
	if kept := f.eventsRefOf(t, conn.ID); kept != first {
		t.Errorf("a replacement with no events secret moved the ref from %s to %s", first, kept)
	}

	const rotated = "whsec_c2Vjb25kLXNlY3JldA=="
	if err := f.store.ReplaceToken(f.ctx, conn.ID, pushToken+"3", rotated); err != nil {
		t.Fatalf("ReplaceToken with a secret: %v", err)
	}
	if secret, ok := f.sealed(t, f.eventsRefOf(t, conn.ID)); !ok || secret != rotated {
		t.Errorf("the vault holds %q after the rotation; want %q", secret, rotated)
	}
	if _, ok := f.sealed(t, first); ok {
		t.Error("the replaced events secret is still in the vault, and no row names it")
	}
	if after := f.lastAuditAfter(t, conn.ID); after["events_secret_replaced"] != true {
		t.Errorf("the rotation's audit shows %v; want events_secret_replaced true", after)
	}
}

func TestDisconnectDestroysTheEventsSecret(t *testing.T) {
	f := newChannelFixture(t, nil)
	f.withPushProvider()
	conn := f.connectPush(t, pushSecret)
	token, events := f.credentialRefOf(t, conn.ID), f.eventsRefOf(t, conn.ID)
	if err := f.store.Disconnect(f.ctx, conn.ID); err != nil {
		t.Fatalf("Disconnect: %v", err)
	}
	for what, ref := range map[string]keyvault.Ref{"bot token": token, "events secret": events} {
		if _, ok := f.sealed(t, ref); ok {
			t.Errorf("the %s outlived the disconnect", what)
		}
	}
}
