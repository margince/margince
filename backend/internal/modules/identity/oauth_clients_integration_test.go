// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

import (
	"errors"
	"slices"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// actorWith is a signed-in human holding this one grant on agent_client,
// so a test states the authority it relies on rather than a role's defaults.
func (e *cimdEnv) actorWith(t *testing.T, grant principal.ObjectGrant) Identity {
	t.Helper()
	var (
		userID ids.UserID
		wsID   ids.WorkspaceID
	)
	err := database.WithWorkspaceTx(e.ctx, e.svc.db.Pool(), func(tx pgx.Tx) error {
		if err := tx.QueryRow(e.ctx, `SELECT id FROM workspace LIMIT 1`).Scan(&wsID); err != nil {
			return err
		}
		return tx.QueryRow(e.ctx, `SELECT id FROM app_user WHERE email = $1`, e.adminEmail).Scan(&userID)
	})
	if err != nil {
		t.Fatal(err)
	}
	return Identity{
		UserID: userID, WorkspaceID: wsID, Roles: []string{"custom"},
		Permissions: principal.Permissions{
			RowScope: principal.RowScopeAll,
			Objects:  map[string]principal.ObjectGrant{agentClientObject: grant},
		},
	}
}

var fullAgentClientGrant = principal.ObjectGrant{Create: true, Read: true, Update: true, Delete: true}

func TestEveryAgentClientOperationRefusesWhoeverLacksItsVerb(t *testing.T) {
	e := setupCIMD(t, "agent-client-rbac")
	admin := e.actorWith(t, fullAgentClientGrant)
	registered, _, err := e.svc.RegisterAgentClient(e.ctx, admin, RegisterAgentClientInput{
		Name: "Gemini", RedirectURIs: []string{"https://vertexaisearch.cloud.google.com/oauth-redirect"}, Confidential: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	rep := e.actorWith(t, principal.ObjectGrant{})
	ops := e.actorWith(t, principal.ObjectGrant{Read: true})
	for name, call := range map[string]func(Identity) error{
		"register": func(a Identity) error {
			_, _, err := e.svc.RegisterAgentClient(e.ctx, a, RegisterAgentClientInput{Name: "x", RedirectURIs: []string{"https://x.example/cb"}})
			return err
		},
		"rotate": func(a Identity) error { _, err := e.svc.RotateAgentClientSecret(e.ctx, a, registered.ID); return err },
		"disable": func(a Identity) error {
			_, err := e.svc.SetAgentClientDisabled(e.ctx, a, registered.ID, true)
			return err
		},
		"delete": func(a Identity) error { return e.svc.DeleteAgentClient(e.ctx, a, registered.ID) },
	} {
		for who, actor := range map[string]Identity{"no grant": rep, "read only": ops} {
			if err := call(actor); !errors.Is(err, apperrors.ErrPermissionDenied) {
				t.Errorf("%s by %s → %v, want permission denied", name, who, err)
			}
		}
	}
	if _, _, err := e.svc.ListAgentClients(e.ctx, rep, nil, nil); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("list with no grant → %v, want permission denied", err)
	}
	listed, _, err := e.svc.ListAgentClients(e.ctx, ops, nil, nil)
	if err != nil || !slices.ContainsFunc(listed, func(c AgentClient) bool { return c.ID == registered.ID }) {
		t.Errorf("list with read → %v %v, want the registered client", listed, err)
	}
}

// Delete ends connections; it does not ban the vendor's URL. Disable does, and
// a disabled document is never fetched again.
func TestADeletedMetadataDocumentClientComesBackAndADisabledOneDoesNot(t *testing.T) {
	e := setupCIMD(t, "agent-client-cimd")
	var fetches atomic.Int32
	plain := e.body
	e.body = func(url string) string { fetches.Add(1); return plain(url) }
	clientID := e.docs.URL + "/client.json"
	if err := e.svc.resolveCIMDClient(e.ctx, clientID); err != nil {
		t.Fatal(err)
	}
	admin := e.actorWith(t, fullAgentClientGrant)
	row := e.agentClientRow(t, admin, clientID)

	if err := e.svc.DeleteAgentClient(e.ctx, admin, row.ID); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.resolveCIMDClient(e.ctx, clientID); err != nil {
		t.Fatalf("the next sign-in after a delete → %v, want the document fetched again", err)
	}
	if fetches.Load() != 2 {
		t.Fatalf("fetched %d times, want 2: a delete must make the cached document stale", fetches.Load())
	}
	back := e.agentClientRow(t, admin, clientID)
	if back.ID != row.ID {
		t.Errorf("the client came back as row %s, want its own row %s", back.ID, row.ID)
	}

	if _, err := e.svc.SetAgentClientDisabled(e.ctx, admin, row.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.resolveCIMDClient(e.ctx, clientID); err != nil {
		t.Fatal(err)
	}
	if fetches.Load() != 2 {
		t.Errorf("fetched %d times, want still 2: a disabled client is never fetched for", fetches.Load())
	}
}

func (e *cimdEnv) agentClientRow(t *testing.T, actor Identity, clientID string) AgentClient {
	t.Helper()
	clients, _, err := e.svc.ListAgentClients(e.ctx, actor, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range clients {
		if c.ClientID == clientID {
			return c
		}
	}
	t.Fatalf("%q is not listed", clientID)
	return AgentClient{}
}

// Self-registered clients pile up, so the list pages, and the clients an admin
// registered come before them on every page boundary.
func TestTheClientListPagesWithAdminClientsFirst(t *testing.T) {
	e := setupCIMD(t, "agent-client-page")
	admin := e.actorWith(t, fullAgentClientGrant)
	err := database.WithWorkspaceTx(e.ctx, e.svc.db.Pool(), func(tx pgx.Tx) error {
		for i := range 3 {
			if _, err := tx.Exec(e.ctx, `INSERT INTO oauth_client (client_id, client_name, redirect_uris, created_at)
				VALUES ($1, 'self', ARRAY['https://x.example/cb'], now() - make_interval(mins => $2))`,
				ids.NewV7().String(), i+1); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, _, err := e.svc.RegisterAgentClient(e.ctx, admin, RegisterAgentClientInput{
			Name: "admin", RedirectURIs: []string{"https://x.example/cb"},
		}); err != nil {
			t.Fatal(err)
		}
	}

	var seen []AgentClient
	var cursor *string
	limit := 2
	for page := 0; ; page++ {
		clients, next, err := e.svc.ListAgentClients(e.ctx, admin, cursor, &limit)
		if err != nil {
			t.Fatalf("page %d: %v", page, err)
		}
		seen = append(seen, clients...)
		if !next.HasMore {
			break
		}
		cursor = &next.NextCursor
	}
	if len(seen) != 5 {
		t.Fatalf("walked %d clients, want 5", len(seen))
	}
	if seen[0].RegisteredBy != createdViaAdmin || seen[1].RegisteredBy != createdViaAdmin || seen[2].RegisteredBy == createdViaAdmin {
		t.Errorf("order = %v, want the two admin clients first", []string{seen[0].RegisteredBy, seen[1].RegisteredBy, seen[2].RegisteredBy})
	}
	bad := "nonsense"
	if _, _, err := e.svc.ListAgentClients(e.ctx, admin, &bad, nil); err == nil {
		t.Error("a cursor nobody minted must be refused")
	}
}
