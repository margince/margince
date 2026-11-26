// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A reply sent as a shared mailbox, through the one send path. The HTTP and
// MCP doors, the staging transaction, the dispatcher and the capture
// registry's credential lookup are real. Only the mail provider's network is
// faked.

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/capture/graph"
	"github.com/margince/margince/backend/internal/modules/comms"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

const salesAddress = "sales@preflight.test"

// sharedMailboxProvider is a Microsoft Graph tenant that records each message
// submitted for a mailbox.
type sharedMailboxProvider struct {
	mu   sync.Mutex
	path []string
	mime []string
	srv  *httptest.Server
}

func newSharedMailboxProvider(t *testing.T) *sharedMailboxProvider {
	t.Helper()
	p := &sharedMailboxProvider{}
	p.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/token") {
			WriteJSON(w, map[string]any{"access_token": "app-tok", "expires_in": 3599})
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("reading the submitted message: %v", err)
		}
		raw, err := base64.StdEncoding.DecodeString(string(body))
		if err != nil {
			t.Errorf("the submission is not base64 MIME: %v", err)
		}
		p.mu.Lock()
		p.path, p.mime = append(p.path, r.URL.EscapedPath()), append(p.mime, string(raw))
		p.mu.Unlock()
		w.WriteHeader(http.StatusAccepted)
	}))
	t.Cleanup(p.srv.Close)
	return p
}

func (p *sharedMailboxProvider) submissions() (paths, messages []string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.path...), append([]string(nil), p.mime...)
}

// sharedMailbox seeds a shared Microsoft mailbox, read by a new team that holds
// readers, with replies turned on. The credential is sealed in the vault the way
// a connect seals it.
func (p *preflightEnv) sharedMailbox(t *testing.T, readers ...string) ids.UUID {
	t.Helper()
	address := salesAddress
	bundle, err := graph.SealShared(graph.SharedCredential{
		TenantID: "contoso.example", ClientID: "app", ClientSecret: "s3cret", Mailbox: address,
	})
	if err != nil {
		t.Fatal(err)
	}
	ws := p.workspaceID(t)
	ref, err := p.Vault.Put(principal.WithWorkspaceID(context.Background(), ws), ids.From[ids.WorkspaceKind](ws), []byte(bundle))
	if err != nil {
		t.Fatalf("sealing the shared credential: %v", err)
	}
	team, box := ids.NewV7(), ids.NewV7()
	if err := apptest.InWorkspace(p.AppEnv, t, func(tx pgx.Tx) error {
		if _, err := tx.Exec(context.Background(), `INSERT INTO team (id, name) VALUES ($1, $2)`, team, "Desk of "+address); err != nil {
			return err
		}
		for _, reader := range readers {
			if _, err := tx.Exec(context.Background(), `INSERT INTO team_membership (team_id, user_id) VALUES ($1, $2)`, team, reader); err != nil {
				return err
			}
		}
		_, err := tx.Exec(context.Background(), `
			INSERT INTO capture_connection (id, provider, user_id, connected_by, reader_team_id, account_label, mail_posture,
			                                credential_ref, status, share_acknowledged_at, provider_scopes)
			VALUES ($1, 'graph', NULL, $2, $3, $4, 'held', $5, 'connected', now(), ARRAY[$6]::text[])`,
			box, p.user, team, address, string(ref), graph.SendScope)
		return err
	}); err != nil {
		t.Fatalf("seeding the shared mailbox: %v", err)
	}
	return box
}

// dispatchAsMailbox drives one real dispatch of a staged delivery through the
// capture registry's own credential lookup, against the fake tenant.
func (p *preflightEnv) dispatchAsMailbox(t *testing.T, provider *sharedMailboxProvider, delivery ids.UUID) comms.Outcome {
	t.Helper()
	return p.dispatchAsMailboxWith(t, provider, delivery, nil)
}

// changeAfterResolve lets a test commit a change after the credential lookup
// has answered. That is before the dispatcher's last check, in the window a
// queued reply meets in production.
type changeAfterResolve struct {
	comms.ConnectionResolver
	change func()
}

