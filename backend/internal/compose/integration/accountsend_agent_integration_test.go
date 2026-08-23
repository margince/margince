// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The account-started send under agent authority, over the real database:
// ADR-0087 §6's whole claim, which is that this operation is governed exactly
// like the reply and gains nobody any new authority.
//
// Three facts, and none of them is observable without a database:
//
//   - a human's own send goes straight out (ADR-0055 — their action IS the
//     confirmation), which is what makes the agent's refusal below a statement
//     about the PRINCIPAL rather than about a send path that does not work;
//   - an agent's identical call stages a real approval row instead, and the
//     row it stages is the CREATE shape — the type the effect will write, no
//     target id, because this send answers no message and pins no row;
//   - a human can then see it, release it, and only then does the message
//     leave — with the outbound activity filed under the records the call
//     named, which is the one thing the account-started origin does
//     differently from the reply.
//
// It rides send_preflight's fixture: a consented recipient on file and a
// mailbox that can actually transmit, so an acceptance here is a real send and
// not a refusal in disguise.

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/gradionhq/margince/backend/internal/compose"
	"github.com/gradionhq/margince/backend/internal/compose/integration/apptest"
	"github.com/gradionhq/margince/backend/internal/modules/agents"
	"github.com/gradionhq/margince/backend/internal/modules/identity"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/principal"
)

// accountSendBody is the one call this suite makes, so the human's send and the
// agent's differ in the CREDENTIAL and in nothing else. The approved retry has
// to be the identical request — the diff hash binds it — which is a second
// reason it is built in one place.
func accountSendBody(org, subject string) apptest.AnyMap {
	return apptest.AnyMap{
		"subject": subject, "body": "Good morning — introducing ourselves.",
		"to": []string{"buyer@preflight.test"}, "consent_purpose": "transactional",
		"links": []apptest.AnyMap{{"entity_type": "organization", "entity_id": org}},
	}
}

// accountSendEnv is the pre-flight fixture plus the record an account-started
// conversation is filed under.
type accountSendEnv struct {
	*preflightEnv
	org string
}

func setupAccountSend(t *testing.T) *accountSendEnv {
	t.Helper()
	p := setupPreflight(t)
	// Without the send grant every send below refuses at the pre-flight, and
	// this suite would pass while proving nothing about authority.
	p.connect(t, gmailReadonlyScope, gmailSendScope)
	return &accountSendEnv{preflightEnv: p, org: anchorOrg(t, p.AppEnv, "Northwind")}
}

// deliveryCount is what a send did or did not do, read from the table both
// transports write: a refusal that leaves it empty refused on the tool surface
// too.
func (a *accountSendEnv) deliveryCount(t *testing.T) int {
	t.Helper()
	return a.stagedDeliveries(t)
}

// linkedActivities counts the outbound activities filed under the organization
// this suite names — the account-started origin's own effect, since a reply
// would have inherited its links from an anchor instead.
func (a *accountSendEnv) linkedActivities(t *testing.T) int {
	t.Helper()
	var n int
	if err := apptest.InWorkspace(a.AppEnv, t, a.Slug, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `
			SELECT count(*) FROM activity a
			JOIN activity_link l ON l.activity_id = a.id
			WHERE l.organization_id = $1 AND a.direction = 'outbound'`, a.org).Scan(&n)
	}); err != nil {
		t.Fatalf("counting the conversation's activities: %v", err)
	}
	return n
}

func TestAHumansAccountStartedSendLeavesWithoutAnApproval(t *testing.T) {
	a := setupAccountSend(t)

	var sent struct {
		ID string `json:"id"`
	}
	if status := a.Call(t, "POST", "/v1/emails", accountSendBody(a.org, "Hello from Fable"), nil, &sent); status != http.StatusAccepted {
		t.Fatalf("human account-started send → %d, want 202 — a human's own action is the approval", status)
	}
	if sent.ID == "" {
		t.Fatal("the accepted send named no activity")
	}
	if n := a.deliveryCount(t); n != 1 {
		t.Fatalf("%d deliveries staged behind an accepted send, want 1", n)
	}
	if n := a.linkedActivities(t); n != 1 {
		t.Fatalf("%d outbound activities filed under the named organization, want 1", n)
	}
	if n := a.pendingApprovals(t); n != 0 {
		t.Fatalf("%d approvals minted for a human's own send, want none", n)
	}
}

