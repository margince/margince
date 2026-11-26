// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"crypto/rand"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// SharedMailbox describes a shared mailbox connection: no seat owns it, an
// admin connected it, and one team reads what it captures.
type SharedMailbox struct {
	Provider    string
	Address     string
	ReaderTeam  ids.UUID
	ConnectedBy ids.UUID
	// Posture is held or shared; a shared mailbox is never classified.
	Posture string
}

// ConnectSharedMailbox connects box through the admin connect, as ConnectedBy,
// and is the one place every suite makes a shared mailbox. registry carries
// a connector named box.Provider and the vault the mailbox's syncs read.
func ConnectSharedMailbox(t *testing.T, registry *capture.Registry, ws ids.UUID, box SharedMailbox) ids.UUID {
	t.Helper()
	admin := principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + box.ConnectedBy.String(), UserID: box.ConnectedBy,
		Permissions: principal.Permissions{RoleKeys: []string{roleAdmin}},
	}
	for _, d := range registry.Connectors() {
		if d.Name == box.Provider {
			admin.Scopes = principal.NewScopeSet(d.Scopes...)
		}
	}
	ctx := principal.WithCorrelationID(principal.WithWorkspaceID(context.Background(), ws), ids.NewV7())
	id, err := registry.ConnectShared(principal.WithActor(ctx, admin), capture.SharedConnect{
		Provider: box.Provider, Address: box.Address, ReaderTeam: box.ReaderTeam, Posture: box.Posture,
		Auth: connector.Auth("shared-mailbox-token"),
	})
	if err != nil {
		t.Fatalf("connecting the shared mailbox: %v", err)
	}
	return id
}

// SharedMailboxRegistry is a registry that can connect a shared mailbox on
// provider, for a suite that never syncs it.
func SharedMailboxRegistry(t *testing.T, pool *pgxpool.Pool, provider string) *capture.Registry {
	t.Helper()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("generating a test root key: %v", err)
	}
	vault, err := keyvault.New(keyvault.Config{RootKey: key, Pool: pool})
	if err != nil {
		t.Fatalf("building the local vault: %v", err)
	}
	registry := capture.NewRegistry(database.Bind(pool, identity.NewService(pool).InstallationWorkspace), nil, nil, vault)
	registry.Register(unsyncedMailbox{name: provider})
	return registry
}

// unsyncedMailbox is a connector that is only ever connected.
type unsyncedMailbox struct{ name string }

func (m unsyncedMailbox) Descriptor() connector.Descriptor {
	return connector.Descriptor{Name: m.name, Version: "1", Scopes: []principal.Scope{principal.ScopeRead}}
}

func (unsyncedMailbox) Authenticate(_ context.Context, req connector.AuthRequest) (connector.Auth, error) {
	return connector.Auth(req.Payload), nil
}

func (unsyncedMailbox) Sync(_ context.Context, _ connector.Auth, cursor connector.Cursor, _ connector.Sink) (connector.Cursor, error) {
	return cursor, nil
}

func (unsyncedMailbox) Normalize(context.Context, connector.RawRecord) ([]connector.NormalizedRecord, error) {
	return nil, nil
}

func (unsyncedMailbox) HealthCheck(context.Context, connector.Auth) error { return nil }
