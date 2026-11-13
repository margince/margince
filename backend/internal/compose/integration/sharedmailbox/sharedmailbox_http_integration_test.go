// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package sharedmailbox

// A shared mailbox end to end. An admin connects it over /v1/shared-mailboxes.
// One mail reaches it and a rep's own mailbox. Each reader finds what they
// should on the contact timeline and through an agent's read_record.

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/imap"
	"github.com/margince/margince/backend/internal/modules/capture/mailmap"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

const (
	salesMailbox = "sales@authz.test"
	// repMailbox is Rep3's own address in the harness.
	repMailbox = "c@authz.test"
	// rejectedSecret is the one secret the fake login check turns away.
	rejectedSecret = "wrong"
	orderBody      = "Please ship order 42 to the new warehouse."
)

// deliveringMailbox is a mail provider with no network: each sync hands the
// sink the raws it holds, parsed for the mailbox address.
type deliveringMailbox struct {
	name, address string
	raws          [][]byte
}

func (m *deliveringMailbox) Descriptor() connector.Descriptor {
	return connector.Descriptor{
		Name: m.name, Version: "1", Scopes: []principal.Scope{principal.ScopeRead},
		RiskTier: mcp.TierAutoExecute, Produces: []datasource.EntityType{datasource.EntityActivity},
	}
}

func (m *deliveringMailbox) Authenticate(context.Context, connector.AuthRequest) (connector.Auth, error) {
	return connector.Auth("token"), nil
}

func (m *deliveringMailbox) Sync(ctx context.Context, _ connector.Auth, cursor connector.Cursor, sink connector.Sink) (connector.Cursor, error) {
	for _, raw := range m.raws {
		msg, err := mailmap.Parse(raw, m.address)
		if err != nil {
			return nil, err
		}
		if _, err := sink.Upsert(ctx, msg.ToRecord(m.name, raw)); err != nil && !errors.Is(err, connector.ErrSkip) {
			return nil, err
		}
	}
	return cursor, nil
}

func (m *deliveringMailbox) Normalize(context.Context, connector.RawRecord) ([]connector.NormalizedRecord, error) {
	return nil, connector.ErrSkip
}

func (m *deliveringMailbox) HealthCheck(context.Context, connector.Auth) error { return nil }

// AccountLabel names the mailbox, which is how capture knows whose address
// proves a delivery.
func (m *deliveringMailbox) AccountLabel(connector.Auth) (string, error) { return m.address, nil }

// loginCheck stands in for the IMAP server's answer to a login.
func loginCheck(_ context.Context, req connector.AuthRequest) (connector.Auth, error) {
	var creds imap.Credentials
	if err := json.Unmarshal(req.Payload, &creds); err != nil {
		return nil, err
	}
	if creds.Password == rejectedSecret {
		return nil, imap.ErrLoginRejected
	}
	return connector.Auth(req.Payload), nil
}

type sharedMailboxSuite struct {
	e        *integration.Env
	registry *capture.Registry
	vault    keyvault.Vault
	shared   *deliveringMailbox
	personal *deliveringMailbox
	mux      http.Handler
	anna     ids.UUID
}

