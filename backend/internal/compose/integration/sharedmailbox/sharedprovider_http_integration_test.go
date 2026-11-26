// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package sharedmailbox

// A Microsoft 365 and a Google Workspace shared mailbox, connected over
// /v1/shared-mailboxes and synced by the registry, against fake providers.

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture/gmail"
	"github.com/margince/margince/backend/internal/modules/capture/graph"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/googlejwt"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const (
	graphUserID  = "4a1d3c2b-0e9f-4a8b-8c7d-6e5f4a3b2c1d"
	notification = "https://hooks.example/webhooks/graph?token=t"
)

// microsoftTenant plays the Entra token endpoint and Graph for sales@. Its
// inbox delta closes on a link that names the mailbox by its object id.
type microsoftTenant struct {
	denyInbox, refuseSubscriptions bool

	mu    sync.Mutex
	paths []string
}

func (m *microsoftTenant) serve(t *testing.T) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		m.paths = append(m.paths, r.Method+" "+r.URL.Path)
		m.mu.Unlock()
		path := r.URL.Path
		switch {
		case strings.HasSuffix(path, "/token"):
			if r.FormValue("client_secret") == rejectedSecret {
				reply(t, w, http.StatusUnauthorized, map[string]any{"error": "invalid_client"})
				return
			}
			reply(t, w, http.StatusOK, map[string]any{"access_token": "app-tok", "expires_in": 3599})
		case r.Header.Get("Authorization") != "Bearer app-tok":
			w.WriteHeader(http.StatusUnauthorized)
		case path == "/subscriptions" && r.Method == http.MethodPost && m.refuseSubscriptions:
			reply(t, w, http.StatusForbidden, map[string]any{"error": map[string]any{"code": "ExtensionError"}})
		case path == "/users/"+salesMailbox:
			reply(t, w, http.StatusOK, map[string]any{"id": graphUserID})
		case strings.HasSuffix(path, "/mailFolders/inbox") && m.denyInbox:
			reply(t, w, http.StatusForbidden, map[string]any{"error": map[string]any{"code": "ErrorAccessDenied"}})
		case strings.HasSuffix(path, "/messages/m1/$value"):
			if _, err := w.Write(orderMail("order-42@acme.example", salesMailbox)); err != nil {
				t.Errorf("writing the message: %v", err)
			}
		case strings.HasSuffix(path, "/mailFolders/inbox/messages/delta") && r.URL.Query().Get("$deltatoken") == "":
			reply(t, w, http.StatusOK, map[string]any{
				"value":            []any{map[string]any{"id": "m1", "receivedDateTime": clocktest.Now(t).Add(-time.Hour).UTC().Format(time.RFC3339)}},
				"@odata.deltaLink": srv.URL + "/users/" + graphUserID + "/mailFolders/inbox/messages/delta?$deltatoken=i1",
			})
		default:
			reply(t, w, http.StatusOK, map[string]any{
				"value": []any{}, "id": "x", "@odata.count": 0, "@odata.deltaLink": srv.URL + path + "?$deltatoken=d1",
			})
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func (m *microsoftTenant) requests() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.paths...)
}

// googleWorkspace plays Google's token endpoint and Gmail for a delegated
// token. Its mailbox holds the one order mail.
type googleWorkspace struct {
	mu     sync.Mutex
	grants []string
	reads  int
}

func (g *googleWorkspace) serve(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		g.mu.Lock()
		defer g.mu.Unlock()
		switch {
		case r.URL.Path == "/token":
			g.grants = append(g.grants, r.FormValue("grant_type"))
			reply(t, w, http.StatusOK, map[string]any{"access_token": "mbx-tok", "expires_in": 3599})
		case r.Header.Get("Authorization") != "Bearer mbx-tok":
			w.WriteHeader(http.StatusUnauthorized)
		case r.URL.Path == "/profile":
			g.reads++
			reply(t, w, http.StatusOK, map[string]any{"emailAddress": salesMailbox, "historyId": "7"})
		case r.URL.Path == "/messages":
			reply(t, w, http.StatusOK, map[string]any{"messages": []any{map[string]any{"id": "g1"}}})
		case r.URL.Path == "/messages/g1":
			raw := base64.RawURLEncoding.EncodeToString(orderMail("order-43@acme.example", salesMailbox))
			reply(t, w, http.StatusOK, map[string]any{"raw": raw, "labelIds": []string{"INBOX"}})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// seen answers the grant types asked for and the profile reads so far.
func (g *googleWorkspace) seen() ([]string, int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]string(nil), g.grants...), g.reads
}

