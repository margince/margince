// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package agentaccess

// What an agent left running follows its connection. A refresh leaves the same agent
// on a fresher passport, and only the connection's end stops the work.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/automation"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	kevents "github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

const (
	setterRuleKey     = "assign_lead_owner"
	setterRuleTrigger = "history.test_event"
)

// countedRule is the workflow behind the agent's rule, counting what it applies.
type countedRule struct{ applied *int }

func (r countedRule) Spec() workflow.Spec {
	return workflow.Spec{Name: setterRuleKey, Trigger: workflow.Trigger{EventType: setterRuleTrigger}, Tier: mcp.TierAutoExecute}
}

func (countedRule) Match(context.Context, workflow.Event) (bool, error) { return true, nil }

func (countedRule) Plan(_ context.Context, ev workflow.Event) (workflow.Effect, error) {
	return workflow.Effect{Actions: []workflow.Action{{Kind: workflow.ActionCreateTask, Target: ev.Entity, Args: json.RawMessage(`{}`)}}}, nil
}

func (r countedRule) Apply(context.Context, workflow.Event, workflow.Effect, *workflow.ApprovalToken) (workflow.RunResult, error) {
	*r.applied++
	return workflow.RunResult{}, nil
}

func (countedRule) IdempotencyKey(ev workflow.Event) string {
	return setterRuleKey + ":" + ev.ID.String()
}

// silentOwner hears a pause and says nothing.
type silentOwner struct{}

func (silentOwner) NotifyTx(context.Context, pgx.Tx, ids.UUID, string, string, datasource.EntityRef, string) error {
	return nil
}

// connectedRule is a rule a connected agent set up, and the engine that fires it.
type connectedRule struct {
	o       *oauthEnv
	id      string
	refresh string
	engine  *automation.WorkflowEngine
	applied *int
}

func newConnectedRule(t *testing.T) *connectedRule {
	t.Helper()
	return newConnectedRuleWithScope(t, "read write offline_access")
}

func newConnectedRuleWithScope(t *testing.T, scope string) *connectedRule {
	t.Helper()
	o := setupOAuth(t)
	code := o.authorize(t, url.Values{"scope": {scope}})
	status, body := o.exchange(t, url.Values{"code": {code}})
	if status != http.StatusOK {
		t.Fatalf("code exchange → %d %v", status, body)
	}
	access, _ := body["access_token"].(string)
	refresh, _ := body["refresh_token"].(string)
	bearer := map[string]string{"Authorization": "Bearer " + access}
	var created struct {
		ID string `json:"id"`
	}
	if status := o.Call(t, "POST", "/v1/automations", integration.AnyMap{
		"key": setterRuleKey, "name": "Assign leads", "params": integration.AnyMap{},
	}, bearer, &created); status != http.StatusCreated {
		t.Fatalf("the agent creating a rule → %d", status)
	}
	if status := o.Call(t, "PATCH", "/v1/automations/"+created.ID, integration.AnyMap{"status": "enabled"}, bearer, nil); status != http.StatusOK {
		t.Fatalf("the agent enabling its rule → %d", status)
	}
	var workspace ids.UUID
	if err := o.Owner.QueryRow(t.Context(), `SELECT id FROM workspace`).Scan(&workspace); err != nil {
		t.Fatal(err)
	}
	db := database.BindTo(o.Pool, ids.From[ids.WorkspaceKind](workspace))
	applied := 0
	engine := automation.NewWorkflowEngine(db, identity.NewService(o.Pool)).WithRulePauses(silentOwner{}, nil)
	engine.RegisterWorkflow(countedRule{applied: &applied})
	return &connectedRule{o: o, id: created.ID, refresh: refresh, engine: engine, applied: &applied}
}

func (r *connectedRule) fire(t *testing.T) {
	t.Helper()
	activity := ids.NewV7()
	if _, err := r.o.Owner.Exec(t.Context(), `
		INSERT INTO activity (id, kind, audience, source, captured_by, occurred_at)
		VALUES ($1, 'note', 'workspace', 'test', 'human:test', now())`, activity); err != nil {
		t.Fatal(err)
	}
	if err := r.engine.HandleEvent(context.Background(), kevents.Envelope{
		EventID: ids.NewV7(), Type: setterRuleTrigger, OccurredAt: clocktest.Now(t),
		Entity: kevents.EntityRef{Type: "activity", ID: activity},
	}); err != nil {
		t.Fatalf("firing: %v", err)
	}
}

func (r *connectedRule) state(t *testing.T) (enabled bool, reason *string) {
	t.Helper()
	if err := r.o.Owner.QueryRow(t.Context(), `SELECT enabled, paused_reason FROM automation WHERE id = $1`, r.id).Scan(&enabled, &reason); err != nil {
		t.Fatal(err)
	}
	return enabled, reason
}

func (r *connectedRule) renew(t *testing.T) {
	t.Helper()
	status, body := r.o.renew(t, r.refresh, nil)
	if status != http.StatusOK {
		t.Fatalf("renewal → %d %v", status, body)
	}
	r.refresh, _ = body["refresh_token"].(string)
}

func (r *connectedRule) wantStopped(t *testing.T, appliedSoFar int) {
	t.Helper()
	r.fire(t)
	if *r.applied != appliedSoFar {
		t.Errorf("the rule applied %d times once its connection ended, want it stopped at %d", *r.applied, appliedSoFar)
	}
	if enabled, reason := r.state(t); enabled || reason == nil || *reason != automation.PausedPassportRevoked {
		t.Errorf("the rule reads enabled %t, reason %v", enabled, reason)
	}
}

