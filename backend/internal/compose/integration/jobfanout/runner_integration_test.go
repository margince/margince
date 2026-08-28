// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package jobfanout

// The Surface-B runner end to end (architecture/07): a scheduled job
// executes the reason-act-observe loop on the offline fake brain against
// the REAL governed tool surface — same registry, same gate, same audit
// stream as every other agent surface. Covers: full run with an agent
// write and its provenance, trigger idempotency, the 🟡 suspend →
// human decision → resume handoff (both verdicts), and the loud
// no-passport failure.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/agents/runner"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/testdb"
	kevents "github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type runnerEnv struct {
	*apptest.AppEnv
	pool  *pgxpool.Pool
	svc   *compose.RunnerService
	store *runner.Store
	brain *ai.FakeClient
	wsID  ids.UUID
	wsCtx context.Context

	passportID ids.PassportID
}

// stagingSpecName is the catalog entry the 🟡 tests drive.
//
// It exists because no SHIPPED agent can stage an approval: every tool
// morning_brief and overnight_at_risk_sweep name is auto-execute, which is what
// their goals ask for. Before AgentSpec.Tools bound, these tests reached
// suspend/approve/resume by scripting the sweep to call archive_record — an
// action its own goal forbids — so the coverage existed only by way of a call
// the product now refuses (and TestTheSweepMayNotArchiveEvenWithAModelThatTriesTo
// asserts that refusal).
//
// This spec is the same shape a real entry has and differs in one way that
// matters: its allowlist NAMES a confirm-first verb, so that path is reachable
// the way it would be for any future agent that legitimately proposes a risky
// action.
//
// The verb is `enrich`. It was archive_record until a passport stopped needing
// a second confirmation from the person who granted it; enrich stays
// confirm-first for a different reason — the MODEL names the URL the server
// fetches — which is exactly what makes it the verb a scheduled, unattended run
// must still put in front of a human.
const stagingSpecName = "e2e_staging_agent"

// stagingSpec resolves the catalog for the 🟡 tests: the shipped entries
// unchanged, plus the one above.
func stagingSpec(name string) (runner.AgentSpec, bool) {
	if name == stagingSpecName {
		return runner.AgentSpec{
			Name:       stagingSpecName,
			Goal:       "Fill in what this workspace does not yet know about its companies, one at a time.",
			DueHourUTC: 2,
			Tools:      []string{"search_records", "read_record", "enrich"},
		}, true
	}
	// The shipped agents fall back to the COMPOSE resolver, not the runner
	// catalog: the catalog carries no tools, and this lane's whole subject is
	// what an allowlist refuses.
	return compose.ScheduledAgentSpecByName(name)
}

