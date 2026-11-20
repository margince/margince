// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// selfRegister posts one registration document as a tool would, and answers
// the status and the client_id a 201 carries.
func (e *cimdEnv) selfRegister(t *testing.T, h Handlers, name string) (int, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	document := `{"client_name":"` + name + `","redirect_uris":["https://tool.example/cb"]}`
	h.oauthRegister(rec, httptest.NewRequest(http.MethodPost, "/oauth/register", strings.NewReader(document)).WithContext(e.ctx))
	var body struct {
		ClientID string `json:"client_id"`
		Error    string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("register answered no JSON document: %v (%s)", err, rec.Body.String())
	}
	if rec.Code == http.StatusTooManyRequests && body.Error != oauthErrTemporarilyUnavailable {
		t.Fatalf("a refusal at the cap answered error %q, want %q", body.Error, oauthErrTemporarilyUnavailable)
	}
	return rec.Code, body.ClientID
}

// consent stands in for the consent a human gives: it makes the grant that
// marks the client used.
func (e *cimdEnv) consent(t *testing.T, clientID string) {
	t.Helper()
	admin := e.actorWith(t, fullAgentClientGrant)
	err := database.WithWorkspaceTx(e.ctx, e.svc.db.Pool(), func(tx pgx.Tx) error {
		_, err := tx.Exec(e.ctx, `INSERT INTO oauth_grant (client_id, user_id, scopes) VALUES ($1, $2, '{read}')`,
			clientID, admin.UserID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

// clientExists reports whether any oauth_client row, deleted or not, holds clientID.
func (e *cimdEnv) clientExists(t *testing.T, clientID string) bool {
	t.Helper()
	var exists bool
	err := database.WithWorkspaceTx(e.ctx, e.svc.db.Pool(), func(tx pgx.Tx) error {
		return tx.QueryRow(e.ctx, `SELECT EXISTS (SELECT 1 FROM oauth_client WHERE client_id = $1)`, clientID).Scan(&exists)
	})
	if err != nil {
		t.Fatal(err)
	}
	return exists
}

func TestSelfRegistrationStopsAtTheCapOnClientsNoConsentUsed(t *testing.T) {
	e := setupCIMD(t, "dcr-cap")
	h := Handlers{svc: e.svc}.WithUnusedDynamicClientCap(2)
	admin := e.actorWith(t, fullAgentClientGrant)
	if _, _, err := e.svc.RegisterAgentClient(e.ctx, admin, RegisterAgentClientInput{
		Name: "Admin tool", RedirectURIs: []string{"https://admin-tool.example/cb"},
	}); err != nil {
		t.Fatal(err)
	}

	var first string
	for i := range 2 {
		code, clientID := e.selfRegister(t, h, "Tool")
		if code != http.StatusCreated {
			t.Fatalf("registration %d under the cap answered %d", i+1, code)
		}
		if i == 0 {
			first = clientID
		}
	}
	if code, _ := e.selfRegister(t, h, "Tool"); code != http.StatusTooManyRequests {
		t.Fatalf("a registration past the cap answered %d, want 429", code)
	}

	e.consent(t, first)
	if code, _ := e.selfRegister(t, h, "Tool"); code != http.StatusCreated {
		t.Errorf("a registration after a consent used one client answered %d, want 201: "+
			"a used client does not count against the cap", code)
	}
}

func TestTheSweepDeletesOnlyOldSelfRegisteredClientsNoConsentUsed(t *testing.T) {
	e := setupCIMD(t, "dcr-sweep")
	h := Handlers{svc: e.svc}
	clients := map[string]string{}
	for _, name := range []string{"old unused", "old used", "fresh unused"} {
		code, clientID := e.selfRegister(t, h, strings.ReplaceAll(name, " ", "-"))
		if code != http.StatusCreated {
			t.Fatalf("registering %s answered %d", name, code)
		}
		clients[name] = clientID
	}
	admin := e.actorWith(t, fullAgentClientGrant)
	byAdmin, _, err := e.svc.RegisterAgentClient(e.ctx, admin, RegisterAgentClientInput{
		Name: "Admin tool", RedirectURIs: []string{"https://admin-tool.example/cb"},
	})
	if err != nil {
		t.Fatal(err)
	}
	clients["old by an admin"] = byAdmin.ClientID
	e.consent(t, clients["old used"])
	err = database.WithWorkspaceTx(e.ctx, e.svc.db.Pool(), func(tx pgx.Tx) error {
		_, err := tx.Exec(e.ctx, `UPDATE oauth_client SET created_at = now() - interval '30 days'
			WHERE client_id = ANY($1)`, []string{clients["old unused"], clients["old used"], clients["old by an admin"]})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	asHuman := principal.WithActor(e.ctx, principal.Principal{Type: principal.PrincipalHuman, UserID: admin.UserID.UUID})
	if _, err := e.svc.PurgeUnusedDynamicClients(asHuman, DefaultUnusedDynamicClientRetention, 100); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a sweep by a human answered %v, want permission denied", err)
	}
	purged, err := e.svc.PurgeUnusedDynamicClients(principal.SystemActing(e.ctx, "system:test"),
		DefaultUnusedDynamicClientRetention, 100)
	if err != nil {
		t.Fatal(err)
	}
	if purged != 1 {
		t.Errorf("purged %d clients, want 1", purged)
	}
	for name, clientID := range clients {
		if kept, want := e.clientExists(t, clientID), name != "old unused"; kept != want {
			t.Errorf("%s client: kept = %v, want %v", name, kept, want)
		}
	}
}
