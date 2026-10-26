// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"testing/fstest"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/extsecrets"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/pkg/extension"
)

const activationObject = "ext_activation_note"

// activationFixture is one unit that holds a setting, owns an RBAC object and
// keeps a secret: everything the activation step touches.
type activationFixture struct {
	env   *Env
	vault keyvault.Vault
	unit  extension.Extension
	verbs []extension.Verb
}

func newActivationFixture(t *testing.T) *activationFixture {
	t.Helper()
	if err := identity.RegisterRbacObjects(activationObject); err != nil {
		t.Fatalf("registering %s: %v", activationObject, err)
	}
	t.Cleanup(identity.ResetRbacObjectsForTest)
	return &activationFixture{
		env:   Setup(t),
		vault: keyvault.NewMemory(),
		unit: extension.Extension{
			Name:     "activation",
			Version:  "1.0.0",
			Settings: fstest.MapFS{extension.SettingsFile: {Data: []byte(`{"vat_rate": 19}`)}},
		},
		verbs: []extension.Verb{{Unit: "activation", RbacObject: activationObject}},
	}
}

func (f *activationFixture) activate(t *testing.T, exts ...extension.Extension) {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := compose.ActivateExtensions(context.Background(), f.env.Pool, f.vault, log, exts, f.verbs); err != nil {
		t.Fatalf("ActivateExtensions: %v", err)
	}
}

func (f *activationFixture) ledger(t *testing.T, action string) int {
	t.Helper()
	return f.env.WsCount(t, `SELECT count(*) FROM system_log WHERE action = $1`, action)
}

func (f *activationFixture) adminGrants(t *testing.T) int {
	t.Helper()
	return f.env.WsCount(t,
		`SELECT count(*) FROM role WHERE key = 'admin' AND permissions->'objects' ? $1`, activationObject)
}

func (f *activationFixture) status(t *testing.T) map[string]string {
	t.Helper()
	ctx := principal.WithWorkspaceID(context.Background(), f.env.WS)
	got, err := compose.ActivatedExtensions(ctx, f.env.Pool)
	if err != nil {
		t.Fatalf("ActivatedExtensions: %v", err)
	}
	return got
}

// TestInstallingAUnitSeedsGrantsAndReportsReady: the first boot with a new unit
// stores its settings and grants the admin role its objects, with an audit row.
// It then marks the unit ready at its version.
func TestInstallingAUnitSeedsGrantsAndReportsReady(t *testing.T) {
	f := newActivationFixture(t)
	f.activate(t, f.unit)

	if got := f.status(t)["activation"]; got != "1.0.0" {
		t.Fatalf("activated version = %q, want 1.0.0", got)
	}
	if got := f.env.WsCount(t, `SELECT count(*) FROM setting WHERE key = 'ext_activation.vat_rate'`); got != 1 {
		t.Fatalf("seeded settings = %d, want 1", got)
	}
	if got := f.adminGrants(t); got != 1 {
		t.Fatalf("admin role holds the unit's object = %d, want 1", got)
	}
	audited := f.env.WsCount(t, `SELECT count(*) FROM audit_log
		WHERE entity_type = 'role' AND action = 'update' AND actor_type = 'system' AND after::text LIKE '%' || $1 || '%'`, activationObject)
	if audited != 1 {
		t.Fatalf("audit rows for the grant = %d, want 1", audited)
	}
}

// TestActivatingAgainChangesNothing: a reboot of an unchanged set writes no row,
// and an operator's later narrowing of the grant survives it.
func TestActivatingAgainChangesNothing(t *testing.T) {
	f := newActivationFixture(t)
	f.activate(t, f.unit)
	rows := f.ledger(t, "extension.activated")

	f.activate(t, f.unit)
	if got := f.ledger(t, "extension.activated"); got != rows {
		t.Fatalf("a repeat boot wrote activation rows: %d, want %d", got, rows)
	}
	if got := f.env.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'role' AND action = 'update'`); got != 1 {
		t.Fatalf("a repeat boot wrote audit rows: %d role updates, want 1", got)
	}
}

// TestRemovingAUnitRevokesItsSecrets: once a unit leaves the composed set its
// secret rows and the sealed material are gone, and the removal is recorded.
func TestRemovingAUnitRevokesItsSecrets(t *testing.T) {
	f := newActivationFixture(t)
	f.activate(t, f.unit)

	ctx := principal.SystemActing(principal.WithWorkspaceID(context.Background(), f.env.WS), "system:test")
	if err := extsecrets.For("activation", f.env.Pool, f.vault).Put(ctx, "signing", []byte("s3cret")); err != nil {
		t.Fatalf("storing the unit's secret: %v", err)
	}
	ref := keyvault.Ref(f.env.WsScalar(t, `SELECT vault_ref FROM extension_secret WHERE extension_name = 'activation'`))

	f.activate(t)

	if got := f.env.WsCount(t, `SELECT count(*) FROM extension_secret WHERE extension_name = 'activation'`); got != 0 {
		t.Fatalf("secret rows after removal = %d, want 0", got)
	}
	if _, err := f.vault.Get(ctx, ids.From[ids.WorkspaceKind](f.env.WS), ref); err == nil {
		t.Fatal("the sealed material survived the removal")
	}
	if got := f.ledger(t, "extension.removed"); got != 1 {
		t.Fatalf("removal rows = %d, want 1", got)
	}
	if _, ready := f.status(t)["activation"]; ready {
		t.Fatal("a removed unit still reports as activated")
	}
}