func newSharedMailboxSuite(t *testing.T) *sharedMailboxSuite {
	t.Helper()
	e := integration.Setup(t)
	// The rep's own connect, and every agent read below, ask the seat's live
	// role, so each reader holds one.
	e.GrantRole(t, e.Rep3, "rep")
	e.GrantRole(t, e.Rep2, "rep")
	// The production registry's parts, with network-free mail providers in
	// place of the standing IMAP one.
	vault := testVault(t, e)
	registry := compose.CaptureRegistryWithoutConnectorsForTest(e.Pool, vault)
	s := &sharedMailboxSuite{
		e: e, registry: registry, vault: vault,
		shared:   &deliveringMailbox{name: "imap", address: salesMailbox},
		personal: &deliveringMailbox{name: "gmail", address: repMailbox},
	}
	registry.Register(s.shared)
	registry.Register(s.personal)
	s.mux = compose.SharedMailboxRouterForTest(e.Pool, registry, loginCheck)
	anna, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{
		FullName: "Anna Acme", Source: "manual", Emails: []contacts.ContactEmailInput{{Email: "anna@acme.example", EmailType: "work", IsPrimary: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	s.anna = ids.UUID(anna.Id)
	return s
}

// testVault is a key vault under a fresh root key, over the harness pool.
func testVault(t *testing.T, e *integration.Env) keyvault.Vault {
	t.Helper()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	vault, err := keyvault.New(keyvault.Config{RootKey: key, Pool: e.Pool})
	if err != nil {
		t.Fatal(err)
	}
	return vault
}

func (s *sharedMailboxSuite) call(ctx context.Context, t *testing.T, method, path string, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	var payload bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&payload).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, "/v1"+path, &payload).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	return rec
}

// adminIdentity is the harness admin as identity's team writes take it.
func adminIdentity(e *integration.Env) identity.Identity {
	return identity.Identity{
		UserID: ids.From[ids.UserKind](e.AdminUser), WorkspaceID: ids.From[ids.WorkspaceKind](e.WS),
		SeatType: "full", Roles: []string{"admin"}, Permissions: integration.AdminPerms,
	}
}

func (s *sharedMailboxSuite) admin() context.Context {
	return s.e.As(s.e.AdminUser, nil, integration.AdminPerms)
}

func connectBody(team ids.UUID, secret string) map[string]any {
	return map[string]any{
		"provider": "imap", "address": salesMailbox, "reader_team_id": team,
		"imap": map[string]any{"host": "mail.authz.test", "username": salesMailbox, "secret": secret},
	}
}

// connect connects sales@ over HTTP as the admin, read by team.
func (s *sharedMailboxSuite) connect(t *testing.T, team ids.UUID) crmcontracts.SharedMailbox {
	t.Helper()
	rec := s.call(s.admin(), t, http.MethodPost, "/shared-mailboxes", connectBody(team, "app-secret"))
	if rec.Code != http.StatusOK {
		t.Fatalf("the admin's connect answered %d: %s", rec.Code, rec.Body)
	}
	var box crmcontracts.SharedMailbox
	if err := json.Unmarshal(rec.Body.Bytes(), &box); err != nil {
		t.Fatal(err)
	}
	return box
}

func (s *sharedMailboxSuite) sync(t *testing.T, connection ids.UUID) {
	t.Helper()
	if err := s.registry.SyncOnce(principal.WithWorkspaceID(context.Background(), s.e.WS), connection); err != nil {
		t.Fatalf("syncing %s: %v", connection, err)
	}
}

// connectRepMailbox connects Rep3's own mailbox as Rep3.
func (s *sharedMailboxSuite) connectRepMailbox(t *testing.T) ids.UUID {
	t.Helper()
	ctx := integration.OnSeat(s.e.As(s.e.Rep3, []ids.UUID{s.e.Team2}, integration.RepPerms), principal.SeatFull)
	rep, _ := principal.Actor(ctx)
	rep.Scopes = principal.NewScopeSet(principal.ScopeRead)
	id, err := s.registry.Connect(principal.WithActor(ctx, rep), "gmail", connector.Auth("refresh"))
	if err != nil {
		t.Fatalf("connecting the rep's mailbox: %v", err)
	}
	return id
}

func orderMail(msgID string, to ...string) []byte {
	return []byte(strings.Join([]string{
		"From: Anna Acme <anna@acme.example>", "To: " + strings.Join(to, ", "), "Subject: order 42 for the warehouse",
		"Date: Wed, 04 Jun 2026 08:00:00 +0000", "Message-ID: <" + msgID + ">",
		"Content-Type: text/plain", "", orderBody, "",
	}, "\r\n"))
}

func TestOnlyASignedInAdminConnectsASharedMailbox(t *testing.T) {
	s := newSharedMailboxSuite(t)
	e := s.e
	for who, ctx := range map[string]context.Context{
		"a rep":               e.As(e.Rep1, []ids.UUID{e.Team1}, integration.RepPerms),
		"the admin's agent":   e.AgentFor(t, e.AdminUser, nil, integration.AdminPerms),
		"a signed-out caller": principal.WithWorkspaceID(context.Background(), e.WS),
	} {
		if rec := s.call(ctx, t, http.MethodPost, "/shared-mailboxes", connectBody(e.Team1, "app-secret")); rec.Code != http.StatusForbidden {
			t.Errorf("%s connecting a shared mailbox got %d, want 403", who, rec.Code)
		}
		if rec := s.call(ctx, t, http.MethodGet, "/shared-mailboxes", nil); rec.Code != http.StatusForbidden {
			t.Errorf("%s listing the shared mailboxes got %d, want 403", who, rec.Code)
		}
	}
	if got := e.WsCount(t, `SELECT count(*) FROM capture_connection WHERE user_id IS NULL`); got != 0 {
		t.Fatalf("a refused connect left %d shared rows", got)
	}

	box := s.connect(t, e.Team1)
	if box.Address != salesMailbox || box.Status != "connected" || box.MailVisibility != "held" || ids.UUID(box.ReaderTeamId) != e.Team1 {
		t.Fatalf("the connect answered %+v, want a held sales@ read by Team1", box)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM capture_connection WHERE id = $1 AND auth IS NULL AND credential_ref IS NOT NULL AND connected_by = $2`, box.Id, e.AdminUser); got != 1 {
		t.Fatal("the credential must sit in the vault, and connected_by must name the admin")
	}
	if got := e.WsCount(t, `SELECT count(*) FROM audit_log a JOIN event_outbox o ON (o.envelope->'trace'->>'audit_log_id')::uuid = a.id
		WHERE a.entity_id = $1 AND o.envelope->>'type' = 'shared_mailbox.changed'`, box.Id); got != 1 {
		t.Fatalf("the connect wrote %d audit rows with their event, want 1", got)
	}
	var list crmcontracts.SharedMailboxList
	if rec := s.call(s.admin(), t, http.MethodGet, "/shared-mailboxes", nil); rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &list) != nil || len(list.Items) != 1 {
		t.Fatalf("the admin's list answered %d: %s", rec.Code, rec.Body)
	}
}

func TestASharedConnectRefusesWhatCouldNotBeRead(t *testing.T) {
	s := newSharedMailboxSuite(t)
	e := s.e
	empty := ids.NewV7()
	e.WsExec(t, `INSERT INTO team (id, name) VALUES ($1, 'Nobody yet')`, empty)
	archived := ids.NewV7()
	e.WsExec(t, `INSERT INTO team (id, name, archived_at) VALUES ($1, 'Gone', now())`, archived)
	e.WsExec(t, `INSERT INTO team_membership (team_id, user_id) VALUES ($1, $2)`, archived, e.Rep1)
	classified := connectBody(e.Team1, "app-secret")
	classified["mail_visibility"] = "classified"
	shared := connectBody(e.Team1, "app-secret")
	shared["mail_visibility"] = "shared"
	for name, tc := range map[string]struct {
		body map[string]any
		code string
	}{
		"a classified mailbox":           {classified, "invalid_mail_visibility"},
		"shared mail without the opt-in": {shared, "shared_posture_not_allowed"},
		"a reader team with no member":   {connectBody(empty, "app-secret"), "reader_team_has_no_member"},
		"an unknown reader team":         {connectBody(ids.NewV7(), "app-secret"), "reader_team_not_found"},
		"an archived reader team":        {connectBody(archived, "app-secret"), "reader_team_not_found"},
		"a login the server rejects":     {connectBody(e.Team1, rejectedSecret), "imap_login_rejected"},
	} {
		rec := s.call(s.admin(), t, http.MethodPost, "/shared-mailboxes", tc.body)
		if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), tc.code) {
			t.Errorf("%s answered %d %s, want 422 %s", name, rec.Code, rec.Body, tc.code)
		}
	}
	if got := e.WsCount(t, `SELECT count(*) FROM capture_connection WHERE user_id IS NULL`); got != 0 {
		t.Fatalf("a refused connect left %d shared rows", got)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM vault_secret`); got != 0 {
		t.Fatalf("a refused connect left %d sealed credentials", got)
	}
}

// sees answers whether reader finds the mail on Anna's timeline, and through
// their agent's read_record.
func (s *sharedMailboxSuite) sees(t *testing.T, reader ids.UUID, teams []ids.UUID, mail ids.UUID) (timeline, agent bool) {
	t.Helper()
	perms := principal.Permissions{
		Objects:  map[string]principal.ObjectGrant{"activity": {Read: true}, "contact": {Read: true}, "company": {Read: true}},
		RowScope: principal.RowScopeAll,
	}
	rec := s.call(s.e.As(reader, teams, perms), t, http.MethodGet, fmt.Sprintf("/activities?entity_type=contact&entity_id=%s", s.anna), nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("the timeline answered %d: %s", rec.Code, rec.Body)
	}
	passport := s.e.SeedPassportFor(t, integration.OwnerConn(t), "shared-mailbox reader", reader)
	ctx := s.e.AgentOnPassport(t, reader, teams, perms, ids.From[ids.PassportKind](passport))
	out, err := compose.NewRegistry(s.e.Pool, compose.SendPath{}).Invoke(ctx, "read_record", json.RawMessage(fmt.Sprintf(`{"record_type":"activity","id":%q}`, mail)))
	// A row the reader may not open still lists, with its content withheld,
	// so the check is on the body.
	return strings.Contains(rec.Body.String(), orderBody), err == nil && strings.Contains(string(out), orderBody)
}

// outsiderSeat is a live member in no team.
func (s *sharedMailboxSuite) outsiderSeat(t *testing.T) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	s.e.WsExec(t, `INSERT INTO app_user (id, email, display_name) VALUES ($1, 'outsider@authz.test', 'Outsider')`, id)
	s.e.GrantRole(t, id, "rep")
	return id
}