func (c changeAfterResolve) Resolve(ctx context.Context, user ids.UserID, provider string, connection *ids.UUID) (connector.EmailSender, connector.Auth, []string, error) {
	sender, auth, granted, err := c.ConnectionResolver.Resolve(ctx, user, provider, connection)
	c.change()
	return sender, auth, granted, err
}

func (c changeAfterResolve) RecheckReply(ctx context.Context, user ids.UserID, connection ids.UUID) error {
	return c.ConnectionResolver.(comms.ReplyRechecker).RecheckReply(ctx, user, connection)
}

func (p *preflightEnv) dispatchAsMailboxWith(t *testing.T, provider *sharedMailboxProvider, delivery ids.UUID, between func()) comms.Outcome {
	t.Helper()
	registry := compose.CaptureRegistryWithoutConnectorsForTest(p.Pool, p.Vault)
	registry.Register(graph.New(nil, nil).WithSharedMailboxes(graph.SharedMailboxConfig{
		Auth:       graph.SharedAuthConfig{TokenURLFormat: provider.srv.URL + "/%s/token", HTTPClient: provider.srv.Client()},
		HTTPClient: provider.srv.Client(), APIBase: provider.srv.URL,
	}))
	var resolver comms.ConnectionResolver = compose.NewSendConnectionResolver(registry)
	if between != nil {
		resolver = changeAfterResolve{ConnectionResolver: resolver, change: between}
	}
	dispatcher := comms.NewDispatcher(
		compose.NewMailDeliveryStore(p.Pool, nil),
		resolver,
		compose.NewSendSeatAuthority(p.Pool),
		compose.NewSendAttachmentAuthority(p.Pool, nil),
		consent.NewGate(consent.NewStore(compose.InstallationDB(p.Pool))),
		nil, func() time.Time { return clocktest.Now(t) }, 24*time.Hour, 10,
	)
	outcome, _, err := dispatcher.DispatchWithWait(compose.SendWorkerContext(context.Background(), p.workspaceID(t)), delivery)
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	return outcome
}

func (p *preflightEnv) sendAs(t *testing.T, box ids.UUID) (int, string) {
	t.Helper()
	var problem struct {
		Code string `json:"code"`
	}
	status := p.Call(t, "POST", "/v1/activities/"+p.activityID+"/send-email", AnyMap{
		"subject": "Re: Inbound question", "body": "Your order shipped.", "from_connection_id": box.String(),
		"to": []string{"buyer@preflight.test"}, "consent_purpose": "transactional",
	}, nil, &problem)
	return status, problem.Code
}

type auditRow struct{ actor, actorType, sentBy, from string }

