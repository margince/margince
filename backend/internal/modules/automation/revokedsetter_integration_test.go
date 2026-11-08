// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package automation

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	kevents "github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

// setterRig is a rule, the engine that fires it and the users who act on it.
type setterRig struct {
	fx       *autoFixture
	store    *AutomationStore
	engine   *WorkflowEngine
	notices  *pauseNotices
	passport ids.UUID
	applied  *int
}

var ruleAuthorGrants = principal.Permissions{RowScope: principal.RowScopeAll, RoleKeys: []string{"admin"}, Objects: map[string]principal.ObjectGrant{
	"automation": {Create: true, Read: true, Update: true, Delete: true},
	"lead":       {Create: true, Read: true, Update: true, Delete: true},
	"activity":   {Create: true, Read: true, Update: true, Delete: true},
	"deal":       {Create: true, Read: true, Update: true, Delete: true},
	"contact":    {Create: true, Read: true, Update: true, Delete: true},
}}

func newSetterRig(t *testing.T) *setterRig {
	t.Helper()
	fx := setupAutomationDB(t)
	db := database.BindTo(fx.pool, ids.From[ids.WorkspaceKind](fx.ws))
	applied := 0
	notices := &pauseNotices{}
	engine := NewWorkflowEngine(db, fullyPermittedOwner()).WithRulePauses(notices, nil)
	engine.RegisterWorkflow(scriptedWorkflow{name: assignLeadOwnerName, apply: func(workflow.Event) (workflow.RunResult, error) {
		applied++
		return workflow.RunResult{}, nil
	}})
	passport := ids.NewV7()
	fx.exec(t, `INSERT INTO passport (id, on_behalf_of, granted_by, label, scopes, token_hash, expires_at)
		VALUES ($1, $2, $2, 'rule setter', ARRAY['read','write'], $3, now() + interval '1 day')`, passport, fx.rep1, "hash-"+passport.String())
	return &setterRig{fx: fx, store: NewAutomationStore(db), engine: engine, notices: notices, passport: passport, applied: &applied}
}

func (r *setterRig) user() context.Context {
	return principal.WithActor(principal.WithWorkspaceID(context.Background(), r.fx.ws), principal.Principal{
		Type: principal.PrincipalHuman, ID: principal.HumanIDPrefix + r.fx.rep1.String(), UserID: r.fx.rep1, SeatType: principal.SeatFull, Permissions: ruleAuthorGrants,
	})
}

func (r *setterRig) agent() context.Context {
	return principal.WithActor(principal.WithWorkspaceID(context.Background(), r.fx.ws), principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:setter", UserID: r.fx.rep1, OnBehalfOf: r.fx.rep1, PassportID: r.passport,
		SeatType: principal.SeatFull, Permissions: ruleAuthorGrants, Scopes: principal.NewScopeSet(principal.ScopeRead, principal.ScopeWrite),
	})
}

func (r *setterRig) ruleBy(ctx context.Context, t *testing.T) Automation {
	t.Helper()
	rule, err := r.store.Create(ctx, CreateAutomationInput{Key: assignLeadOwnerName, Name: "Assign leads", Params: map[string]any{}})
	if err != nil {
		t.Fatalf("creating the rule: %v", err)
	}
	return r.enable(ctx, t, rule)
}

func (r *setterRig) enable(ctx context.Context, t *testing.T, rule Automation) Automation {
	t.Helper()
	on := true
	enabled, err := r.store.Update(ctx, rule.ID, UpdateAutomationInput{Enabled: &on, IfVersion: &rule.Version})
	if err != nil {
		t.Fatalf("enabling the rule: %v", err)
	}
	return enabled
}

func (r *setterRig) revoke(t *testing.T) {
	t.Helper()
	r.fx.exec(t, `UPDATE passport SET revoked_at = now() WHERE id = $1`, r.passport)
}

func (r *setterRig) fire(t *testing.T) {
	t.Helper()
	if err := r.engine.HandleEvent(context.Background(), activityEventFor(r.fx.seedActivity(t, "workspace"))); err != nil {
		t.Fatalf("firing: %v", err)
	}
}