// The setter's passport is revoked by the refresh, and the rule keeps running.
func TestARuleKeepsRunningAcrossItsSettersRefresh(t *testing.T) {
	r := newConnectedRule(t)
	r.fire(t)
	r.renew(t)
	assertOwnerCount(t, r.o, 1, `SELECT count(*) FROM automation a JOIN passport p ON p.id = a.setter_passport_id WHERE a.id = $1 AND p.revoked_at IS NOT NULL`, r.id)
	r.fire(t)
	if enabled, reason := r.state(t); !enabled || reason != nil {
		t.Errorf("after a refresh the rule reads enabled %t, reason %v", enabled, reason)
	}
	if *r.applied != 2 {
		t.Errorf("the rule applied %d times, want 2", *r.applied)
	}
}

// A connection is judged by what it can still do. Its access token lapsing leaves
// the rule running while the grant can refresh, and the refresh window closing stops it.
func TestARuleOutlivesALapsedAccessTokenWhileItsGrantCanRefresh(t *testing.T) {
	r := newConnectedRule(t)
	r.fire(t)
	r.engine.SetClock(func() time.Time { return clocktest.Now(t).AddDate(0, 0, 45) })
	assertOwnerCount(t, r.o, 0, `SELECT count(*) FROM passport WHERE oauth_grant_id IS NOT NULL AND revoked_at IS NULL AND now() + interval '45 days' < expires_at`)
	r.fire(t)
	if enabled, reason := r.state(t); !enabled || reason != nil || *r.applied != 2 {
		t.Errorf("with a refreshable grant the rule reads enabled %t, reason %v, applied %d", enabled, reason, *r.applied)
	}
	r.engine.SetClock(func() time.Time { return clocktest.Now(t).AddDate(1, 0, 0) })
	r.wantStopped(t, 2)
	assertOwnerCount(t, r.o, 1, `SELECT count(*) FROM audit_log WHERE entity_id = $1 AND after->>'setter_passport_id_state' = 'expired'`, r.id)
}

// A grant that never allowed refresh ends with the passports it holds.
func TestARuleStopsWhenAConnectionThatCannotRefreshLapses(t *testing.T) {
	r := newConnectedRuleWithScope(t, "read write")
	r.fire(t)
	r.engine.SetClock(func() time.Time { return clocktest.Now(t).AddDate(0, 0, 45) })
	r.wantStopped(t, 1)
}

// Ending the connection stops the rule, however many refreshes came before.
func TestARuleStopsWhenItsSettersConnectionIsRevoked(t *testing.T) {
	r := newConnectedRule(t)
	r.renew(t)
	r.fire(t)
	if status := r.o.revoke(t, r.refresh, "refresh_token"); status != http.StatusOK {
		t.Fatalf("revoking → %d", status)
	}
	r.wantStopped(t, 1)
}

// An admin disabling the tool ends what every passport of that tool may do.
func TestARuleStopsWhenItsSettersClientIsDisabled(t *testing.T) {
	r := newConnectedRule(t)
	r.fire(t)
	var row string
	if err := r.o.Owner.QueryRow(t.Context(), `SELECT id FROM oauth_client WHERE client_id = $1`, r.o.clientID).Scan(&row); err != nil {
		t.Fatal(err)
	}
	if status := r.o.Call(t, "PATCH", "/v1/installation/agent-clients/"+row, integration.AnyMap{"disabled": true}, nil, nil); status != http.StatusOK {
		t.Fatalf("disabling the client → %d", status)
	}
	r.wantStopped(t, 1)
}

// Work queued under a connected agent runs after a refresh, on the newer passport.
// It stops when the connection does.
func TestStoredWorkFollowsItsAgentsConnection(t *testing.T) {
	o := setupOAuth(t)
	access, refresh := o.connect(t)
	ctx := context.Background()
	var workspace, first, connection ids.UUID
	if err := o.Owner.QueryRow(ctx, `SELECT id FROM workspace`).Scan(&workspace); err != nil {
		t.Fatal(err)
	}
	if err := o.Owner.QueryRow(ctx, `SELECT id, oauth_grant_id FROM passport WHERE token_hash = $1`, sha256Hex(access)).Scan(&first, &connection); err != nil {
		t.Fatal(err)
	}
	ctx = principal.WithWorkspaceID(ctx, workspace)
	live, err := identity.NewService(o.Pool).AuthenticateAgentByID(ctx, ids.From[ids.PassportKind](first))
	if err != nil {
		t.Fatal(err)
	}
	stored := migration.StoredActorOf(live.Principal())
	correlation := ids.NewV7()
	run := migration.Run{ID: migration.RunID(ids.NewV7())}
	run.JobAttempt, run.Actor, run.AttemptCorrelationID = 1, &stored, &correlation

	status, body := o.renew(t, refresh, nil)
	if status != http.StatusOK {
		t.Fatalf("renewal → %d %v", status, body)
	}
	bound, err := compose.ActAsImportRunForTest(ctx, o.Pool, run)
	if err != nil {
		t.Fatalf("rebuilding the agent after a refresh: %v", err)
	}
	actor, ok := principal.Actor(bound)
	if !ok || actor.PassportID == first || actor.ConnectionID != connection {
		t.Errorf("the rebuilt agent names passport %s on connection %s, want the successor of %s on %s", actor.PassportID, actor.ConnectionID, first, connection)
	}
	refreshed, _ := body["refresh_token"].(string)
	if status := o.revoke(t, refreshed, "refresh_token"); status != http.StatusOK {
		t.Fatalf("revoking → %d", status)
	}
	if _, err := compose.ActAsImportRunForTest(ctx, o.Pool, run); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("rebuilding after the connection ended answered %v, want a permission refusal", err)
	}
}