// deliverToBoth sends one mail to the rep and to sales@, and answers it. The
// rep's copy lands first and files it under Anna; shared-only mail files under
// nobody.
func (s *sharedMailboxSuite) deliverToBoth(t *testing.T, box ids.UUID) ids.UUID {
	t.Helper()
	personal := s.connectRepMailbox(t)
	raw := orderMail("order-42@acme.example", salesMailbox, repMailbox)
	s.shared.raws, s.personal.raws = [][]byte{raw}, [][]byte{raw}
	s.sync(t, personal)
	s.sync(t, box)
	if got := s.e.WsCount(t, `SELECT count(*) FROM activity WHERE kind = 'email'`); got != 1 {
		t.Fatalf("the mail landed as %d activities, want one", got)
	}
	var mail ids.UUID
	if err := integration.OwnerConn(t).QueryRow(context.Background(), `SELECT id FROM activity WHERE kind = 'email'`).Scan(&mail); err != nil {
		t.Fatal(err)
	}
	if got := s.e.WsCount(t, `SELECT count(*) FROM capture_import WHERE activity_id = $1`, mail); got != 2 {
		t.Fatalf("the mail carries %d import rows, want the shared mailbox's and the rep's", got)
	}
	return mail
}

func TestEachReaderSeesWhatTheSharedMailboxAndTheRepReceived(t *testing.T) {
	s := newSharedMailboxSuite(t)
	e := s.e
	box := s.connect(t, e.Team1)
	mail := s.deliverToBoth(t, ids.UUID(box.Id))
	outsider := s.outsiderSeat(t)

	for _, reader := range []struct {
		who   string
		seat  ids.UUID
		teams []ids.UUID
		sees  bool
	}{
		{"a member of the reader team", e.Rep2, []ids.UUID{e.Team1}, true},
		{"the rep who received it", e.Rep3, []ids.UUID{e.Team2}, true},
		{"a seat in no team", outsider, nil, false},
	} {
		timeline, agent := s.sees(t, reader.seat, reader.teams, mail)
		if timeline != reader.sees || agent != reader.sees {
			t.Errorf("%s: timeline=%v agent=%v, want %v for both", reader.who, timeline, agent, reader.sees)
		}
	}

	service := compose.TeamServiceForTest(e.Pool)
	if err := service.SetTeamMember(e.Admin(), adminIdentity(e), e.Team1, e.Rep2, false); err != nil {
		t.Fatalf("removing the member: %v", err)
	}
	if timeline, agent := s.sees(t, e.Rep2, []ids.UUID{e.Team1}, mail); timeline || agent {
		t.Fatalf("a removed member still reads the shared mail: timeline=%v agent=%v", timeline, agent)
	}
}