func reply(t *testing.T, w http.ResponseWriter, status int, body map[string]any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		t.Errorf("writing the answer: %v", err)
	}
}

// newProviderSuite is the production registry with the real Graph and Gmail
// connectors, each reading shared mailboxes from its fake.
func newProviderSuite(t *testing.T, ms *microsoftTenant) (*sharedMailboxSuite, *googleWorkspace) {
	t.Helper()
	e := integration.Setup(t)
	registry := compose.CaptureRegistryWithoutConnectorsForTest(e.Pool, testVault(t, e))
	msSrv := ms.serve(t)
	registry.Register(graph.New(nil, graph.NewAPI(nil, "")).WithSharedMailboxes(graph.SharedMailboxConfig{
		Auth:       graph.SharedAuthConfig{TokenURLFormat: msSrv.URL + "/%s/token", HTTPClient: msSrv.Client()},
		HTTPClient: msSrv.Client(), APIBase: msSrv.URL,
	}))
	google := &googleWorkspace{}
	gSrv := google.serve(t)
	registry.Register(gmail.New(nil, gmail.NewAPI(gSrv.Client(), gSrv.URL)).WithSharedMailboxes(gmail.SharedAuthConfig{
		TokenURL: gSrv.URL + "/token", HTTPClient: gSrv.Client(),
	}))
	s := &sharedMailboxSuite{e: e, registry: registry, mux: compose.SharedMailboxRouterForTest(e.Pool, registry, loginCheck)}
	return s, google
}

func graphBody(team ids.UUID, tenant, secret string) map[string]any {
	return map[string]any{
		"provider": "graph", "address": salesMailbox, "reader_team_id": team,
		"graph": map[string]any{"tenant_id": tenant, "client_id": "app-1", "client_secret": secret},
	}
}

