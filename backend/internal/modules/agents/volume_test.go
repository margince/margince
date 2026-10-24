// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/platform/agentvolume"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// countingCharger is the meter as the registry uses it: it only ever receives
// charges, which is the whole of this half of the seam. It keeps them PER
// COUNTER, because most of what is worth asserting here is that a call was
// charged against the right one.
type countingCharger struct {
	charged map[agentvolume.Counter]int
	times   map[agentvolume.Counter]int
	err     error
	// errOn narrows a failing meter to ONE counter. Redis being down fails
	// every charge, and a test about the answer-stage asymmetry needs the
	// call-ceiling charge — which runs first and refuses before the handler —
	// to succeed, or it proves the wrong refusal.
	errOn agentvolume.Counter
}

func newCountingCharger() *countingCharger {
	return &countingCharger{charged: map[agentvolume.Counter]int{}, times: map[agentvolume.Counter]int{}}
}

func (c *countingCharger) Consume(_ context.Context, counter agentvolume.Counter, n int) error {
	c.times[counter]++
	c.charged[counter] += n
	if c.err != nil && (c.errOn == "" || c.errOn == counter) {
		return c.err
	}
	return nil
}

// servingTool hands back the records it was built with, through the ONE place
// a datasource.Record becomes tool output — so it exercises the charge point
// the real tools ride rather than a parallel one written for the test.
type servingTool struct {
	spec    mcp.ToolSpec
	records int
	fail    bool
}

func (s *servingTool) Spec() mcp.ToolSpec { return s.spec }

func (s *servingTool) Handle(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
	for range s.records {
		newWireRecord(ctx, datasource.Record{
			Ref: datasource.EntityRef{Type: "contact", ID: ids.NewV7()},
		})
	}
	if s.fail {
		return nil, errors.New("the handler failed after reading")
	}
	return json.RawMessage(`{}`), nil
}

func readToolSpec(name string) mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: name, Title: name, Version: testToolVersion, Description: describedForRegistration,
		InputSchema:   json.RawMessage(`{"type":"object"}`),
		RequiredScope: principal.ScopeRead, Tier: mcp.TierAutoExecute,
	}
}

// chargingRegistry is the ONE setup every test here shares: a registry with a
// counting charger, the tool under test registered, and an agent context
// holding scope. Options vary only what a given test is about.
func chargingRegistry(t *testing.T, tool mcp.Tool, opts ...chargeTestOption) (*Registry, *countingCharger, context.Context) {
	t.Helper()
	cfg := chargeTest{charger: newCountingCharger(), scope: principal.ScopeRead}
	for _, opt := range opts {
		opt(&cfg)
	}
	r := NewRegistry(cfg.approvals, auth.NewGate(fullSeatAuthority{}), WithVolumeCharger(cfg.charger))
	if cfg.noCharger {
		r = NewRegistry(cfg.approvals, auth.NewGate(fullSeatAuthority{}))
	}
	r.Register(tool)
	ctx := principal.WithWorkspaceID(context.Background(), ids.NewV7())
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:t", OnBehalfOf: ids.NewV7(),
		PassportID: ids.NewV7(),
		Scopes:     principal.NewScopeSet(cfg.scope),
	})
	return r, cfg.charger, ctx
}

type chargeTest struct {
	charger   *countingCharger
	scope     principal.Scope
	noCharger bool
	approvals Approvals
}

type chargeTestOption func(*chargeTest)

// withChargerError makes the meter unreachable, so a test can say what an
// uncountable charge does.
func withChargerError(err error) chargeTestOption {
	return func(c *chargeTest) { c.charger.err = err }
}

// withChargerErrorOn makes exactly one counter unrecordable.
func withChargerErrorOn(counter agentvolume.Counter, err error) chargeTestOption {
	return func(c *chargeTest) { c.charger.err, c.charger.errOn = err, counter }
}

// withScope runs the caller under a scope other than read.
func withScope(s principal.Scope) chargeTestOption {
	return func(c *chargeTest) { c.scope = s }
}

// withApprovals composes the registry with an approvals engine, which is what a
// call presenting an approval_id needs to be redeemed against at all.
func withApprovals(a Approvals) chargeTestOption {
	return func(c *chargeTest) { c.approvals = a }
}

// withNoCharger composes the registry with no meter at all.
func withNoCharger() chargeTestOption {
	return func(c *chargeTest) { c.noCharger = true }
}