// ruleState reads whether a rule runs, why it stopped, and which passport it names.
func (r *setterRig) ruleState(t *testing.T, rule Automation) (enabled bool, reason, setter *string) {
	t.Helper()
	if err := r.fx.owner.QueryRow(context.Background(),
		`SELECT enabled, paused_reason, setter_passport_id::text FROM automation WHERE id = $1`, rule.ID).Scan(&enabled, &reason, &setter); err != nil {
		t.Fatalf("reading the rule: %v", err)
	}
	return enabled, reason, setter
}

// A rule an agent set up stops with the passport it was set up on. It does not
// fire first, the owner is told, and the audit row says why.
func TestARuleStopsWithoutFiringOnceItsSettersPassportIsRevoked(t *testing.T) {
	r := newSetterRig(t)
	rule := r.ruleBy(r.agent(), t)
	if _, _, setter := r.ruleState(t, rule); setter == nil || *setter != r.passport.String() {
		t.Fatalf("the rule names passport %v, want %s", setter, r.passport)
	}
	r.fire(t)
	if *r.applied != 1 {
		t.Fatalf("the live rule fired %d times, want 1", *r.applied)
	}
	r.revoke(t)
	r.fire(t)
	if *r.applied != 1 {
		t.Errorf("the rule fired %d times after its passport was revoked, want it stopped at 1", *r.applied)
	}
	enabled, reason, _ := r.ruleState(t, rule)
	if enabled || reason == nil || *reason != PausedPassportRevoked {
		t.Errorf("the rule reads enabled %t, reason %v", enabled, reason)
	}
	if got := r.fx.count(t, `SELECT count(*) FROM audit_log WHERE entity_id = $1 AND after->>'paused_reason' = 'passport_revoked'`, rule.ID); got != 1 {
		t.Errorf("%d audit rows name why, want 1", got)
	}
	if got := r.fx.count(t, `SELECT count(*) FROM workflow_run WHERE status = 'skipped' AND detail::text LIKE '%passport%'`); got != 1 {
		t.Errorf("%d runs record the firing the revocation stopped, want 1", got)
	}
	if len(r.notices.told) != 1 || r.notices.told[0] != r.fx.rep1 {
		t.Errorf("the notices went to %v, want the owner", r.notices.told)
	}
}

// A user enabling the rule makes it theirs, so it runs whatever becomes of the passport.
func TestARuleTheUserReEnabledKeepsFiring(t *testing.T) {
	r := newSetterRig(t)
	rule := r.ruleBy(r.agent(), t)
	r.revoke(t)
	r.fire(t)
	var current Automation
	current, err := r.store.Get(r.user(), rule.ID)
	if err != nil {
		t.Fatal(err)
	}
	r.enable(r.user(), t, current)
	if enabled, reason, setter := r.ruleState(t, rule); !enabled || reason != nil || setter != nil {
		t.Fatalf("after the user's enable the rule reads %t, %v, %v", enabled, reason, setter)
	}
	r.fire(t)
	if *r.applied != 1 {
		t.Errorf("the user's rule fired %d times, want 1", *r.applied)
	}
}

// A user editing a running rule takes it over too, even when the edit changes nothing else.
func TestAUserEditingAnAgentsRuleTakesItOver(t *testing.T) {
	r := newSetterRig(t)
	rule := r.ruleBy(r.agent(), t)
	on := true
	if _, err := r.store.Update(r.user(), rule.ID, UpdateAutomationInput{Enabled: &on, IfVersion: &rule.Version}); err != nil {
		t.Fatal(err)
	}
	r.revoke(t)
	r.fire(t)
	if *r.applied != 1 {
		t.Errorf("the rule fired %d times, want 1", *r.applied)
	}
}

// A rule a user made themselves names no passport, so no revocation reaches it.
func TestAUsersOwnRuleIgnoresARevokedPassport(t *testing.T) {
	r := newSetterRig(t)
	rule := r.ruleBy(r.user(), t)
	if _, _, setter := r.ruleState(t, rule); setter != nil {
		t.Fatalf("a user's rule names passport %s", *setter)
	}
	r.revoke(t)
	r.fire(t)
	if enabled, _, _ := r.ruleState(t, rule); !enabled || *r.applied != 1 {
		t.Errorf("the rule reads enabled %t after %d firings", enabled, *r.applied)
	}
}