// pendingApprovals counts what is waiting on a human. Zero is the assertion for
// a human's own send; one is the assertion for an agent's.
func (a *accountSendEnv) pendingApprovals(t *testing.T) int {
	t.Helper()
	var n int
	if err := apptest.InWorkspace(a.AppEnv, t, a.Slug, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT count(*) FROM approval WHERE kind = 'send_account_email'`).Scan(&n)
	}); err != nil {
		t.Fatalf("counting staged approvals: %v", err)
	}
	return n
}

// The MCP door stages through the tool's own StageInfo rather than through the
// REST gate's route walk, so the two could disagree about what an
// account-started send targets — and a human would then be deciding a
// different question depending on which transport the agent used.
//
// They agree here because the REST gate takes its target from the route and
// this route has no {id}, so the id-less create is the only shape both doors
// can reach. That is a bound on the approver too — read+create on `activity`
// rather than the row scope of the records the message is filed under — and
// #928 is where the gate gains the body-derived target that would lift it.
//
// It drives Registry.Invoke directly, for the reason channelsend_mcp does: the
// JSON-RPC framing is proven elsewhere, and Invoke is the one call every
// transport dispatches through.
func TestTheMCPDoorStagesTheSameShapeAsTheRESTDoor(t *testing.T) {
	a := setupAccountSend(t)
	token := a.mintAccountSendPassport(t)

	args, err := json.Marshal(map[string]any{
		"to": []string{"buyer@preflight.test"}, "subject": "Hello over MCP",
		"body": "Good morning.", "consent_purpose": "transactional",
		"links": []map[string]string{{"entity_type": "organization", "entity_id": a.org}},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Asked of the tool's own StageInfo. The verb executes directly now, so
	// Invoke no longer stages — but the SUBJECT it would put in front of a
	// human must still match the REST door's, because a workspace tier floor
	// brings the confirm-first path back for both.
	info := a.accountSendStageInfo(t, token, args)

	if info.TargetType != "activity" {
		t.Errorf("the MCP door would stage target type %q, want the REST door's \"activity\"", info.TargetType)
	}
	if !info.TargetID.IsZero() {
		t.Errorf("the MCP door named target id %v, want none — this send answers no message, so there is "+
			"no row for an approval to pin", info.TargetID)
	}
	if info.Summary == "" {
		t.Error("the staged subject has no summary; an inbox would show a human nothing to decide")
	}
}

// accountSendStageInfo asks the tool for the subject it WOULD stage, on the
// passport that would make the call. Registry.Invoke no longer stages this
// verb, so the comparison reaches StageInfo directly — which is the same thing
// a workspace tier floor reaches.
func (a *accountSendEnv) accountSendStageInfo(t *testing.T, token string, args json.RawMessage) agents.StageInfo {
	t.Helper()
	registry := compose.NewRegistry(a.Pool, compose.SendPath{})
	authSvc := identity.NewService(a.Pool)
	wsID, err := authSvc.InstallationWorkspace(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	ctx := principal.WithWorkspaceID(t.Context(), wsID.UUID)
	agent, err := authSvc.AuthenticateAgent(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	ctx = principal.WithCorrelationID(principal.WithActor(ctx, agent.Principal()), ids.NewV7())

	stager, ok := registry.StagerFor("send_account_email")
	if !ok {
		t.Fatal("send_account_email describes no staging; a floored installation could not confirm it")
	}
	info, err := stager.StageInfo(ctx, args)
	if err != nil {
		t.Fatalf("StageInfo: %v", err)
	}
	return info
}

// mintAccountSendPassport issues the credential an outreach agent presents.
func (a *accountSendEnv) mintAccountSendPassport(t *testing.T) string {
	t.Helper()
	var minted struct {
		Token string `json:"token"`
	}
	if status := a.Call(t, "POST", "/v1/passports", apptest.AnyMap{
		"label": "outreach agent", "scopes": []string{"read", "send"},
	}, nil, &minted); status != http.StatusCreated {
		t.Fatalf("issue passport → %d", status)
	}
	return minted.Token
}