// A registry composed without a charger records nothing and does not crash.
// The Surface-B runner builds one, and it runs as the human or the system that
// started it — neither of whom this bound governs.
func TestARegistryWithNoChargerServesNormally(t *testing.T) {
	r, _, ctx := chargingRegistry(t, &servingTool{spec: readToolSpec("search_records"), records: 3}, withNoCharger())

	if _, err := r.Invoke(ctx, "search_records", json.RawMessage(`{}`)); err != nil {
		t.Fatalf("a registry with no read charger failed to serve: %v", err)
	}
}

// A WRITE whose charge fails is still served. By the time the charge runs the
// mutation has committed and any approval it redeemed is consumed — send_email
// has SENT. Reporting that as a failure invites the caller to retry an
// irreversible act, and a second email costs more than an uncounted read.
func TestAWriteIsServedEvenWhenItsChargeCannotBeRecorded(t *testing.T) {
	spec := readToolSpec("send_email")
	spec.RequiredScope = principal.ScopeWrite
	r, _, ctx := chargingRegistry(t, &servingTool{spec: spec, records: 1},
		withScope(principal.ScopeWrite),
		// The ANSWER-stage charge is the one that fails here: by then the send
		// has happened. A meter that failed the call-ceiling charge too would
		// refuse before the handler ran, which is the opposite property.
		withChargerErrorOn(agentvolume.Egress, errors.New("redis is unreachable")))

	out, err := r.Invoke(ctx, "send_email", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("a committed write was reported as failed because it could not be counted: %v", err)
	}
	if len(out) == 0 {
		t.Error("the write's result was withheld")
	}
}

// Every admitted call spends the ceiling every other counter sits under, and it
// spends one however many records the answer carried. A per-record charge would
// make MCP-SESS-CALLS a record count under another name.
func TestEveryAdmittedCallSpendsOneOfTheCallCeiling(t *testing.T) {
	r, charger, ctx := chargingRegistry(t, &servingTool{spec: readToolSpec("search_records"), records: 20})

	if _, err := r.Invoke(ctx, "search_records", json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}

	if got := charger.charged[agentvolume.Calls]; got != 1 {
		t.Errorf("one call answering 20 records spent %d of the call ceiling, want 1", got)
	}
}

// A call the meter cannot COUNT does not run. This is the one charge point at
// which nothing has happened yet, which is exactly what lets it refuse — and
// the ceiling it defends is the one every other counter sits under, so leaving it
// uncounted while Redis blinks would let a flood through the gap.
func TestACallThatCannotBeCountedIsNotRun(t *testing.T) {
	tool := &servingTool{spec: readToolSpec("search_records"), records: 5}
	r, charger, ctx := chargingRegistry(t, tool,
		withChargerErrorOn(agentvolume.Calls, errors.New("redis is unreachable")))

	out, err := r.Invoke(ctx, "search_records", json.RawMessage(`{}`))

	if !errors.Is(err, apperrors.ErrBudgetExceeded) {
		t.Fatalf("an uncountable call → %v, want ErrBudgetExceeded", err)
	}
	if out != nil {
		t.Error("the answer was served anyway")
	}
	if len(charger.times) != 1 {
		t.Errorf("the handler ran: %v were charged for a call that was never counted", charger.times)
	}
}

// The act a call performs is charged against the counter its own KIND names,
// derived by the same function the gate refuses on. The two halves must agree,
// and the test that would pass without them agreeing is one that calls the
// derivation directly — so this one goes through the registry.
func TestTheActIsChargedAgainstTheCounterItsKindNames(t *testing.T) {
	cases := []struct {
		name    string
		scope   principal.Scope
		egress  bool
		counter agentvolume.Counter
	}{
		{"update_record", principal.ScopeWrite, false, agentvolume.Writes},
		{"send_email", principal.ScopeSend, true, agentvolume.Egress},
	}
	for _, c := range cases {
		spec := readToolSpec(c.name)
		spec.RequiredScope, spec.Egress = c.scope, c.egress
		r, charger, ctx := chargingRegistry(t, &servingTool{spec: spec, records: 1}, withScope(c.scope))

		if _, err := r.Invoke(ctx, c.name, json.RawMessage(`{}`)); err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}

		if got := charger.charged[c.counter]; got != 1 {
			t.Errorf("%s spent %d of %s, want 1", c.name, got, c.counter)
		}
		// And nothing of the OTHER act counter: a send that also spent the write
		// volume budget would exhaust the loose bound while the tight one guarded nothing.
		other := agentvolume.Writes
		if c.counter == agentvolume.Writes {
			other = agentvolume.Egress
		}
		if got := charger.charged[other]; got != 0 {
			t.Errorf("%s also spent %d of %s", c.name, got, other)
		}
	}
}