// A seeded rule belongs to the product. An agent enabling one leaves no setter on it.
func TestAnAgentEnablingASeededRuleLeavesNoSetter(t *testing.T) {
	r := newSetterRig(t)
	current := r.seededRule(t, stageChangeNotifyName)
	r.enable(r.agent(), t, current)
	if _, _, setter := r.ruleState(t, current); setter != nil {
		t.Errorf("the seeded rule names passport %s", *setter)
	}
}

// A write that loses the race with a revocation is refused, on a passport the
// request was admitted under moments before.
func TestARevokedPassportSetsUpNoRule(t *testing.T) {
	r := newSetterRig(t)
	r.revoke(t)
	_, err := r.store.Create(r.agent(), CreateAutomationInput{Key: assignLeadOwnerName, Name: "Too late", Params: map[string]any{}})
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("creating on a revoked passport answered %v", err)
	}
}

// A failed firing of a stopped rule is not retried past the revocation.
func TestARevokedSettersRuleIsNotRetried(t *testing.T) {
	r := newSetterRig(t)
	failing := NewWorkflowEngine(database.BindTo(r.fx.pool, ids.From[ids.WorkspaceKind](r.fx.ws)), fullyPermittedOwner()).WithRulePauses(r.notices, nil)
	failing.RegisterWorkflow(scriptedWorkflow{name: assignLeadOwnerName, redrivable: true, apply: func(workflow.Event) (workflow.RunResult, error) {
		*r.applied++
		return workflow.RunResult{}, errors.New("the downstream service was unreachable")
	}})
	rule := r.ruleBy(r.agent(), t)
	if err := failing.HandleEvent(context.Background(), r.fx.stagedEvent(t, ids.NewV7())); err == nil {
		t.Fatal("the scripted Apply failed, so HandleEvent must surface it")
	}
	failed, _ := r.fx.runOf(t, rule.ID, "failed")
	r.revoke(t)
	outcome, err := failing.RetryRun(context.Background(), failed)
	if err != nil || outcome.Retried || outcome.Refusal != RetryRefusedPassportRevoked {
		t.Errorf("retrying answered %+v, %v", outcome, err)
	}
	if *r.applied != 1 {
		t.Errorf("the rule ran %d times, want only the first", *r.applied)
	}
}

