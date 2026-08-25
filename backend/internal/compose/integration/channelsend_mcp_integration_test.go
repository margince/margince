// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The send_message tool's own loop, end to end over the real database: an
// MCP tools/call → the 🟡 refusal STAGES an approval instead of dead-ending
// (agents.Registry.Invoke's staging branch, registry.go:126-150) → a human
// approves it → the retry carrying approval_id is the ONLY call that reaches
// Handle and performs the send.
//
// channelsend_integration_test.go proves the REST twin of this refusal, but
// that is a DIFFERENT code path (compose/agentgate.go's stageRefusal via
// canonicalRESTCall) that predates this branch and exercises neither
// sendMessageTool.StageInfo nor Invoke's staging branch. This file is the
// one proof of the MCP loop.
//
// It drives Registry.Invoke directly rather than the wire transport: the
// JSON-RPC framing, discovery and admission machinery is already proven end
// to end by mcp_transport_integration_test.go against a different tool, and
// Invoke is the one call every transport — stdio, hosted MCP, Surface B —
// dispatches through (registry.go's package comment). What was missing is
// the send_message-specific stage → approve → redeem loop against real
// Postgres, which is what this test drives.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/gradionhq/margince/backend/internal/compose"
	"github.com/gradionhq/margince/backend/internal/compose/integration/apptest"
	"github.com/gradionhq/margince/backend/internal/modules/identity"
	"github.com/gradionhq/margince/backend/internal/platform/jobs"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/principal"
)

// sendMessageInvoker builds the SAME governed registry the api role
// composes — including a REAL delivery machinery, so a redeemed send lands
// in comms_outbound instead of refusing on a nil stager — and returns an
// Invoke closure that re-authenticates the passport per call, exactly as
// every transport does (registry.go's Invoke doc: "There is no other path
// to a Handle in this package").
// sendMessageInvoker calls send_message. Kept as its own name because the
// send-specific tests below are about that verb and nothing else.
func (c *channelSendEnv) sendMessageInvoker(t *testing.T, agentToken string) func(args string) (string, error) {
	t.Helper()
	return c.verbInvoker(t, agentToken, "send_message")
}

// enrichTarget is a company for the approval-mechanism tests to name, created
// on demand so the send tests carry none of it.
func (c *channelSendEnv) enrichTarget(t *testing.T) string {
	t.Helper()
	var org struct {
		ID string `json:"id"`
	}
	if status := c.Call(t, "POST", "/v1/organizations",
		apptest.AnyMap{"display_name": "Approval Mechanism GmbH"}, nil, &org); status != http.StatusCreated {
		t.Fatalf("create organization → %d", status)
	}
	return org.ID
}

// enrichArgs is one enrich call, and the same call every time it is asked for:
// the mechanism tests re-issue an identical call to prove one approval is
// collected however often it is retried.
func enrichArgs(orgID string) string {
	return fmt.Sprintf(`{"organization_id":%q}`, orgID)
}

func enrichRetry(orgID, approvalID string) string {
	return fmt.Sprintf(`{"organization_id":%q,"approval_id":%q}`, orgID, approvalID)
}

// enrichInvoker calls the verb that still stages by default.
//
// The approval MECHANISM — one row per refused call, single-use redemption,
// one passport's approval never offered to another — is what the tests using
// this are about, and it needs some confirm-first verb to exercise. It used to
// be send_message, until a passport stopped needing a second confirmation from
// the person who granted it. `enrich` stays confirm-first for a different
// reason (the model names the URL the server fetches), which makes it the verb
// that still puts a call in front of a human.
func (c *channelSendEnv) enrichInvoker(t *testing.T, agentToken string) func(args string) (string, error) {
	t.Helper()
	return c.verbInvoker(t, agentToken, "enrich")
}

func (c *channelSendEnv) verbInvoker(t *testing.T, agentToken, verb string) func(args string) (string, error) {
	t.Helper()
	ApplyRiverSchema(t)
	inserter, err := jobs.NewInserter(c.Pool, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("jobs.NewInserter: %v", err)
	}
	registry := compose.NewRegistry(c.Pool, compose.SendPath{
		Delivery: compose.NewDeliveryStager(c.Pool, inserter),
	})
	authSvc := identity.NewService(c.Pool)
	return func(args string) (string, error) {
		wsID, err := authSvc.InstallationWorkspace(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		ctx := principal.WithWorkspaceID(context.Background(), wsID.UUID)
		agent, err := authSvc.AuthenticateAgent(ctx, agentToken)
		if err != nil {
			t.Fatal(err)
		}
		ctx = principal.WithCorrelationID(principal.WithActor(ctx, agent.Principal()), ids.NewV7())
		out, invokeErr := registry.Invoke(ctx, verb, json.RawMessage(args))
		return string(out), invokeErr
	}
}

// TestSendMessageMCPLoopSendsOnASendScopedPassportAgainstRealPostgres proves
// the loop end to end over MCP against real Postgres: a send-scoped agent's
// tools/call reaches Handle and produces the outbound activity plus its staged
// delivery, anchored on the conversation it answers.
//
// It used to stage first, and the staging half is what changed: a passport
// carries the granting human's own seat and row scope, and `send` is a cap that
// human chose to lend, so a second confirmation from the same person bought
// nothing. What still bounds the call is the cap — a passport never granted
// `send` cannot reach this at all (TestSendMessageRefusesAPassportWithoutTheSendCap).
func TestSendMessageMCPLoopSendsOnASendScopedPassportAgainstRealPostgres(t *testing.T) {
	c := setupChannelSend(t)
	token := c.mintPassport(t, []string{"read", "send"})
	invoke := c.sendMessageInvoker(t, token)

	args := fmt.Sprintf(`{"activity_id":%q,"body":"Yes — shipping Monday.","consent_purpose":"transactional"}`, c.activityID)

	out, err := invoke(args)
	if err != nil {
		t.Fatalf("send_message on a send-scoped passport → %v, want it to reach Handle and send", err)
	}
	var sent struct {
		ActivityID string `json:"activity_id"`
		Status     string `json:"status"`
	}
	if err := json.Unmarshal(ToolPayload(t, json.RawMessage(out)), &sent); err != nil {
		t.Fatalf("send_message result does not decode: %v (%s)", err, out)
	}
	if sent.Status != "accepted" {
		t.Fatalf("send_message result = %+v, want status accepted", sent)
	}

	// The delivery names THIS activity (channelsend.go's outboundChannelMessage
	// doc), so the id the tool call handed back must be the same id the staged
	// delivery anchors on — otherwise the response could name any activity while
	// the delivery transmits a different one.
	var deliveryActivity string
	if err := apptest.InWorkspace(c.AppEnv, t, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT activity_id::text FROM comms_outbound WHERE channel_user_id IS NOT NULL`).Scan(&deliveryActivity)
	}); err != nil {
		t.Fatalf("reading the staged delivery's activity: %v", err)
	}
	if deliveryActivity != sent.ActivityID {
		t.Fatalf("the staged delivery anchors activity %s, want the one the response named (%s)", deliveryActivity, sent.ActivityID)
	}

	if n := c.stagedChannelDeliveries(t); n != 1 {
		t.Fatalf("%d channel deliveries staged after the approved retry, want 1", n)
	}
	if n := c.outboundActivities(t); n != 1 {
		t.Fatalf("%d outbound activities logged after the approved retry, want 1", n)
	}

}