// A read-only tool spends one call and nothing else. Its act is a call, so a
// second charge would meter the same answer twice against one counter.
func TestAReadOnlyToolSpendsOneCallAndNothingElse(t *testing.T) {
	r, charger, ctx := chargingRegistry(t, &servingTool{spec: readToolSpec("search_records"), records: 4})

	if _, err := r.Invoke(ctx, "search_records", json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}

	if got := charger.charged[agentvolume.Calls]; got != 1 || charger.times[agentvolume.Calls] != 1 || len(charger.times) != 1 {
		t.Errorf("a read-only tool charged %v, want one call and no other counter", charger.charged)
	}
}

// The SECOND door's charge points, which the REST gate calls because it admits
// against a tool spec and never invokes this registry. Without them the
// counters the gate reads on that door are counters nothing pays into, and a
// credential that never touches /mcp is unbounded.
func TestTheRestDoorChargesTheSameCountersTheMCPDoorDoes(t *testing.T) {
	spec := readToolSpec("update_record")
	spec.RequiredScope = principal.ScopeWrite
	r, charger, ctx := chargingRegistry(t, &servingTool{spec: spec}, withScope(principal.ScopeWrite))

	if err := r.ChargeAdmittedCall(ctx, spec); err != nil {
		t.Fatalf("charging an admitted REST call: %v", err)
	}
	r.ChargeEffect(ctx, spec)

	if got := charger.charged[agentvolume.Calls]; got != 1 {
		t.Errorf("a REST call spent %d of the call ceiling, want 1", got)
	}
	if got := charger.charged[agentvolume.Writes]; got != 1 {
		t.Errorf("a REST mutation spent %d of the write quota, want 1", got)
	}
}

// A REST call the meter cannot COUNT is not run — the same rule the MCP door
// keeps, at the same moment: after admission, before the handler.
func TestARestCallThatCannotBeCountedIsRefused(t *testing.T) {
	spec := readToolSpec("update_record")
	spec.RequiredScope = principal.ScopeWrite
	r, _, ctx := chargingRegistry(t, &servingTool{spec: spec}, withScope(principal.ScopeWrite),
		withChargerErrorOn(agentvolume.Calls, errors.New("redis is unreachable")))

	if err := r.ChargeAdmittedCall(ctx, spec); !errors.Is(err, apperrors.ErrBudgetExceeded) {
		t.Fatalf("an uncountable REST call → %v, want ErrBudgetExceeded", err)
	}
}

// A REST EFFECT that cannot be counted is never reported as a failure: by then
// the handler has answered and the mutation has committed, so an error here
// would invite the retry of something that already happened.
func TestARestEffectThatCannotBeCountedIsStillDone(t *testing.T) {
	spec := readToolSpec("send_email")
	spec.RequiredScope, spec.Egress = principal.ScopeSend, true
	r, charger, ctx := chargingRegistry(t, &servingTool{spec: spec}, withScope(principal.ScopeSend),
		withChargerError(errors.New("redis is unreachable")))

	r.ChargeEffect(ctx, spec) // no error to return, and must not panic

	if got := charger.times[agentvolume.Egress]; got != 1 {
		t.Errorf("the egress charge was attempted %d times", got)
	}
}

// A read-only REST call has no ACT to charge beyond the call it already spent.
func TestARestReadHasNoActToCharge(t *testing.T) {
	spec := readToolSpec("search_records")
	r, charger, ctx := chargingRegistry(t, &servingTool{spec: spec})

	r.ChargeEffect(ctx, spec)

	if len(charger.times) != 0 {
		t.Errorf("a read-only REST call charged %v", charger.times)
	}
}