func (p *preflightEnv) sendAudits(t *testing.T, box ids.UUID) []auditRow {
	t.Helper()
	var out []auditRow
	if err := apptest.InWorkspace(p.AppEnv, t, func(tx pgx.Tx) error {
		rows, err := tx.Query(context.Background(), `SELECT actor_id, actor_type, after->>'sent_by', after->>'from'
		  FROM audit_log WHERE action = 'send_email' AND entity_type = 'capture_connection' AND entity_id = $1 ORDER BY occurred_at`, box)
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (auditRow, error) {
			var a auditRow
			return a, row.Scan(&a.actor, &a.actorType, &a.sentBy, &a.from)
		})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestAReaderRepliesAsTheSharedMailboxAndTheRecordsNameBothIdentities(t *testing.T) {
	p := setupPreflight(t)
	p.stakeADeal(t)
	box := p.sharedMailbox(t, p.user)
	provider := newSharedMailboxProvider(t)

	if status, code := p.sendAs(t, box); status != http.StatusAccepted {
		t.Fatalf("a reader's send as the mailbox → %d %q, want 202", status, code)
	}
	var delivery ids.UUID
	var staged struct{ connection, provider, user string }
	if err := apptest.InWorkspace(p.AppEnv, t, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT id, connection_id::text, provider, user_id::text FROM comms_outbound`).
			Scan(&delivery, &staged.connection, &staged.provider, &staged.user)
	}); err != nil {
		t.Fatal(err)
	}
	if staged.connection != box.String() || staged.provider != "graph" || staged.user != p.user {
		t.Fatalf("the delivery names %+v, want the shared mailbox on graph sent by the pressing seat %s", staged, p.user)
	}
	audits := p.sendAudits(t, box)
	if len(audits) != 1 || audits[0].sentBy != p.user || audits[0].from != salesAddress || audits[0].actorType != "human" || audits[0].actor != "human:"+p.user {
		t.Fatalf("the mailbox's send audit rows read %+v, want one naming the seat %s and the address %s", audits, p.user, salesAddress)
	}

	if outcome := p.dispatchAsMailbox(t, provider, delivery); outcome != comms.OutcomeSent {
		t.Fatalf("dispatch outcome = %q, want sent", outcome)
	}
	paths, messages := provider.submissions()
	if len(paths) != 1 || paths[0] != "/users/"+salesAddress+"/sendMail" {
		t.Fatalf("the tenant received %v, want one sendMail under the shared mailbox", paths)
	}
	headers, _, _ := strings.Cut(messages[0], "\r\n\r\n")
	if !strings.Contains(headers, "<"+salesAddress+">") || strings.Contains(headers, "sender@fable.test") {
		t.Errorf("the From header must carry the shared address and no one's personal one:\n%s", headers)
	}
}

func TestASeatOutsideTheReaderTeamCannotReplyAsTheMailbox(t *testing.T) {
	p := setupPreflight(t)
	p.stakeADeal(t)
	p.connect(t, apptest.GmailReadonlyScope, apptest.GmailSendScope)
	box := p.sharedMailbox(t)

	status, code := p.sendAs(t, box)
	if status != http.StatusUnprocessableEntity || code != "mailbox_not_send_capable" {
		t.Fatalf("a seat outside the team sending as the mailbox → %d %q, want 422 mailbox_not_send_capable", status, code)
	}
	if n := p.stagedDeliveries(t); n != 0 {
		t.Fatalf("%d deliveries staged behind a refused send, want 0", n)
	}
	if got := p.sendAudits(t, box); len(got) != 0 {
		t.Fatalf("a refused send left audit rows %+v", got)
	}
}

func TestAReplyIsNotSentOnceTheSeatLeftTheTeam(t *testing.T) {
	p := setupPreflight(t)
	p.stakeADeal(t)
	box := p.sharedMailbox(t, p.user)
	provider := newSharedMailboxProvider(t)
	if status, code := p.sendAs(t, box); status != http.StatusAccepted {
		t.Fatalf("a reader's send as the mailbox → %d %q, want 202", status, code)
	}
	var delivery ids.UUID
	if err := apptest.InWorkspace(p.AppEnv, t, func(tx pgx.Tx) error {
		if err := tx.QueryRow(context.Background(), `SELECT id FROM comms_outbound`).Scan(&delivery); err != nil {
			return err
		}
		_, err := tx.Exec(context.Background(), `DELETE FROM team_membership WHERE user_id = $1`, p.user)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	if outcome := p.dispatchAsMailbox(t, provider, delivery); outcome != comms.OutcomeParked {
		t.Fatalf("dispatch outcome = %q, want parked", outcome)
	}
	if paths, _ := provider.submissions(); len(paths) != 0 {
		t.Fatalf("the tenant received %v after the seat left the team, want nothing", paths)
	}
}

func TestASuppressedRecipientIsRefusedWhenTheReplyGoesOutAsTheMailbox(t *testing.T) {
	p := setupPreflight(t)
	p.stakeADeal(t)
	box := p.sharedMailbox(t, p.user)
	if status := p.Call(t, "POST", "/v1/contacts/"+p.contactID+"/consent/suppress", AnyMap{
		"kind": "subject_request", "reason": "The recipient asked us to stop.",
	}, nil, nil); status != http.StatusNoContent {
		t.Fatalf("recording the stop request → %d", status)
	}
	if status, _ := p.sendAs(t, box); status != http.StatusConflict {
		t.Fatalf("a reply as the mailbox to a suppressed recipient → %d, want 409 from the consent gate", status)
	}
	if n := p.stagedDeliveries(t); n != 0 {
		t.Fatalf("%d deliveries staged for a suppressed recipient, want 0", n)
	}
}

// An agent's send tools take no mailbox argument, so an agent never names the
// shared mailbox. Nothing it sends is chosen onto one by default.
func TestAnAgentCannotNameTheSharedMailboxAndItsOwnSendsStayOnItsPersonsMailbox(t *testing.T) {
	a := accountSendOn(t, setupPreflightServingMCP(t))
	box := a.sharedMailbox(t, a.user)
	agent := apptest.NewMCPClient(a.AppEnv, apptest.MCPBearerToken(t, a.AppEnv, "outreach agent", "read", "write", "send"))
	body := accountSendBody(a.company, a.deal, "Hello from the desk")
	body["as"], body["from_connection_id"] = "email_new", box.String()

	if got := agent.Call(t, "send_outbound", body); !got.IsError || !strings.Contains(got.Text, "from_connection_id") {
		t.Fatalf("an agent naming the shared mailbox got %+v, want the argument refused", got)
	}
	if n := a.deliveryCount(t); n != 0 {
		t.Fatalf("%d deliveries after the refused agent send, want 0", n)
	}

	delete(body, "from_connection_id")
	if got := agent.Call(t, "send_outbound", body); got.IsError {
		t.Fatalf("the agent's ordinary send was refused: %s", got.Text)
	}
	var connection *string
	if err := apptest.InWorkspace(a.AppEnv, t, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT connection_id::text FROM comms_outbound`).Scan(&connection)
	}); err != nil {
		t.Fatal(err)
	}
	if connection == nil || *connection == box.String() {
		t.Fatalf("the agent's send went out through %v, want its person's own mailbox", connection)
	}
	if got := a.sendAudits(t, box); len(got) != 0 {
		t.Fatalf("the shared mailbox has send audit rows %+v after an agent send that never named it", got)
	}
}