func setupRunner(t *testing.T) *runnerEnv {
	t.Helper()
	e := apptest.SetupApp(t)

	apptest.BootstrapWorkspaceSession(t, e, "Runner E2E", "runner@fable.test", "Admin")

	var minted struct {
		PassportID string `json:"passport_id"`
	}
	if status := e.Call(t, "POST", "/v1/passports", integration.AnyMap{
		"label": "overnight runner", "scopes": []string{"read", "write", "enrich"},
	}, nil, &minted); status != http.StatusCreated {
		t.Fatalf("issue passport → %d", status)
	}
	passportID, err := ids.ParseAs[ids.PassportKind](minted.PassportID)
	if err != nil {
		t.Fatal(err)
	}

	pool, err := testdb.OwnPool(context.Background(), os.Getenv("MARGINCE_TEST_APP_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	wsRaw := apptest.InstallationWorkspaceID(context.Background(), t, e.Owner)
	wsID, err := ids.Parse(wsRaw)
	if err != nil {
		t.Fatal(err)
	}

	brain := ai.NewFakeClient()
	// The AgentLoop lane rides the DB-less router (WithFakeClient swaps in this
	// scripted fake) instead of the deleted FakeModelPath's direct client
	// wiring, so the runner e2e exercises the same routing/budget/secret-
	// stripping pipeline production's AgentLoop lane does. WithoutResultCache
	// keeps every scripted step reaching the fake — the tests below script
	// a distinct response per reason-act step.
	modelPath, err := compose.NewLocalModelPath(ai.FakeRoutingConfig(), ai.WithFakeClient(brain), ai.WithoutResultCache())
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	return &runnerEnv{
		AppEnv: e,
		pool:   pool,
		svc: compose.NewRunnerService(pool, modelPath.AgentLoop, modelPath.DraftReply, nil, logger, nil,
			compose.SendPath{}, compose.WithSpecResolver(stagingSpec)),
		store: runner.NewStore(database.BindTo(pool, ids.From[ids.WorkspaceKind](wsID))),
		brain: brain,
		wsID:  wsID,
		// The pass's own actor, exactly as RunnerService.Tick binds one in
		// production. It is not scaffolding: seeding and finishing a job now
		// announce the occurrence to the AI-activity projection, and that
		// announcement carries the write shape — a ledger row and an outbox row,
		// both of which take their actor from the context. A helper that called
		// EnqueueJob with only a workspace bound would be testing a caller
		// production does not have.
		wsCtx: principal.WithActor(
			principal.WithCorrelationID(principal.WithWorkspaceID(context.Background(), wsID), ids.NewV7()),
			principal.Principal{Type: principal.PrincipalSystem, ID: "system:agent_scheduler"},
		),
		passportID: passportID,
	}
}

// tick runs ONE workspace's scheduler pass, under the bound context and the
// clock reading the job worker hands it in production — the fan-out that puts
// a tenant's pass on its own job row is
// TestAgentSchedulerFansOutOneJobPerLiveWorkspaceAndFailsOnlyTheFailedTenant's
// subject, not this suite's.
func (re *runnerEnv) tick(t *testing.T) {
	t.Helper()
	if err := re.svc.Tick(re.wsCtx, time.Now()); err != nil {
		t.Fatal(err)
	}
}

func (re *runnerEnv) enqueue(t *testing.T, spec, trigger string, passport *ids.PassportID) {
	t.Helper()
	if err := re.store.EnqueueJob(re.wsCtx, spec, trigger, passport, time.Now().Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
}

func (re *runnerEnv) runRow(t *testing.T, trigger string) (status string, trace []runner.Step, approvalID *string) {
	t.Helper()
	var traceJSON []byte
	err := database.WithWorkspaceTx(re.wsCtx, re.pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT status, trace, approval_id::text FROM agent_run WHERE trigger_ref = $1`, trigger).
			Scan(&status, &traceJSON, &approvalID)
	})
	if err != nil {
		t.Fatalf("run row for %s: %v", trigger, err)
	}
	if err := json.Unmarshal(traceJSON, &trace); err != nil {
		t.Fatal(err)
	}
	return status, trace, approvalID
}

func TestRunnerFullLoopWritesAsGovernedAgent(t *testing.T) {
	re := setupRunner(t)
	trigger := "overnight_at_risk_sweep:e2e-full"

	// The model proposes one governed write, then finishes.
	re.brain.Script(
		`{"tool":"log_activity","args":{"kind":"note","subject":"At-risk: no touch in 21 days","body":"evidence: none found"}}`,
		`{"final":{"summary":"one at-risk deal flagged"}}`,
	)
	re.enqueue(t, "overnight_at_risk_sweep", trigger, &re.passportID)
	re.tick(t)

	status, trace, _ := re.runRow(t, trigger)
	if status != "completed" {
		t.Fatalf("run status = %s, want completed", status)
	}
	if len(trace) != 1 || trace[0].Tool != "log_activity" {
		t.Fatalf("trace = %+v", trace)
	}
	if strings.Contains(trace[0].Observation, "refused") || strings.Contains(trace[0].Observation, "failed") {
		t.Fatalf("the governed write did not land: %s", trace[0].Observation)
	}

	// The write carries the AGENT's provenance in the same audit stream
	// as every other surface — no runner back door.
	var actorType, actorID, capturedBy string
	err := database.WithWorkspaceTx(re.wsCtx, re.pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `
			SELECT al.actor_type, al.actor_id, a.captured_by
			FROM audit_log al JOIN activity a ON a.id = al.entity_id
			WHERE al.action = 'create' AND al.entity_type = 'activity'
			ORDER BY al.occurred_at DESC LIMIT 1`).Scan(&actorType, &actorID, &capturedBy)
	})
	if err != nil {
		t.Fatal(err)
	}
	if actorType != "agent" || !strings.Contains(actorID, re.passportID.String()) {
		t.Fatalf("audit actor = %s %s, want the passport-bound agent", actorType, actorID)
	}
	if !strings.HasPrefix(capturedBy, "agent:") {
		t.Fatalf("captured_by = %q, want agent provenance", capturedBy)
	}

	// Idempotency: re-seeding and re-ticking the same occurrence starts
	// no second run.
	re.enqueue(t, "overnight_at_risk_sweep", trigger, &re.passportID)
	re.tick(t)
	var runs int
	err = database.WithWorkspaceTx(re.wsCtx, re.pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT count(*) FROM agent_run WHERE trigger_ref = $1`, trigger).Scan(&runs)
	})
	if err != nil {
		t.Fatal(err)
	}
	if runs != 1 {
		t.Fatalf("duplicate trigger started %d runs, want 1", runs)
	}
}

func TestRunnerConfirmationRequiredSuspendApproveResume(t *testing.T) {
	re := setupRunner(t)

	var person struct {
		ID string `json:"id"`
	}
	if status := re.Call(t, "POST", "/v1/organizations", integration.AnyMap{"display_name": "Unknown Co"}, nil, &person); status != http.StatusCreated {
		t.Fatalf("create organization → %d", status)
	}

	trigger := "overnight_at_risk_sweep:e2e-confirmation-required"
	re.brain.Script(
		fmt.Sprintf(`{"tool":"enrich","args":{"organization_id":"%s"}}`, person.ID),
		`{"final":{"summary":"enrich executed after approval"}}`,
	)
	re.enqueue(t, stagingSpecName, trigger, &re.passportID)
	re.tick(t)

	status, _, approvalID := re.runRow(t, trigger)
	if status != "awaiting_approval" || approvalID == nil {
		t.Fatalf("run = %s approval=%v, want a parked run", status, approvalID)
	}
	// The target is untouched while parked.
	var parked struct {
		ArchivedAt *string `json:"archived_at"`
	}
	if got := re.Call(t, "GET", "/v1/organizations/"+person.ID, nil, nil, &parked); got != http.StatusOK || parked.ArchivedAt != nil {
		t.Fatalf("target mutated while approval pending: GET → %d archived_at=%v", got, parked.ArchivedAt)
	}

	// A human approves in the same inbox every surface stages into.
	if got := re.Call(t, "POST", "/v1/approvals/"+*approvalID+"/approve", integration.AnyMap{}, nil, nil); got != http.StatusOK {
		t.Fatalf("approve → %d", got)
	}

	// The decision event resumes the run (the bus delivery itself is the
	// bus lane's suite; this drives the consumer handler directly).
	if err := re.svc.HandleEvent(context.Background(), decidedEnvelope(re.wsID, *approvalID, "approved")); err != nil {
		t.Fatal(err)
	}

	status, trace, _ := re.runRow(t, trigger)
	if status != "completed" {
		t.Fatalf("resumed run status = %s, want completed", status)
	}
	// The APPROVED call reached the tool, which is what the suspend/resume loop
	// exists to deliver. What enrich then answers is the crawler's business —
	// this deployment binds no model path — and the distinction is admission:
	// a step still awaiting a decision never runs at all.
	if len(trace) < 2 {
		t.Fatalf("trace lost the suspension boundary: %+v", trace)
	}
	if first, resumed := trace[0], trace[len(trace)-1]; first.Admission != "staged" || resumed.Admission == "staged" {
		t.Fatalf("the run did not cross the approval gap: first=%q resumed=%q; trace: %+v",
			first.Admission, resumed.Admission, trace)
	}
}

func TestRunnerConfirmationRequiredRejectionReplansWithoutEffect(t *testing.T) {
	re := setupRunner(t)

	var person struct {
		ID string `json:"id"`
	}
	if status := re.Call(t, "POST", "/v1/organizations", integration.AnyMap{"display_name": "Keep Me"}, nil, &person); status != http.StatusCreated {
		t.Fatalf("create organization → %d", status)
	}

	trigger := "overnight_at_risk_sweep:e2e-reject"
	re.brain.Script(
		fmt.Sprintf(`{"tool":"enrich","args":{"organization_id":"%s"}}`, person.ID),
		`{"final":{"summary":"left the record alone after rejection"}}`,
	)
	re.enqueue(t, stagingSpecName, trigger, &re.passportID)
	re.tick(t)
	_, _, approvalID := re.runRow(t, trigger)
	if approvalID == nil {
		t.Fatal("no staged approval")
	}
	if got := re.Call(t, "POST", "/v1/approvals/"+*approvalID+"/reject", integration.AnyMap{"reason": "not a duplicate"}, nil, nil); got != http.StatusOK {
		t.Fatalf("reject → %d", got)
	}
	if err := re.svc.HandleEvent(context.Background(), decidedEnvelope(re.wsID, *approvalID, "rejected")); err != nil {
		t.Fatal(err)
	}

	status, _, _ := re.runRow(t, trigger)
	if status != "completed" {
		t.Fatalf("rejected resume status = %s, want completed", status)
	}
	// A rejected action never reaches the tool: no step in the trace is admitted.
	_, trace, _ := re.runRow(t, trigger)
	for _, step := range trace {
		if step.Admission == "executed" {
			t.Fatalf("a rejected action executed anyway: %+v", trace)
		}
	}
}

func TestRunnerJobWithoutPassportFailsLoudly(t *testing.T) {
	re := setupRunner(t)
	trigger := "morning_brief:e2e-no-passport"
	re.enqueue(t, "morning_brief", trigger, nil)
	re.tick(t)
	var status, lastError string
	err := database.WithWorkspaceTx(re.wsCtx, re.pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT status, last_error FROM runner_job WHERE trigger_ref = $1`, trigger).Scan(&status, &lastError)
	})
	if err != nil {
		t.Fatal(err)
	}
	if status != "failed" || !strings.Contains(lastError, "no passport bound") {
		t.Fatalf("passport-less job = %s %q, want a loud failure", status, lastError)
	}
	// And no run started with ambient authority.
	var runs int
	err = database.WithWorkspaceTx(re.wsCtx, re.pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT count(*) FROM agent_run WHERE trigger_ref = $1`, trigger).Scan(&runs)
	})
	if err != nil {
		t.Fatal(err)
	}
	if runs != 0 {
		t.Fatalf("a run started without a passport: %d", runs)
	}
}