func TestADisconnectAndAReconnectKeepTheMailboxItsMailAndItsReaders(t *testing.T) {
	s := newSharedMailboxSuite(t)
	e := s.e
	box := s.connect(t, e.Team1)
	id := ids.UUID(box.Id)
	mail := s.deliverToBoth(t, id)

	for range 2 {
		if rec := s.call(s.admin(), t, http.MethodPost, "/shared-mailboxes/"+id.String()+"/disconnect", nil); rec.Code != http.StatusNoContent {
			t.Fatalf("the disconnect answered %d: %s", rec.Code, rec.Body)
		}
	}
	if got := e.WsCount(t, `SELECT count(*) FROM capture_connection WHERE id = $1 AND status = 'disconnected'
		AND credential_ref IS NULL AND reader_team_id = $2`, id, e.Team1); got != 1 {
		t.Fatal("a disconnected shared mailbox must keep its row and its readers, and lose its credential")
	}
	if timeline, _ := s.sees(t, e.Rep2, []ids.UUID{e.Team1}, mail); !timeline {
		t.Fatal("the reader team must still read what a disconnected mailbox captured")
	}
	// Read after the disconnect, which raises the generation itself.
	generation := e.WsCount(t, `SELECT generation::int FROM capture_connection WHERE id = $1`, id)
	if rec := s.call(s.admin(), t, http.MethodPost, "/shared-mailboxes", connectBody(e.Team2, "app-secret")); rec.Code != http.StatusConflict {
		t.Fatalf("a reconnect naming other readers answered %d, want 409", rec.Code)
	}
	again := s.connect(t, e.Team1)
	if again.Id != box.Id || again.Status != "connected" {
		t.Fatalf("the reconnect answered %+v, want connection %s connected again", again, box.Id)
	}
	if got := e.WsCount(t, `SELECT generation::int FROM capture_connection WHERE id = $1`, id); got <= generation {
		t.Fatalf("the reconnect left generation at %d (was %d), so a cycle under the old credential could still commit", got, generation)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM capture_import WHERE connection_id = $1 AND activity_id = $2`, id, mail); got != 1 {
		t.Fatal("the reconnected mailbox must keep the mail it captured")
	}
}