func (s *sharedMailboxSuite) connectAs(t *testing.T, body map[string]any) ids.UUID {
	t.Helper()
	rec := s.call(s.admin(), t, http.MethodPost, "/shared-mailboxes", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("the connect answered %d: %s", rec.Code, rec.Body)
	}
	var box struct {
		ID ids.UUID `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &box); err != nil {
		t.Fatal(err)
	}
	return box.ID
}

func TestAMicrosoftSharedMailboxConnectsSyncsByItsObjectIDAndNeverCallsMe(t *testing.T) {
	ms := &microsoftTenant{}
	s, _ := newProviderSuite(t, ms)
	box := s.connectAs(t, graphBody(s.e.Team1, "authz.test", "app-secret"))
	s.sync(t, box)
	// The stored delta link names the mailbox by its object id, which only a
	// bundle holding the id the connect read admits.
	s.sync(t, box)
	if got := s.e.WsCount(t, `SELECT count(*) FROM capture_import WHERE connection_id = $1`, box); got != 1 {
		t.Fatalf("the shared mailbox captured %d messages, want the order mail", got)
	}
	if got := s.e.WsCount(t, `SELECT count(*) FROM capture_connection WHERE id = $1 AND status = 'connected'
		AND sync_cursor->>'email' = $2`, box, salesMailbox); got != 1 {
		t.Fatal("the synced cursor must name the shared address, which push notifications route on")
	}
	for _, request := range ms.requests() {
		if strings.Contains(request, " /me") {
			t.Errorf("a shared mailbox called the signed-in user's root: %s", request)
		}
	}
}

func TestARefusedSubscriptionLeavesTheSharedMailboxOnThePoll(t *testing.T) {
	ms := &microsoftTenant{refuseSubscriptions: true}
	s, _ := newProviderSuite(t, ms)
	box := s.connectAs(t, graphBody(s.e.Team1, "authz.test", "app-secret"))
	ctx := principal.WithWorkspaceID(context.Background(), s.e.WS)
	if err := s.registry.RenewWatch(ctx, box, notification); err == nil {
		t.Fatal("Graph refused the subscription, and the renewal reported success")
	}
	if got := s.e.WsCount(t, `SELECT count(*) FROM capture_connection WHERE id = $1 AND status = 'connected'
		AND watch_expires_at IS NULL`, box); got != 1 {
		t.Fatal("a refused subscription must leave the mailbox connected and unwatched")
	}
	s.sync(t, box)
	if got := s.e.WsCount(t, `SELECT count(*) FROM capture_import WHERE connection_id = $1`, box); got != 1 {
		t.Fatalf("the polled mailbox captured %d messages, want the order mail", got)
	}
}

func TestARefusedMicrosoftProbeStoresNothingAndNamesItsClass(t *testing.T) {
	for name, tc := range map[string]struct {
		denyInbox      bool
		tenant, secret string
		code           string
	}{
		"a refused app":          {false, "authz.test", rejectedSecret, "shared_mailbox_grant_refused"},
		"a mailbox out of scope": {true, "authz.test", "app-secret", "shared_mailbox_mailbox_denied"},
		"a multi-tenant alias":   {false, "common", "app-secret", "shared_mailbox_credentials_invalid"},
	} {
		t.Run(name, func(t *testing.T) {
			s, _ := newProviderSuite(t, &microsoftTenant{denyInbox: tc.denyInbox})
			rec := s.call(s.admin(), t, http.MethodPost, "/shared-mailboxes", graphBody(s.e.Team1, tc.tenant, tc.secret))
			if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), tc.code) {
				t.Fatalf("answered %d %s, want 422 %s", rec.Code, rec.Body, tc.code)
			}
			if strings.Contains(rec.Body.String(), tc.secret) {
				t.Error("the refusal echoes the secret")
			}
			if got := s.e.WsCount(t, `SELECT count(*) FROM capture_connection WHERE user_id IS NULL`) +
				s.e.WsCount(t, `SELECT count(*) FROM vault_secret`); got != 0 {
				t.Fatalf("a refused probe left %d rows or sealed credentials", got)
			}
		})
	}
}

func TestAGoogleSharedMailboxMustBeOnAnOwnDomainAndSyncsWithADelegatedToken(t *testing.T) {
	s, google := newProviderSuite(t, &microsoftTenant{})
	body := map[string]any{
		"provider": "gmail", "address": salesMailbox, "reader_team_id": s.e.Team1,
		"gmail": map[string]any{"service_account_key": serviceAccountKey(t)},
	}
	rec := s.call(s.admin(), t, http.MethodPost, "/shared-mailboxes", body)
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "shared_mailbox_foreign_domain") {
		t.Fatalf("a mailbox on no own domain answered %d %s, want 422 shared_mailbox_foreign_domain", rec.Code, rec.Body)
	}
	if grants, _ := google.seen(); len(grants) != 0 {
		t.Fatal("a foreign mailbox reached Google")
	}
	s.e.WsExec(t, `INSERT INTO workspace_email_domain (domain, source, verified) VALUES ('authz.test', 'admin', true)`)
	box := s.connectAs(t, body)
	s.sync(t, box)
	if got := s.e.WsCount(t, `SELECT count(*) FROM capture_import WHERE connection_id = $1`, box); got != 1 {
		t.Fatalf("the shared mailbox captured %d messages, want the order mail", got)
	}
	if grants, reads := google.seen(); len(grants) == 0 || grants[0] != googlejwt.JWTBearerGrant || reads < 2 {
		t.Errorf("grants %v, profile reads %d; want delegated grants and a profile read by the probe and the sync", grants, reads)
	}
}

func serviceAccountKey(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	file, err := json.Marshal(map[string]string{
		"type": "service_account", "client_email": "reader@example-project.iam.gserviceaccount.com",
		"private_key": string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})),
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(file)
}