func decidedEnvelope(wsID ids.UUID, approvalID, verdict string) kevents.Envelope {
	id, _ := ids.Parse(approvalID)
	payload, _ := json.Marshal(map[string]string{"verdict": verdict})
	return kevents.Envelope{
		EventID: ids.NewV7(),
		Type:    "approval.decided",
		Entity:  kevents.EntityRef{Type: "approval", ID: id},
		Payload: payload,
	}
}

// One approval, one resume. The bus is at-least-once, so the same
// approval.decided arrives more than once — redelivered on a handler
// error, reclaimed by a peer replica while the first is still looping, or
// replayed after a worker restart. A resumed run is a fresh multi-step
// loop with a fresh budget, not an idempotent upsert, so the SECOND
// delivery must find nothing: the run is claimed as it is read, and the
// first loop's outcome and trace stand untouched.
func TestRunnerResumeIsClaimedSoARedeliveryIsANoOp(t *testing.T) {
	re := setupRunner(t)

	var person struct {
		ID string `json:"id"`
	}
	if status := re.Call(t, "POST", "/v1/organizations", integration.AnyMap{"display_name": "Resume Once"}, nil, &person); status != http.StatusCreated {
		t.Fatalf("create organization → %d", status)
	}

	trigger := "overnight_at_risk_sweep:e2e-resume-once"
	// Exactly enough script for ONE loop and one resume: a second loop
	// would run past the end of it, so the assertions below catch a
	// duplicate resume by its outcome as well as by its trace.
	re.brain.Script(
		fmt.Sprintf(`{"tool":"enrich","args":{"organization_id":"%s"}}`, person.ID),
		`{"final":{"summary":"enrich executed after approval"}}`,
	)
	re.enqueue(t, stagingSpecName, trigger, &re.passportID)
	re.tick(t)
	_, _, approvalID := re.runRow(t, trigger)
	if approvalID == nil {
		t.Fatal("no staged approval")
	}
	if got := re.Call(t, "POST", "/v1/approvals/"+*approvalID+"/approve", integration.AnyMap{}, nil, nil); got != http.StatusOK {
		t.Fatalf("approve → %d", got)
	}

	decided := decidedEnvelope(re.wsID, *approvalID, "approved")
	if err := re.svc.HandleEvent(context.Background(), decided); err != nil {
		t.Fatal(err)
	}
	firstStatus, firstTrace, _ := re.runRow(t, trigger)
	if firstStatus != "completed" {
		t.Fatalf("first resume status = %s, want completed", firstStatus)
	}

	// The redelivery. It is not an error — the group must keep flowing —
	// it simply has no parked run to act on.
	if err := re.svc.HandleEvent(context.Background(), decided); err != nil {
		t.Fatalf("a redelivered decision must be a no-op, not an error: %v", err)
	}
	secondStatus, secondTrace, _ := re.runRow(t, trigger)
	if secondStatus != firstStatus {
		t.Errorf("redelivery moved the run from %s to %s — the first outcome was overwritten", firstStatus, secondStatus)
	}
	if len(secondTrace) != len(firstTrace) {
		t.Errorf("redelivery appended %d trace steps — a second loop ran on one approval",
			len(secondTrace)-len(firstTrace))
	}
	// The approved call was ADMITTED exactly once. The trace is the record of
	// that: a second resume would append another step, and the count above
	// would move. Read here by admission rather than by counting the effect's
	// audit rows, because what the tool then does is its own business and a
	// redelivery must be a no-op whatever that is.
	admitted := 0
	for _, step := range secondTrace {
		if step.Admission != "staged" {
			admitted++
		}
	}
	if admitted != 1 {
		t.Errorf("the approved call was admitted %d times, want 1: %+v", admitted, secondTrace)
	}
}