// A REST read spends one call, wherever the request came in.
func TestARestReadSpendsOneCall(t *testing.T) {
	r, charger, ctx := chargingRegistry(t, &servingTool{spec: readToolSpec("search_records")})

	if err := r.ChargeAdmittedRead(ctx); err != nil {
		t.Fatalf("charging an admitted REST read: %v", err)
	}

	if got := charger.charged[agentvolume.Calls]; got != 1 || len(charger.times) != 1 {
		t.Errorf("a REST read charged %v, want one call", charger.charged)
	}
}

// A REST read the meter cannot count is not run. Serving it would raise the
// ceiling by one per blip.
func TestARestReadThatCannotBeCountedIsRefused(t *testing.T) {
	r, _, ctx := chargingRegistry(t, &servingTool{spec: readToolSpec("search_records")},
		withChargerError(errors.New("redis is unreachable")))

	if err := r.ChargeAdmittedRead(ctx); !errors.Is(err, apperrors.ErrBudgetExceeded) {
		t.Fatalf("an uncountable REST read → %v, want ErrBudgetExceeded", err)
	}
}

// A registry with no meter charges nothing on that door either, and says so by
// not failing: the Surface-B runner composes exactly that.
func TestTheRestChargePointsAreSafeWithNoMeter(t *testing.T) {
	spec := readToolSpec("update_record")
	spec.RequiredScope = principal.ScopeWrite
	r, _, ctx := chargingRegistry(t, &servingTool{spec: spec}, withScope(principal.ScopeWrite), withNoCharger())

	if err := r.ChargeAdmittedCall(ctx, spec); err != nil {
		t.Fatalf("a registry with no meter refused a REST call: %v", err)
	}
	r.ChargeEffect(ctx, spec)
}

// One CALL spends one of the call ceiling, whichever arm carried it — and a
// call that never ran spends none.
//
// The two arms charge at different MOMENTS, and that is what makes the count
// worth pinning. An unapproved call is charged before dispatch, where nothing
// has happened yet and an uncountable charge can still refuse it. A call
// presenting an approval_id skips that point entirely and is charged at the
// redemption instead, absorbed rather than refusable, because by then the
// human's approval is consumed and refusing would burn it on a call that never
// ran. Charging at both would bill one act twice; charging at neither would
// leave the redeeming arm free, and the REST door mirrors exactly this
// (compose.TestOneRestCallSpendsOneOfTheCallCeiling).
func TestOneToolCallSpendsOneOfTheCallCeiling(t *testing.T) {
	for name, tc := range map[string]struct {
		tier      mcp.RiskTier
		waits     bool
		presented bool
		wantCalls int
		wantErr   error
	}{
		"confirm-first that waits even for its user, staged and never run": {
			tier: mcp.TierConfirmationRequired, waits: true, wantCalls: 0, wantErr: apperrors.ErrRequiresApproval,
		},
		"auto-execute, nothing presented":                                {tier: mcp.TierAutoExecute, wantCalls: 1},
		"auto-execute, redeeming a presented approval":                   {tier: mcp.TierAutoExecute, presented: true, wantCalls: 1},
		"confirm-first, redeeming a presented approval":                  {tier: mcp.TierConfirmationRequired, presented: true, wantCalls: 1},
		"confirm-first on a colleague's passport, run as that colleague": {tier: mcp.TierConfirmationRequired, wantCalls: 1},
	} {
		t.Run(name, func(t *testing.T) {
			spec := readToolSpec("update_record")
			spec.RequiredScope, spec.Tier, spec.WaitsEvenForItsUser = principal.ScopeWrite, tc.tier, tc.waits
			r, charger, ctx := chargingRegistry(t, &servingTool{spec: spec},
				withScope(principal.ScopeWrite), withApprovals(&recordingApprovals{}))

			args := json.RawMessage(`{}`)
			if tc.presented {
				args = json.RawMessage(`{"approval_id":"` + ids.New[ids.ApprovalKind]().String() + `"}`)
			}
			_, err := r.Invoke(ctx, spec.Name, args)
			if tc.wantErr == nil && err != nil {
				t.Fatalf("the call was refused: %v — this case is not the arm it names", err)
			}
			if tc.wantErr != nil && !errors.Is(err, tc.wantErr) {
				t.Fatalf("the call answered %v, want %v — this case is not the arm it names", err, tc.wantErr)
			}

			if got := charger.charged[agentvolume.Calls]; got != tc.wantCalls {
				t.Errorf("the call ceiling was charged %d, want %d", got, tc.wantCalls)
			}
		})
	}
}