// A rule an agent creates from a seeded key is the agent's, so revoking stops it.
func TestAnAgentsRuleFromASeededKeyNamesItsPassport(t *testing.T) {
	r := newSetterRig(t)
	rule, err := r.store.Create(r.agent(), CreateAutomationInput{Key: stageChangeNotifyName, Name: "Mine too", Params: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, setter := r.ruleState(t, rule); setter == nil || *setter != r.passport.String() {
		t.Errorf("the rule names passport %v, want %s", setter, r.passport)
	}
	running := r.enable(r.agent(), t, rule)
	off := false
	if _, err := r.store.Update(r.agent(), rule.ID, UpdateAutomationInput{Enabled: &off, IfVersion: &running.Version}); err != nil {
		t.Errorf("the agent pausing its own rule answered %v", err)
	}
	if enabled, _, _ := r.ruleState(t, rule); enabled {
		t.Error("the agent's pause left its own rule running")
	}
}

// seededRule is an unowned row of a seeded key, as the product starts it.
func (r *setterRig) seededRule(t *testing.T, key string) Automation {
	t.Helper()
	id := ids.New[ids.AutomationKind]()
	r.fx.exec(t, `INSERT INTO automation (id, key, name, trigger, action, enabled)
		VALUES ($1, $2, $2, '{"event_type":"test"}', '{"kind":"test"}', false)`, id, key)
	rule, err := r.store.Get(r.user(), id)
	if err != nil {
		t.Fatal(err)
	}
	return rule
}

// An agent does not change who owns a rule. So it never takes a seeded rule as its
// own user's, and the seeded limits keep holding.
func TestAnAgentDoesNotTakeASeededRuleAsItsOwn(t *testing.T) {
	r := newSetterRig(t)
	rule := r.seededRule(t, stageChangeNotifyName)
	me := ids.From[ids.UserKind](r.fx.rep1)
	_, err := r.store.Update(r.agent(), rule.ID, UpdateAutomationInput{OwnerID: &me})
	if detailed, ok := errors.AsType[*httperr.DetailedError](err); !ok || detailed.Code != "agent_hands_on_no_rule" {
		t.Errorf("an agent naming its own user as the owner answered %v", err)
	}
	if n := r.fx.count(t, `SELECT count(*) FROM automation WHERE id = $1 AND owner_id IS NULL`, rule.ID); n != 1 {
		t.Error("the seeded rule gained an owner")
	}
}

// The agent's enable is what turns a draft starter on, and the starter spends a model
// call each time it fires. Revoking the agent's passport stops that spend.
func TestAnAgentEnablingADraftStarterIsItsSetter(t *testing.T) {
	r := newSetterRig(t)
	rule := r.seededRule(t, "post_meeting_recap")
	ctx := r.agent()
	actor, _ := principal.Actor(ctx)
	actor.Scopes = principal.NewScopeSet(principal.ScopeRead, principal.ScopeWrite, principal.ScopeEnrich)
	r.enable(principal.WithActor(ctx, actor), t, rule)
	if _, _, setter := r.ruleState(t, rule); setter == nil || *setter != r.passport.String() {
		t.Errorf("the started rule names passport %v, want %s", setter, r.passport)
	}
}

// An edit that names a running rule's status claims nothing: the rule stays the
// user's unless the agent is what turned it on.
func TestAnAgentEditingAUsersRunningRuleDoesNotBecomeItsSetter(t *testing.T) {
	r := newSetterRig(t)
	rule := r.ruleBy(r.user(), t)
	on, renamed := true, "Renamed by the agent"
	if _, err := r.store.Update(r.agent(), rule.ID, UpdateAutomationInput{Enabled: &on, Name: &renamed, IfVersion: &rule.Version}); err != nil {
		t.Fatal(err)
	}
	if _, _, setter := r.ruleState(t, rule); setter != nil {
		t.Errorf("the user's rule names passport %s after the agent's edit", *setter)
	}
	r.revoke(t)
	r.fire(t)
	if enabled, _, _ := r.ruleState(t, rule); !enabled || *r.applied != 1 {
		t.Errorf("the user's rule reads enabled %t after %d firings", enabled, *r.applied)
	}
}

// An agent turning a rule back on, after someone paused it, stands behind it.
func TestAnAgentResumingAPausedRuleBecomesItsSetter(t *testing.T) {
	r := newSetterRig(t)
	rule := r.ruleBy(r.user(), t)
	off := false
	paused, err := r.store.Update(r.user(), rule.ID, UpdateAutomationInput{Enabled: &off, IfVersion: &rule.Version})
	if err != nil {
		t.Fatal(err)
	}
	r.enable(r.agent(), t, paused)
	if _, _, setter := r.ruleState(t, rule); setter == nil || *setter != r.passport.String() {
		t.Errorf("the resumed rule names passport %v, want %s", setter, r.passport)
	}
}

// A list rule's check may move no records, so no firing would ever reach the stop.
// The rule still stops on the check itself.
func TestAListRuleStopsOnARevokedSetterWhenItsCheckMovedNothing(t *testing.T) {
	r := newSetterRig(t)
	r.engine.RegisterWorkflow(listRule{name: listNotifyName, action: ActionTypeNotify, ex: Executors{Lists: scriptedLists{}}})
	rule := r.fx.listRule(t, r.fx.rep1, ids.NewV7())
	r.fx.exec(t, `UPDATE automation SET setter_passport_id = $1 WHERE id = $2`, r.passport, rule)
	checkOfAnotherList := func() kevents.Envelope {
		return kevents.Envelope{
			EventID: ids.NewV7(), Type: eventListEvaluated, OccurredAt: time.Date(2026, 7, 16, 9, 0, 0, 0, time.UTC),
			Entity: kevents.EntityRef{Type: rbacObjList, ID: ids.NewV7()},
		}
	}
	if err := r.engine.HandleEvent(context.Background(), checkOfAnotherList()); err != nil {
		t.Fatal(err)
	}
	if n := r.fx.count(t, `SELECT count(*) FROM automation WHERE id = $1 AND enabled`, rule); n != 1 {
		t.Fatal("the check paused a rule whose setter is still live")
	}
	r.revoke(t)
	if err := r.engine.HandleEvent(context.Background(), checkOfAnotherList()); err != nil {
		t.Fatal(err)
	}
	if n := r.fx.count(t, `SELECT count(*) FROM automation WHERE id = $1 AND NOT enabled AND paused_reason = $2`, rule, PausedPassportRevoked); n != 1 {
		t.Error("the check that moved nothing left the rule running under a revoked passport")
	}
}

// A user taking the rule over after the engine read the revoked setter wins. The
// pause waits on the user's uncommitted row, then finds a newer version and leaves
// it, and the user's rule fires.
func TestARuleTakenOverWhileItIsBeingPausedFires(t *testing.T) {
	r := newSetterRig(t)
	rule := r.ruleBy(r.agent(), t)
	r.revoke(t)
	event := activityEventFor(r.fx.seedActivity(t, "workspace"))
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	takeover, err := r.fx.owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := takeover.Rollback(context.Background()); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Errorf("rolling back the takeover: %v", err)
		}
	})
	var pid int32
	if err := takeover.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
		t.Fatal(err)
	}
	if _, err := takeover.Exec(ctx, `UPDATE automation SET setter_passport_id = NULL, version = version + 1 WHERE id = $1`, rule.ID); err != nil {
		t.Fatal(err)
	}
	returned := make(chan struct{})
	var fired error
	go func() {
		defer close(returned)
		fired = r.engine.HandleEvent(context.Background(), event)
	}()
	testdb.AwaitQueuedBehind(t, r.fx.owner, pid, 1, returned, "the pause")
	if err := takeover.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-returned:
	case <-ctx.Done():
		t.Fatal("the firing never returned after the takeover committed")
	}
	if fired != nil {
		t.Fatal(fired)
	}
	if enabled, reason, setter := r.ruleState(t, rule); !enabled || reason != nil || setter != nil {
		t.Errorf("the rule reads enabled %t, reason %v, setter %v, want the user's running rule", enabled, reason, setter)
	}
	if *r.applied != 1 {
		t.Errorf("the rule fired %d times, want 1", *r.applied)
	}
}