// The sweep's goal has always said "do not advance stages, send anything, or
// archive anything". Until AgentSpec.Tools bound, that was prose: the scope
// model grants `write` in one block, so the passport admitted archive_record and
// only the prompt discouraged it.
//
// This drives the real shipped entry with a model that tries anyway — the exact
// script the three tests above used to rely on — and proves the refusal reaches
// the governed surface rather than the prompt: the run completes without
// parking, no approval is staged, and the target is untouched.
func TestTheSweepMayNotArchiveEvenWithAModelThatTriesTo(t *testing.T) {
	re := setupRunner(t)

	var person struct {
		ID string `json:"id"`
	}
	if status := re.Call(t, "POST", "/v1/people", integration.AnyMap{"full_name": "Stale Duplicate"}, nil, &person); status != http.StatusCreated {
		t.Fatalf("create person → %d", status)
	}

	// Still archive_record: this case is about the SWEEP's own allowlist
	// refusing a verb its goal does not include, which has nothing to do with
	// the tier that verb carries.
	trigger := "overnight_at_risk_sweep:e2e-archive-refused"
	re.brain.Script(
		fmt.Sprintf(`{"tool":"archive_record","args":{"record_type":"person","id":"%s"}}`, person.ID),
		`{"final":{"summary":"could not archive; noted the risk instead"}}`,
	)
	re.enqueue(t, "overnight_at_risk_sweep", trigger, &re.passportID)
	re.tick(t)

	status, trace, approvalID := re.runRow(t, trigger)
	if approvalID != nil {
		t.Fatalf("a tool outside the sweep's entry staged an approval: %v", approvalID)
	}
	if status != "completed" {
		t.Fatalf("run = %s, want completed — the refusal is an observation, not a crash", status)
	}
	// The refusal must be the governed surface's, recorded on the step.
	var refused bool
	for _, step := range trace {
		if step.Tool == "archive_record" && step.Admission == "refused" {
			refused = true
		}
	}
	if !refused {
		t.Fatalf("no refused archive_record step in the trace: %+v", trace)
	}
	// And the target is untouched, which is the property that actually matters.
	var after struct {
		ArchivedAt *string `json:"archived_at"`
	}
	if got := re.Call(t, "GET", "/v1/people/"+person.ID, nil, nil, &after); got != http.StatusOK || after.ArchivedAt != nil {
		t.Fatalf("the sweep archived a record it may not archive: GET → %d archived_at=%v", got, after.ArchivedAt)
	}
}

