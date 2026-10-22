// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	capturemod "github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// The mailbox gate and the sink read the two mail postures inside their own
// writes. A boot value wins there over a row that says the opposite. Not
// parallel: the binding is the process's own.
func TestBootSetMailPosturesWinInsideTheMailboxGateAndTheSink(t *testing.T) {
	apptest.BindEnv(t, []string{
		"MARGINCE_CAPTURE_SHARED_POSTURE_ALLOWED=true",
		"MARGINCE_CAPTURE_MAIL_SHARING=false",
	})
	e := integration.SetupSearch(t)
	if _, err := e.Owner.Exec(context.Background(), `
		INSERT INTO setting (key, value) VALUES ($1, 'false'::jsonb), ($2, 'true'::jsonb)
		ON CONFLICT (key) DO UPDATE SET value = excluded.value`,
		capturemod.SharedPostureAllowed.Key(), capturemod.MailSharing.Key()); err != nil {
		t.Fatalf("saving the opposite postures: %v", err)
	}
	contactID := e.SeedID(t, `INSERT INTO contact (id, full_name, source, captured_by) VALUES ($1, 'Inbox Sender', 'manual', 'human:x')`)
	registry := newTestCaptureRegistry(e, newTestKeyvault(t, e))
	registry.Register(&mailFake{linkTo: contactID})
	grantCtx := humanWithScopes(e, e.Rep1, []principal.Scope{principal.ScopeRead})
	connID, err := registry.Connect(grantCtx, "graph", connector.Auth("token"))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := registry.SetMailPosture(grantCtx, "graph", capturemod.PostureShared, false); err != nil {
		t.Fatalf("the boot-set opt-in did not admit a shared mailbox: %v", err)
	}
	if err := registry.SyncOnce(grantCtx, connID); err != nil {
		t.Fatal(err)
	}
	var audience string
	if err := e.Owner.QueryRow(context.Background(),
		`SELECT audience FROM activity WHERE source_system = 'email' AND source_id = 'msg-1'`).Scan(&audience); err != nil {
		t.Fatal(err)
	}
	if audience != "participants" {
		t.Fatalf("with sharing off at boot a shared mailbox's email is born %q, want participants", audience)
	}
}