// A passport with no grant runs out at its own expiry. The rule pauses at its next
// firing without running, the audit row says why, and the owner is told.
func TestARuleStopsWithoutFiringOnceItsSettersPassportExpires(t *testing.T) {
	r := newSetterRig(t)
	rule := r.ruleBy(r.agent(), t)
	r.fire(t)
	r.engine.SetClock(func() time.Time { return clocktest.Now(t).Add(48 * time.Hour) })
	r.fire(t)
	if *r.applied != 1 {
		t.Errorf("the rule fired %d times after its passport expired, want it stopped at 1", *r.applied)
	}
	enabled, reason, _ := r.ruleState(t, rule)
	if enabled || reason == nil || *reason != PausedPassportRevoked {
		t.Errorf("the rule reads enabled %t, reason %v", enabled, reason)
	}
	if got := r.fx.count(t, `SELECT count(*) FROM audit_log WHERE entity_id = $1 AND after->>'setter_passport_id_state' = 'expired'`, rule.ID); got != 1 {
		t.Errorf("%d audit rows name the expiry, want 1", got)
	}
	if len(r.notices.told) != 1 || r.notices.told[0] != r.fx.rep1 {
		t.Errorf("the notices went to %v, want the owner", r.notices.told)
	}
}

// A revocation's audit row keeps saying so, whatever the clock reads.
func TestARevokedSettersPauseAuditsItAsRevoked(t *testing.T) {
	r := newSetterRig(t)
	rule := r.ruleBy(r.agent(), t)
	r.revoke(t)
	r.engine.SetClock(func() time.Time { return clocktest.Now(t).Add(48 * time.Hour) })
	r.fire(t)
	if got := r.fx.count(t, `SELECT count(*) FROM audit_log WHERE entity_id = $1 AND after->>'setter_passport_id_state' = 'revoked'`, rule.ID); got != 1 {
		t.Errorf("%d audit rows name the revocation, want 1", got)
	}
}