// A parked run whose authority dies before the decision arrives is CLOSED, not
// left parked forever.
//
// The revoked-passport branch is the only terminal write on this path that runs
// under the SUBSCRIBER's own context rather than the resumed run's, so it is the
// one that breaks if that context carries no actor: announcing the occurrence
// to the AI-activity projection is part of the same transaction, and a rollback
// there undoes MarkFailed while the claim stays taken — a redelivery then finds
// nothing to resume and the run is stuck in a state nothing can close.
func TestASuspendedRunWhoseAuthorityDiesIsClosedRatherThanParkedForever(t *testing.T) {
	re := setupRunner(t)
	var person struct {
		ID string `json:"id"`
	}
	if status := re.Call(t, "POST", "/v1/organizations", integration.AnyMap{
		"display_name": "Authority Dies Parked",
	}, nil, &person); status != http.StatusCreated {
		t.Fatalf("create organization → %d", status)
	}

	trigger := "overnight_at_risk_sweep:e2e-authority-died"
	re.brain.Script(
		fmt.Sprintf(`{"tool":"enrich","args":{"organization_id":"%s"}}`, person.ID),
		`{"final":{"summary":"never reached"}}`,
	)
	re.enqueue(t, stagingSpecName, trigger, &re.passportID)
	re.tick(t)

	status, _, approvalID := re.runRow(t, trigger)
	if status != "awaiting_approval" || approvalID == nil {
		t.Fatalf("run = %s approval=%v, want a parked run", status, approvalID)
	}
	if got := re.Call(t, "POST", "/v1/approvals/"+*approvalID+"/approve", integration.AnyMap{}, nil, nil); got != http.StatusOK {
		t.Fatalf("approve → %d", got)
	}

	// The authority dies while the run is parked — the case the branch exists
	// for. Revoking through the surface a human uses, not by editing the row.
	if got := re.Call(t, "DELETE", "/v1/passports/"+re.passportID.String(), nil, nil, nil); got != http.StatusNoContent {
		t.Fatalf("revoke passport → %d", got)
	}

	// The bare context is the point: this is exactly what the subscriber hands
	// the handler in production.
	if err := re.svc.HandleEvent(context.Background(), decidedEnvelope(re.wsID, *approvalID, "approved")); err != nil {
		t.Fatalf("the decision could not be handled: %v", err)
	}

	if status, _, _ = re.runRow(t, trigger); status != "failed" {
		t.Fatalf("run status = %s, want failed — a run whose authority died must be closed, not left parked", status)
	}
}
