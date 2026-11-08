// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package toolrole

import (
	"maps"
	"net/http"
	"regexp"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/integrations"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/httpserver"
	"github.com/margince/margince/backend/internal/platform/keyvault"
)

// The statutory retention floor, armed for this binary as the composed boot
// arms it. The registry is process-global, so the parent package's arming does
// not reach here, and a pin would be refused for a floor of no length.
func init() {
	integration.RegisterGoBDFloorPack()
}

// toolRoleApp is the sweep's app, with the services an installation runs and
// the test stack leaves off. Each is the fake another integration test starts,
// so a call the installation serves reaches its gate here too. The job queue
// only inserts, and the Google calendar answers from memory.
func toolRoleApp(t *testing.T, slug string, more ...compose.Option) *apptest.TwoDoors {
	t.Helper()
	jobs, embeddings := apptest.InsertOnlyRunner(t), apptest.EmbedReindexRouter(t, "tool-role-embed")
	sweepEmbedder = embeddings
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
	// The card photo's model reads one card from each photo the sweep sends.
	cardReplies := slices.Repeat([]string{`{"cards":[{"full_name":"Role Sweep","title":"","company":"Sweep Co","emails":["card@sweep.test"],"phones":[],"url":"","address":"","confidence":0.9}]}`}, 64)
	cardReads, err := compose.NewLocalModelPath(ai.FakeRoutingConfig(), ai.WithFakeClient(ai.NewFakeClient().Script(cardReplies...)), ai.WithoutResultCache())
	if err != nil {
		t.Fatalf("starting the card photo's fake model path: %v", err)
	}
	sweepVoiceModel, err = compose.NewLocalModelPath(ai.FakeRoutingConfig(),
		ai.WithFakeClient(ai.NewFakeClient().Script(slices.Repeat([]string{voiceInferenceReply}, 512)...)), ai.WithoutResultCache())
	if err != nil {
		t.Fatalf("starting the voice build's fake model path: %v", err)
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
	// Each role's seat signs in from its own address through the loopback, so
	// no seat spends another's sign-in budget.
	loopback, err := httpserver.ParseTrustedProxies("127.0.0.1/32")
	if err != nil {
		t.Fatal(err)
	}
	// The channel surface takes its vault from WithKeyvault, so it is composed first.
	d := apptest.SetupTwoDoors(t, slug, append([]compose.Option{
		compose.WithBlobstore(sweepBlobs), compose.WithRouteChecksRecordedForTest(releases.note), compose.WithTrustedProxies(loopback),
		compose.WithCardPhotoReader(cardReads.DocumentExtract), compose.WithVoiceBuildEnqueue(jobs), compose.WithChannelSurface(), compose.WithKeyvault(vault), compose.WithGmailCapture(compose.GmailConfig{
			ClientID: "calendar-fixture", ClientSecret: "calendar-fixture",
			StateKey: "0123456789abcdef0123456789abcdef", PublicBaseURL: "https://mail.example.test",
		}, compose.CaptureConfig{}), compose.WithCaptureBackfill(jobs),
		compose.WithTranscriptRead(jobs), compose.WithDocumentRead(jobs), compose.WithDeepRead(jobs, siteReads.ColdStart),
		compose.WithRoleProposals(models.ProposeRoles), compose.WithEmbedReindex(embeddings, jobs),
		compose.WithAssuranceRun(jobs), compose.WithKnowledgeIngest(jobs), compose.WithCaptureTrace(true),
		compose.WithListsEnabled(true), compose.WithOperatorMail(apptest.DiscardingMailer{}),
		compose.WithFilterProposals(sweepLane{reply: sweepFilterReply}), compose.WithCorpusAsk(apptest.AskableEmbedder{}, nil, apptest.QuietAskLog()), compose.WithReportSetup(sweepLane{reply: sweepSetupReply}), compose.WithVatChecking(jobs),
		compose.WithProvider(providers, vault, jobs), compose.WithSchemaPool(integration.SchemaPool(t)),
		compose.WithRateRefresh(jobs), compose.WithFinanceSources(jobs),
	}, more...), "read", "write", "send", "draft", "enrich")
	// The VAT queue binds the stores compose builds anywhere in the process.
	t.Cleanup(func() { compose.BindVatChecking(nil) })
	// Boot binds the embedding model an installation runs; the harness does not boot.
	apptest.SeedRouterBinding(t, d.E, embeddings)
	d.E.Vault = vault
	seedDataTasks(t, d.E)
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
// rather than where it waits. A release a gate refused the human, as a 403 or
// a hidden row, answers what that gate noted; the stage is then the answer.
//
//craft:ignore naked-any an argument takes whichever JSON type its verb's schema names
func callThroughApproval(t *testing.T, e *apptest.AppEnv, client *apptest.MCPClient, verb string, args map[string]any) (apptest.MCPResult, []string) {
	t.Helper()
	got := client.Call(t, verb, args)
	staged := stagedApprovalID.FindStringSubmatch(got.Text)
	if !got.IsError || staged == nil {
		return got, nil
	}
	path := "/v1/approvals/" + staged[1] + "/approve"
	status := e.Call(t, "POST", path, integration.AnyMap{}, nil, nil)
	release := releases.take(path, apptest.SessionUserID(t, e))
	switch status {
	case http.StatusOK:
	case http.StatusForbidden, http.StatusNotFound:
		return got, release.Refused
	default:
		return got, nil
	}
	released := map[string]any{"approval_id": staged[1]}
	maps.Copy(released, args)
	return client.Call(t, verb, released), nil
}

// sweepEmbedder is the embedding lane of the last sweep app, which a seed's
// reindex worker embeds with.
var sweepEmbedder search.Embedder

// sweepVoiceModel is the model the last sweep app's voice builds are worked on.
var sweepVoiceModel compose.ModelPath

// sweepBlobs is the object store every sweep app keeps its files in, so a seed
// can park what a worker would have stored.
var sweepBlobs = blobstore.NewMemory()

// releases keeps what the gates noted of each human's last request to a
// route. Every sweep app records into it, and each role's human is its own.
var releases = &routeRecorder{last: map[routeKey]auth.RecordedCall{}}

type routeKey struct{ route, human string }

type routeRecorder struct {
	mu   sync.Mutex
	last map[routeKey]auth.RecordedCall
}

func (r *routeRecorder) note(method, path string, call auth.RecordedCall) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.last[routeKey{method + " " + path, call.Human}] = call
}

// take hands over what the human's last POST to path was asked, once.
func (r *routeRecorder) take(path, human string) auth.RecordedCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := routeKey{"POST " + path, human}
	call := r.last[key]
	delete(r.last, key)
	return call
}
