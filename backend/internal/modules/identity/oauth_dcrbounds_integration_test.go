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
	"time"

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
	return rec.Code, body.ClientID
}

// consent stands in for the consent a human gives: it makes the grant that
// marks the client used.
func (e *cimdEnv) consent(t *testing.T, clientID string) {
	t.Helper()
	admin := e.actorWith(t, fullAgentClientGrant)
	err := database.WithTx(e.ctx, e.svc.db.Pool(), func(tx pgx.Tx) error {
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
	err := database.WithTx(e.ctx, e.svc.db.Pool(), func(tx pgx.Tx) error {
		return tx.QueryRow(e.ctx, `SELECT EXISTS (SELECT 1 FROM oauth_client WHERE client_id = $1)`, clientID).Scan(&exists)
	})
	if err != nil {
		t.Fatal(err)
	}
	return exists
}

// ageSelfMade moves every self-made client's registration back by age, as if
// that much time had passed since each one registered.
func (e *cimdEnv) ageSelfMade(t *testing.T, age time.Duration) {
	t.Helper()
	err := database.WithTx(e.ctx, e.svc.db.Pool(), func(tx pgx.Tx) error {
		_, err := tx.Exec(e.ctx, `UPDATE oauth_client SET created_at = created_at - make_interval(secs => $1::float8)
			WHERE created_via IN ('dcr', 'cimd')`, age.Seconds())
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

// unusedSelfMade counts the self-made clients no consent used.
func (e *cimdEnv) unusedSelfMade(t *testing.T) int {
	t.Helper()
	var n int
	err := database.WithTx(e.ctx, e.svc.db.Pool(), func(tx pgx.Tx) error {
		return tx.QueryRow(e.ctx, `SELECT count(*) FROM oauth_client c WHERE `+unusedSelfMadeClient).Scan(&n)
	})
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// A full cap makes room instead of refusing: someone filling it cannot shut a
// real tool out, and the never-used clients stay at the cap.
func TestAFullCapMakesRoomByRemovingTheOldestUnusedClient(t *testing.T) {
	e := setupCIMD(t, "dcr-cap")
	h := Handlers{svc: e.svc}.WithUnusedDynamicClientCap(2)
	admin := e.actorWith(t, fullAgentClientGrant)
	byAdmin, _, err := e.svc.RegisterAgentClient(e.ctx, admin, RegisterAgentClientInput{
		Name: "Admin tool", RedirectURIs: []string{"https://admin-tool.example/cb"},
	})
	if err != nil {
		t.Fatal(err)
	}
	registered := make([]string, 0, 4)
	for i := range 4 {
		code, clientID := e.selfRegister(t, h, "Tool")
		if code != http.StatusCreated {
			t.Fatalf("registration %d answered %d, want 201 whatever the table holds", i+1, code)
		}
		registered = append(registered, clientID)
		if i == 1 {
			e.consent(t, registered[0])
		}
		if i == 2 {
			e.ageSelfMade(t, 2*selfMadeClientSignInWindow)
		}
	}
	if got := e.unusedSelfMade(t); got != 2 {
		t.Errorf("never-used self-made clients = %d, want the cap of 2", got)
	}
	for i, want := range []bool{true, false, true, true} {
		if kept := e.clientExists(t, registered[i]); kept != want {
			t.Errorf("registration %d: kept = %v, want %v (the first was used, the second was the oldest unused)", i+1, kept, want)
		}
	}
	if !e.clientExists(t, byAdmin.ClientID) {
		t.Error("an admin's client was removed to make room; only self-made clients count")
	}
}

// A metadata-document client is a self-made client too: its first resolution
// is held to the cap a registration is.
func TestADocumentClientMakesRoomUnderTheSameCap(t *testing.T) {
	e := setupCIMD(t, "cimd-cap")
	h := Handlers{svc: e.svc}.WithUnusedDynamicClientCap(1)
	_, registered := e.selfRegister(t, h, "Tool")
	e.ageSelfMade(t, 2*selfMadeClientSignInWindow)
	clientID := e.docs.URL + "/client.json"
	if err := e.svc.resolveCIMDClient(e.ctx, clientID, 1); err != nil {
		t.Fatal(err)
	}
	if e.clientExists(t, registered) || !e.clientExists(t, clientID) {
		t.Errorf("after a document client under a cap of 1: registered kept = %v, document kept = %v; want false, true",
			e.clientExists(t, registered), e.clientExists(t, clientID))
	}
	if err := e.svc.resolveCIMDClient(e.ctx, clientID, 1); err != nil {
		t.Fatal(err)
	}
	if got := e.unusedSelfMade(t); got != 1 {
		t.Errorf("refreshing a known document client changed the count to %d, want 1", got)
	}
}

// A client inside its sign-in window has no grant yet, because the grant
// appears only at the token exchange. A registration flood at the cap must not
// evict it mid-consent, so the table goes over the cap instead.
func TestAClientStillSigningInIsNotEvictedAtTheCap(t *testing.T) {
	e := setupCIMD(t, "dcr-window")
	h := Handlers{svc: e.svc}.WithUnusedDynamicClientCap(1)
	_, signingIn := e.selfRegister(t, h, "Signing-in")
	code, flood := e.selfRegister(t, h, "Flood")
	if code != http.StatusCreated {
		t.Fatalf("a registration at the cap answered %d, want 201", code)
	}
	if !e.clientExists(t, signingIn) || !e.clientExists(t, flood) {
		t.Errorf("kept signing-in = %v, flood = %v; want both, as neither is past the sign-in window",
			e.clientExists(t, signingIn), e.clientExists(t, flood))
	}
	e.ageSelfMade(t, 2*selfMadeClientSignInWindow)
	_, next := e.selfRegister(t, h, "Next")
	if e.clientExists(t, signingIn) || !e.clientExists(t, next) {
		t.Errorf("past the window: kept oldest = %v, newest = %v; want false, true",
			e.clientExists(t, signingIn), e.clientExists(t, next))
	}
}

// A disabled document client is the ban on its URL. Neither the cap nor the
// sweep may delete it, or the next sign-in would fetch the document again.
func TestADisabledDocumentClientSurvivesTheCapAndTheSweep(t *testing.T) {
	e := setupCIMD(t, "cimd-disabled")
	h := Handlers{svc: e.svc}.WithUnusedDynamicClientCap(1)
	banned := e.docs.URL + "/client.json"
	if err := e.svc.resolveCIMDClient(e.ctx, banned, DefaultUnusedDynamicClientCap); err != nil {
		t.Fatal(err)
	}
	err := database.WithTx(e.ctx, e.svc.db.Pool(), func(tx pgx.Tx) error {
		_, err := tx.Exec(e.ctx, `UPDATE oauth_client SET disabled_at = now() WHERE client_id = $1`, banned)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	e.ageSelfMade(t, 30*24*time.Hour)
	for _, name := range []string{"First", "Second"} {
		if code, _ := e.selfRegister(t, h, name); code != http.StatusCreated {
			t.Fatalf("registering %s answered %d", name, code)
		}
	}
	if !e.clientExists(t, banned) {
		t.Fatal("making room under the cap deleted a disabled document client")
	}
	if _, err := e.svc.PurgeUnusedDynamicClients(principal.SystemActing(e.ctx, "system:test"),
		DefaultUnusedDynamicClientRetention, 100); err != nil {
		t.Fatal(err)
	}
	if !e.clientExists(t, banned) {
		t.Error("the sweep deleted a disabled document client")
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
	clients["old document"] = e.docs.URL + "/client.json"
	if err := e.svc.resolveCIMDClient(e.ctx, clients["old document"], DefaultUnusedDynamicClientCap); err != nil {
		t.Fatal(err)
	}
	e.consent(t, clients["old used"])
	err = database.WithTx(e.ctx, e.svc.db.Pool(), func(tx pgx.Tx) error {
		_, err := tx.Exec(e.ctx, `UPDATE oauth_client SET created_at = now() - interval '30 days'
			WHERE client_id = ANY($1)`, []string{clients["old unused"], clients["old used"], clients["old by an admin"], clients["old document"]})
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
	if purged != 2 {
		t.Errorf("purged %d clients, want 2", purged)
	}
	for name, clientID := range clients {
		if kept, want := e.clientExists(t, clientID), name != "old unused" && name != "old document"; kept != want {
			t.Errorf("%s client: kept = %v, want %v", name, kept, want)
		}
	}
}

// A consent holds its client locked while it makes the grant. The sweep skips
// that client and still deletes the rest of its batch, and the consent then
// commits.
func TestTheSweepLeavesAClientAConsentHoldsAndDeletesTheRest(t *testing.T) {
	e := setupCIMD(t, "dcr-race")
	h := Handlers{svc: e.svc}
	_, held := e.selfRegister(t, h, "Held")
	_, idle := e.selfRegister(t, h, "Idle")
	admin := e.actorWith(t, fullAgentClientGrant)

	consent, err := e.svc.db.Pool().Begin(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := consent.Rollback(e.ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Error(err)
		}
	}()
	if err := lockClientRegistration(e.ctx, consent, held); err != nil {
		t.Fatal(err)
	}
	purged, err := e.svc.PurgeUnusedDynamicClients(principal.SystemActing(e.ctx, "system:test"), 0, 100)
	if err != nil {
		t.Fatalf("the sweep failed while a consent held one client: %v", err)
	}
	if purged != 1 || e.clientExists(t, idle) {
		t.Errorf("purged %d, idle client kept = %v; want 1 and false", purged, e.clientExists(t, idle))
	}
	if _, err := consent.Exec(e.ctx, `INSERT INTO oauth_grant (client_id, user_id, scopes) VALUES ($1, $2, '{read}')`,
		held, admin.UserID); err != nil {
		t.Fatalf("the consent could not make its grant after the sweep: %v", err)
	}
	if err := consent.Commit(e.ctx); err != nil {
		t.Fatalf("the consent could not commit after the sweep: %v", err)
	}
}
