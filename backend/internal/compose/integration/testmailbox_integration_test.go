// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The test_mailbox connector over the real composition: connect through the
// real endpoint, send through the real dispatcher, echo back through the
// real registry sync, and confirm the echo reconciles onto the send's own
// activity instead of duplicating it. Finally, disconnect and confirm a
// further send is refused, and confirm the connect endpoint 422s when
// AllowTestMailbox is unset.
//
// This connector never dials a real network, so — unlike
// comms_send_integration_test.go, which must stub an HTTP Gmail because a
// real one is unreachable in a test — there is nothing here a stub buys:
// dispatch resolves the sending connection through a real
// *capture.Registry's SenderFor (testMailboxResolver only carries the
// commsResolver-shaped error translation compose.commsResolver applies,
// unexported and so not importable from this package), and the echo runs
// through that same registry's SyncOnce against the connection id connect
// itself returned.

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/comms"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// setupTestMailboxEnv boots the composition with test_mailbox armed. Its own
// setup rather than setupPreflightIn's, because WithCaptureConfig must be
// applied BEFORE WithKeyvault for s.captureConfig.AllowTestMailbox to be set
// when WithKeyvault's registry construction reads it — an ordering
// setupPreflightIn's own opts assembly does not give an extra option (see
// compose/capture.go's WithCaptureConfig doc comment).
func setupTestMailboxEnv(t *testing.T) *preflightEnv {
	t.Helper()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("generating a test root key: %v", err)
	}
	vaultPool := apptest.EarlyPool(t)
	vault, err := keyvault.New(keyvault.Config{RootKey: key, Pool: vaultPool})
	if err != nil {
		t.Fatalf("building the local vault: %v", err)
	}
	e := apptest.SetupAppWithOptions(
		t,
		compose.WithCaptureConfig(compose.CaptureConfig{AllowTestMailbox: true}),
		compose.WithKeyvault(vault),
		compose.WithOperatorMail(discardingMailer{}),
		compose.WithPublicBaseURL(preflightBaseURL),
	)
	e.Vault = vault
	apptest.BootstrapWorkspaceSession(t, e, "Test Mailbox E2E", "sender@fable.test", "Admin")

	var person struct {
		ID string `json:"id"`
	}
	if status := e.Call(t, "POST", "/v1/people", AnyMap{
		"full_name": "Consented Buyer",
		"emails":    []AnyMap{{"email": "buyer@preflight.test"}},
	}, nil, &person); status != http.StatusCreated {
		t.Fatalf("create person → %d", status)
	}
	var activity struct {
		ID string `json:"id"`
	}
	if status := e.Call(t, "POST", "/v1/activities", AnyMap{
		"kind": "email", "subject": "Inbound question", "direction": "inbound",
		"links": []AnyMap{{"entity_type": "person", "entity_id": person.ID}},
	}, nil, &activity); status != http.StatusCreated {
		t.Fatalf("log anchor activity → %d", status)
	}

	var purposes struct {
		Data []struct {
			ID  string `json:"id"`
			Key string `json:"key"`
		} `json:"data"`
	}
	if status := e.Call(t, "GET", "/v1/consent-purposes", nil, nil, &purposes); status != http.StatusOK {
		t.Fatalf("list purposes → %d", status)
	}
	var transactional string
	for _, p := range purposes.Data {
		if p.Key == "transactional" {
			transactional = p.ID
		}
	}
	if transactional == "" {
		t.Fatalf("bootstrap seeded no transactional purpose: %+v", purposes.Data)
	}
	if status := e.Call(t, "POST", "/v1/people/"+person.ID+"/consent", AnyMap{
		"purpose_id": transactional, "new_state": "granted", "lawful_basis": "consent",
		"wording": "Yes, you may contact me about this.",
	}, nil, nil); status != http.StatusOK {
		t.Fatalf("record consent → %d", status)
	}

	var ws, user string
	if err := apptest.InWorkspace(e, t, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT (SELECT id FROM workspace ORDER BY created_at LIMIT 1), id FROM app_user WHERE email = $1`, "sender@fable.test").Scan(&ws, &user)
	}); err != nil {
		t.Fatalf("resolving the acting human: %v", err)
	}
	return &preflightEnv{AppEnv: e, activityID: activity.ID, personID: person.ID, ws: ws, user: user}
}

// connectTestMailbox drives the REAL connect endpoint and returns the created
// connection's id.
func (p *preflightEnv) connectTestMailbox(t *testing.T) (status int, connectionID string) {
	t.Helper()
	var resp struct {
		Connection struct {
			ID string `json:"id"`
		} `json:"connection"`
	}
	status = p.Call(t, "POST", "/v1/connectors/test_mailbox/connect", AnyMap{}, nil, &resp)
	return status, resp.Connection.ID
}

// captureRegistry builds a *capture.Registry against this env's own pool and
// vault, with test_mailbox armed — the same construction compose.WithKeyvault
// runs at boot (compose/capture.go's NewCaptureRegistry), so SenderFor and
// SyncOnce here resolve credentials, connections and scopes exactly as the
// composed server does.
func (p *preflightEnv) captureRegistry() *capture.Registry {
	return compose.NewCaptureRegistry(p.Pool, p.Vault, compose.CaptureConfig{AllowTestMailbox: true})
}

// testMailboxResolver is comms.ConnectionResolver for test_mailbox, backed by
// a real *capture.Registry's SenderFor rather than a canned answer — this
// connector never touches the network, so unlike the Gmail suite's
// stubMailbox (which must stand in for an unreachable HTTP Gmail) there is
// nothing here a stub buys. The error translation mirrors
// compose.commsResolver's, unimportable from this package because it is
// unexported: only capture's three deployment-fact sentinels become comms
// parking sentinels, everything else stays a transient failure.
type testMailboxResolver struct {
	registry *capture.Registry
}

var _ comms.ConnectionResolver = testMailboxResolver{}

func (m testMailboxResolver) Resolve(ctx context.Context, userID ids.UserID, provider string) (connector.EmailSender, connector.Auth, []string, error) {
	sender, auth, granted, err := m.registry.SenderFor(ctx, userID, provider)
	switch {
	case errors.Is(err, capture.ErrNoConnection):
		return nil, nil, nil, fmt.Errorf("%w: %w", comms.ErrNoMailbox, err)
	case errors.Is(err, capture.ErrConnectorCannotSend):
		return nil, nil, nil, fmt.Errorf("%w: %w", comms.ErrCannotSend, err)
	case errors.Is(err, capture.ErrConnectorNotConfigured):
		return nil, nil, nil, fmt.Errorf("%w: %w", comms.ErrProviderNotConfigured, err)
	case err != nil:
		return nil, nil, nil, err
	}
	return sender, auth, granted, nil
}

func (m testMailboxResolver) ResolveChannel(context.Context, ids.UserID, string) (connector.MessageSender, connector.Auth, error) {
	return nil, nil, errors.New("testMailboxResolver: this suite stages mail deliveries only; a channel resolve here is a shape-branch defect")
}

// dispatchTestMailboxOnce drives one real dispatch through the production
// store/gate/dispatcher, resolving through a real *capture.Registry — no stub
// server needed, since this connector never touches the network.
func (p *preflightEnv) dispatchTestMailboxOnce(t *testing.T, deliveryID ids.UUID) comms.Outcome {
	t.Helper()
	db := compose.InstallationDB(p.Pool)
	dispatcher := comms.NewDispatcher(
		comms.NewStore(db, time.Now, activities.NewStore(db)),
		testMailboxResolver{registry: p.captureRegistry()},
		compose.NewSendSeatAuthority(p.Pool),
		compose.NewSendAttachmentAuthority(p.Pool, nil),
		consent.NewGate(consent.NewStore(db)),
		nil, time.Now, 24*time.Hour, 10,
	)
	outcome, _, err := dispatcher.DispatchWithWait(
		compose.SendWorkerContext(context.Background(), p.workspaceID(t)), deliveryID,
	)
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	return outcome
}

func TestTestMailboxConnectRefusedWhenTheFlagIsUnset(t *testing.T) {
	// setupPreflightWithoutGoogleApp still wires a registry (WithKeyvault
	// builds one) but never sets AllowTestMailbox, so test_mailbox is not
	// registered on it — the same 422 an unknown provider gets.
	p := setupPreflightWithoutGoogleApp(t)
	status, _ := p.connectTestMailbox(t)
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("connect status = %d, want 422 when AllowTestMailbox is unset", status)
	}
}

func TestTestMailboxFullLoop(t *testing.T) {
	p := setupTestMailboxEnv(t)

	status, connectionID := p.connectTestMailbox(t)
	if status != http.StatusOK {
		t.Fatalf("connect status = %d, want 200", status)
	}
	if connectionID == "" {
		t.Fatal("connect returned no connection id")
	}

	sentActivity := p.sendExpectingAcceptance(t, "transactional", "Re: Inbound question", "As discussed.")
	deliveryID, messageID := p.deliveryFor(t, sentActivity)

	outcome := p.dispatchTestMailboxOnce(t, deliveryID)
	if outcome != comms.OutcomeSent {
		t.Fatalf("dispatch outcome = %q, want sent", outcome)
	}

	// The send-side write: RecordSent's comms_outbound row.
	var deliveryStatus string
	if err := apptest.InWorkspace(p.AppEnv, t, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT status FROM comms_outbound WHERE id = $1`, deliveryID).Scan(&deliveryStatus)
	}); err != nil {
		t.Fatal(err)
	}
	if deliveryStatus != "sent" {
		t.Fatalf("comms_outbound.status = %q, want sent", deliveryStatus)
	}

	// The send-side ledger row exists before any sync runs — SyncOnce below
	// is what drains it, not what writes it.
	db := compose.InstallationDB(p.Pool)
	ledger := capture.NewTestMailboxLedger(db)
	userID, err := ids.Parse(p.user)
	if err != nil {
		t.Fatalf("parsing the acting human's id: %v", err)
	}
	unechoed, err := ledger.Unechoed(context.Background(), userID)
	if err != nil {
		t.Fatalf("Unechoed: %v", err)
	}
	if len(unechoed) != 1 || unechoed[0].MessageID != messageID {
		t.Fatalf("ledger.Unechoed = %+v, want one row for %q", unechoed, messageID)
	}

	connID, err := ids.Parse(connectionID)
	if err != nil {
		t.Fatalf("parsing the connection id: %v", err)
	}
	syncCtx := principal.WithWorkspaceID(context.Background(), p.workspaceID(t))
	if err := p.captureRegistry().SyncOnce(syncCtx, connID); err != nil {
		t.Fatalf("SyncOnce: %v", err)
	}

	// The reconciliation: exactly one activity carries this message's natural
	// key — the one the SEND created — not two.
	var rows int
	if err := apptest.InWorkspace(p.AppEnv, t, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT count(*) FROM activity WHERE source_system = $1 AND source_id = $2`,
			connector.EmailSourceSystem, messageID).Scan(&rows)
	}); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Fatalf("%d activities carry the sent message's natural key, want exactly 1 — the echo must reconcile, not duplicate", rows)
	}
	var survivingID ids.UUID
	if err := apptest.InWorkspace(p.AppEnv, t, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT id FROM activity WHERE source_system = $1 AND source_id = $2`,
			connector.EmailSourceSystem, messageID).Scan(&survivingID)
	}); err != nil {
		t.Fatal(err)
	}
	if survivingID != sentActivity {
		t.Errorf("the surviving activity is %s, not the one the send created (%s)", survivingID, sentActivity)
	}

	// Disconnect, then a further send is refused again.
	if status := p.Call(t, "POST", "/v1/connectors/test_mailbox/disconnect", AnyMap{}, nil, nil); status != http.StatusNoContent {
		t.Fatalf("disconnect status = %d, want 204", status)
	}
	status2, code, _ := p.send(t)
	if status2 != http.StatusUnprocessableEntity || code != "mailbox_not_send_capable" {
		t.Fatalf("send after disconnect → %d %q, want 422 mailbox_not_send_capable", status2, code)
	}
}
