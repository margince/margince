// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package toolrole

import (
	"maps"
	"net/http"
	"regexp"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/integrations"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/keyvault"
)

// toolRoleApp is the sweep's app, with the services an installation runs and
// the test stack leaves off. Each is the fake another integration test starts,
// so a call the installation serves reaches its gate here too. The job queue
// only inserts, and the Google calendar answers from memory.
func toolRoleApp(t *testing.T, slug string, more ...compose.Option) *apptest.TwoDoors {
	t.Helper()
	jobs, embeddings := apptest.InsertOnlyRunner(t), apptest.EmbedReindexRouter(t, "tool-role-embed")
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
	providers, err := integrations.NewRegistry(integrations.NewOfflineProvider(0, func() time.Time { return clocktest.Now(t) }))
	if err != nil {
		t.Fatalf("registering the offline data provider: %v", err)
	}
	vault := keyvault.NewMemory()
	calendar := apptest.NewBookingProvider(sweepGmail{next: http.DefaultTransport})
	previous := http.DefaultTransport
	http.DefaultTransport = calendar
	t.Cleanup(func() { http.DefaultTransport = previous })
	// The channel surface takes its vault from WithKeyvault, so it is composed first.
	d := apptest.SetupTwoDoors(t, slug, append([]compose.Option{
		compose.WithChannelSurface(), compose.WithKeyvault(vault), compose.WithGmailCapture(compose.GmailConfig{
			ClientID: "calendar-fixture", ClientSecret: "calendar-fixture",
			StateKey: "0123456789abcdef0123456789abcdef", PublicBaseURL: "https://mail.example.test",
		}, compose.CaptureConfig{}), compose.WithCaptureBackfill(jobs),
		compose.WithTranscriptRead(jobs), compose.WithDocumentRead(jobs), compose.WithDeepRead(jobs, siteReads.ColdStart),
		compose.WithRoleProposals(models.ProposeRoles), compose.WithEmbedReindex(embeddings, jobs),
		compose.WithAssuranceRun(jobs), compose.WithKnowledgeIngest(jobs), compose.WithCaptureTrace(true),
		compose.WithListsEnabled(true), compose.WithOperatorMail(apptest.DiscardingMailer{}),
		compose.WithFilterProposals(sweepLane{reply: sweepFilterReply}), compose.WithCorpusAsk(apptest.AskableEmbedder{}, nil, apptest.QuietAskLog()), compose.WithReportSetup(sweepLane{reply: sweepSetupReply}), compose.WithVatChecking(jobs),
		compose.WithProvider(providers, vault, jobs),
	}, more...), "read", "write", "send", "draft", "enrich")
	// The VAT queue binds the stores compose builds anywhere in the process.
	t.Cleanup(func() { compose.BindVatChecking(nil) })
	// Boot binds the embedding model an installation runs; the harness does not boot.
	apptest.SeedRouterBinding(t, d.E, embeddings)
	d.E.Vault = vault
	d.E.DescribeCompany(t)
	apptest.EnableGoogleBookingPage(t, d.E)
	sendCapableMailbox(t, d.E)
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
			apptest.SessionUserID(t, e), []byte(`{"refresh_token":"r","granted":[]}`), []string{apptest.GmailReadonlyScope, apptest.GmailSendScope})
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
// rather than where it waits. It also names what refused the release: the
// human's grants, or a row their inbox hides though the approval is there.
//
//craft:ignore naked-any an argument takes whichever JSON type its verb's schema names
func callThroughApproval(t *testing.T, e *apptest.AppEnv, client *apptest.MCPClient, verb string, args map[string]any) (apptest.MCPResult, []string) {
	t.Helper()
	got := client.Call(t, verb, args)
	staged := stagedApprovalID.FindStringSubmatch(got.Text)
	if !got.IsError || staged == nil {
		return got, nil
	}
	switch status := e.Call(t, "POST", "/v1/approvals/"+staged[1]+"/approve", integration.AnyMap{}, nil, nil); status {
	case http.StatusOK:
	case http.StatusForbidden:
		return got, []string{releaseRefusedByGrant}
	case http.StatusNotFound:
		if approvalExists(t, e, staged[1]) {
			return got, []string{releaseRefusedByRow}
		}
		return got, nil
	default:
		return got, nil
	}
	released := map[string]any{"approval_id": staged[1]}
	maps.Copy(released, args)
	return client.Call(t, verb, released), nil
}

// What refused the release of a staged call, beside the refusals the gates
// noted in the call itself.
const (
	releaseRefusedByGrant = "release:permission"
	releaseRefusedByRow   = "release:row:approval"
)

// approvalExists reads the approval past every gate, so a 404 is a hidden row
// only for an approval that is there.
func approvalExists(t *testing.T, e *apptest.AppEnv, id string) bool {
	t.Helper()
	var exists bool
	if err := e.Owner.QueryRow(t.Context(), `SELECT EXISTS (SELECT 1 FROM approval WHERE id = $1)`, id).Scan(&exists); err != nil {
		t.Fatalf("reading approval %s: %v", id, err)
	}
	return exists
}
