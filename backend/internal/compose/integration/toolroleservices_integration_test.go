// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"regexp"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/integrations"
	"github.com/margince/margince/backend/internal/platform/keyvault"
)

// toolRoleApp is the sweep's app, with the services an installation runs and
// the test stack leaves off. Each is the fake another integration test starts,
// so a call the installation serves reaches its gate here too. The job queue
// only inserts, and the Google calendar answers from memory.
func toolRoleApp(t *testing.T, slug string, more ...compose.Option) *twoDoors {
	t.Helper()
	jobs, embeddings := insertOnlyRunner(t), embedReindexRouter(t, "tool-role-embed")
	models, err := compose.NewLocalModelPath(ai.FakeRoutingConfig(), ai.WithFakeClient(ai.NewFakeClient()), ai.WithoutResultCache())
	if err != nil {
		t.Fatalf("starting the fake model path: %v", err)
	}
	// The site read's model answers a question about the read in words, as the
	// read's conversation expects; one answer for each call the sweep makes.
	siteReplies := slices.Repeat([]string{`{"kind":"answer","message":"Role sweep.","proposed_changes":[],"source_ids":[]}`}, 64)
	siteReads, err := compose.NewLocalModelPath(ai.FakeRoutingConfig(), ai.WithFakeClient(ai.NewFakeClient().Script(siteReplies...)), ai.WithoutResultCache())
	if err != nil {
		t.Fatalf("starting the site read's fake model path: %v", err)
	}
	providers, err := integrations.NewRegistry(integrations.NewOfflineProvider(0, time.Now))
	if err != nil {
		t.Fatalf("registering the offline data provider: %v", err)
	}
	vault := keyvault.NewMemory()
	calendar := &bookingProviderTransport{fallback: http.DefaultTransport, calendar: "ada@example.com", events: map[string]json.RawMessage{}}
	previous := http.DefaultTransport
	http.DefaultTransport = calendar
	t.Cleanup(func() { http.DefaultTransport = previous })
	d := newTwoDoorsWith(t, slug, append([]compose.Option{
		compose.WithKeyvault(vault), compose.WithGmailCapture(compose.GmailConfig{
			ClientID: "calendar-fixture", ClientSecret: "calendar-fixture",
			StateKey: "0123456789abcdef0123456789abcdef", PublicBaseURL: "https://mail.example.test",
		}, compose.CaptureConfig{}),
		compose.WithTranscriptRead(jobs), compose.WithDocumentRead(jobs), compose.WithDeepRead(jobs, siteReads.ColdStart),
		compose.WithRoleProposals(models.ProposeRoles), compose.WithEmbedReindex(embeddings, jobs),
		compose.WithAssuranceRun(jobs), compose.WithKnowledgeIngest(jobs), compose.WithCaptureTrace(true),
		compose.WithChannelSurface(), compose.WithListsEnabled(true), compose.WithVatChecking(jobs),
		compose.WithProvider(providers, vault, jobs),
	}, more...), "read", "write", "send", "draft", "enrich")
	// The VAT queue binds the stores compose builds anywhere in the process.
	t.Cleanup(func() { compose.BindVatChecking(nil) })
	// Boot binds the embedding model an installation runs; the harness does not boot.
	seedRouterBinding(t, d.e, embeddings)
	d.e.Vault = vault
	d.e.DescribeCompany(t)
	enableBookingPage(t, d.e)
	sendCapableMailbox(t, d.e)
	return d
}

// sendCapableMailbox grants the connected Google mailbox the send scope, as a
// user who allowed sending leaves it; the calendar connect grants only its own.
func sendCapableMailbox(t *testing.T, e *apptest.AppEnv) {
	t.Helper()
	if err := apptest.InWorkspace(e, t, func(tx pgx.Tx) error {
		_, err := tx.Exec(t.Context(), `
			INSERT INTO capture_connection (provider, user_id, scopes, status, auth, provider_scopes)
			VALUES ('gmail', $1, '{}', 'connected', $2, $3)
			ON CONFLICT (user_id, provider) DO UPDATE SET status = 'connected',
				provider_scopes = capture_connection.provider_scopes || EXCLUDED.provider_scopes`,
			sessionUserID(t, e), []byte(`{"refresh_token":"r","granted":[]}`), []string{gmailReadonlyScope, gmailSendScope})
		return err
	}); err != nil {
		t.Fatalf("granting the mailbox its send scope: %v", err)
	}
}

// stagedApprovalID finds the approval a confirm-first answer names, which the
// call repeats with once a human has released it.
var stagedApprovalID = regexp.MustCompile(`repeats with "approval_id": "([0-9a-f-]{36})"`)

// callThroughApproval calls a tool, and when the answer is a confirm-first
// stage, releases it as the passport's human and repeats the call with the
// approval. So a governed tool is measured where it runs, behind its gate,
// rather than where it waits.
//
//craft:ignore naked-any an argument takes whichever JSON type its verb's schema names
func callThroughApproval(t *testing.T, e *apptest.AppEnv, client *apptest.MCPClient, verb string, args map[string]any) apptest.MCPResult {
	t.Helper()
	got := client.Call(t, verb, args)
	staged := stagedApprovalID.FindStringSubmatch(got.Text)
	if !got.IsError || staged == nil {
		return got
	}
	var refusal json.RawMessage
	switch status := e.Call(t, "POST", "/v1/approvals/"+staged[1]+"/approve", AnyMap{}, nil, &refusal); status {
	case http.StatusOK:
	case http.StatusForbidden:
		// The human may not release the call, so the role cannot run it.
		return apptest.MCPResult{IsError: true, Text: "releasing the staged call: permission denied " + string(refusal)}
	default:
		return got
	}
	released := map[string]any{"approval_id": staged[1]}
	for name, value := range args {
		released[name] = value
	}
	return client.Call(t, verb, released)
}
