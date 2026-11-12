// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package sharedmailbox

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func TestAPersonalConnectionIsNoSharedMailboxToAnyEndpoint(t *testing.T) {
	s := newSharedMailboxSuite(t)
	e := s.e
	personal := s.connectRepMailbox(t).String()
	for _, call := range []struct {
		method, path string
		body         map[string]any
	}{
		{http.MethodPatch, "/shared-mailboxes/" + personal, map[string]any{"reader_team_id": e.Team1, "confirm": true}},
		{http.MethodPatch, "/shared-mailboxes/" + personal, map[string]any{"mail_visibility": "held", "apply_to_history": true, "confirm": true}},
		{http.MethodPost, "/shared-mailboxes/" + personal + "/disconnect", nil},
	} {
		if rec := s.call(s.admin(), t, call.method, call.path, call.body); rec.Code != http.StatusNotFound {
			t.Errorf("%s %s on a personal connection answered %d, want 404", call.method, call.path, rec.Code)
		}
	}
	enqueue := func(context.Context, pgx.Tx, ids.UUID) error { return nil }
	if _, err := s.registry.StartSharedBackfill(s.admin(), ids.MustParse(personal), 3, connector.BackfillEstimate{}, enqueue); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("a history import on a personal connection got %v, want not found", err)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM capture_connection WHERE id = $1 AND status = 'connected' AND reader_team_id IS NULL`, personal); got != 1 {
		t.Fatal("a refused shared call must leave the personal connection as it was")
	}
}

func TestAConnectThatCannotOpenItsTransactionLeavesNoSealedCredential(t *testing.T) {
	s := newSharedMailboxSuite(t)
	e := s.e
	unbound := database.Bind(e.Pool, func(context.Context) (ids.WorkspaceID, error) {
		return ids.WorkspaceID{}, errors.New("no installation workspace")
	})
	registry := capture.NewRegistry(unbound, nil, nil, s.vault)
	registry.Register(&deliveringMailbox{name: "imap", address: salesMailbox})
	_, err := registry.ConnectShared(withReadScope(s.admin()), capture.SharedConnect{
		Provider: "imap", Address: salesMailbox, ReaderTeam: e.Team1, Auth: connector.Auth("secret"),
	})
	if err == nil {
		t.Fatal("a connect whose transaction cannot begin must fail")
	}
	if got := e.WsCount(t, `SELECT count(*) FROM vault_secret`); got != 0 {
		t.Fatalf("the failed connect left %d sealed credentials", got)
	}
}

// withReadScope lends the actor the read scope a mail connector declares.
func withReadScope(ctx context.Context) context.Context {
	actor, ok := principal.Actor(ctx)
	if !ok {
		return ctx
	}
	actor.Scopes = principal.NewScopeSet(principal.ScopeRead)
	return principal.WithActor(ctx, actor)
}