// A change can commit after the credential lookup and before the provider call.
// It must still stop the reply, so the dispatcher asks again at the last moment.
func TestAChangeCommittedBetweenTheLookupAndTheProviderCallParksTheReply(t *testing.T) {
	for name, change := range map[string]string{
		"the seat leaves the team": `DELETE FROM team_membership WHERE user_id = $1`,
		"replies are turned off":   `UPDATE capture_connection SET provider_scopes = '{}' WHERE user_id IS NULL AND $1::uuid IS NOT NULL`,
		"the mailbox is archived":  `UPDATE capture_connection SET archived_at = now() WHERE user_id IS NULL AND $1::uuid IS NOT NULL`,
	} {
		t.Run(name, func(t *testing.T) {
			p := setupPreflight(t)
			p.stakeADeal(t)
			box := p.sharedMailbox(t, p.user)
			provider := newSharedMailboxProvider(t)
			if status, code := p.sendAs(t, box); status != http.StatusAccepted {
				t.Fatalf("a reader's send as the mailbox → %d %q, want 202", status, code)
			}
			var delivery ids.UUID
			if err := apptest.InWorkspace(p.AppEnv, t, func(tx pgx.Tx) error {
				return tx.QueryRow(context.Background(), `SELECT id FROM comms_outbound`).Scan(&delivery)
			}); err != nil {
				t.Fatal(err)
			}
			committed := func() {
				if err := apptest.InWorkspace(p.AppEnv, t, func(tx pgx.Tx) error {
					_, err := tx.Exec(context.Background(), change, p.user)
					return err
				}); err != nil {
					t.Errorf("committing the change: %v", err)
				}
			}

			if outcome := p.dispatchAsMailboxWith(t, provider, delivery, committed); outcome != comms.OutcomeParked {
				t.Fatalf("dispatch outcome = %q, want parked", outcome)
			}
			if paths, _ := provider.submissions(); len(paths) != 0 {
				t.Fatalf("the tenant received %v after %s, want nothing", paths, name)
			}
		})
	}
}
