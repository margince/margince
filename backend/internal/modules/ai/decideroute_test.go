// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/decision"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// scriptedDecider answers each call with the next scripted reply and keeps
// every request it was sent.
type scriptedDecider struct {
	replies []decisionReply
	calls   []decision.Request
	// block makes every call wait for its context, the way a hung endpoint does.
	block bool
}

type decisionReply struct {
	resp decision.Response
	err  error
}

func (d *scriptedDecider) Decide(ctx context.Context, req decision.Request) (decision.Response, error) {
	d.calls = append(d.calls, req)
	if d.block {
		<-ctx.Done()
		return decision.Response{}, ctx.Err()
	}
	reply := d.replies[min(len(d.calls), len(d.replies))-1]
	return reply.resp, reply.err
}

func answered(choice string, confidence float64) decisionReply {
	return decisionReply{resp: decision.Response{
		Answers:     map[string]decision.Answer{"kind": {Choice: choice, Confidence: confidence}},
		InputTokens: 400, ServedModel: "typesafe/jev-1.13-20260917", ServedProvider: "TypeSafe",
	}}
}

// floorGate is a site gate with one floor across every label.
func floorGate(a decision.Answer) DecisionVerdict {
	if a.Confidence >= 0.8 {
		return DecisionAccepted
	}
	return DecisionBelowFloor
}

func acceptAnything(string) error { return nil }

var triageLLMRequest = model.Request{Messages: []model.Message{{Role: "user", Content: "what is example.org?"}}}

// decideFixture is a router serving site_triage's ladder from a stub, with a
// decisions lane answered by decider and a budget of 100 tokens of which spent
// are gone.
type decideFixture struct {
	router *Router
	store  *fakeCallStore
	meter  *memMeter
}

func newDecideFixture(t *testing.T, decider decision.Client, spent int64) decideFixture {
	t.Helper()
	store, meter := &fakeCallStore{}, &memMeter{spent: spent}
	ladder := stubClient{resp: model.Response{Text: "ladder answer", InputTokens: 30, OutputTokens: 5}}
	r := assembleRouter(map[Tier]model.Client{TierCheapCloud: ladder, TierPremium: ladder}, NewFakeClient(),
		ProfileCloudFrontier, meter, StaticBudget(100), store,
		map[Tier]routeMeta{TierCheapCloud: {provider: "gemini", model: "cheap"}, TierPremium: {provider: "gemini", model: "premium"}},
		false, nil)
	r.now = func() time.Time { return time.Date(2026, 9, 25, 9, 0, 0, 0, time.UTC) }
	var lane *decisionLane
	if decider != nil {
		lane = &decisionLane{client: decider, meta: jevLane.routeMeta()}
	}
	r.install(r.binding().withConfig(RoutingConfig{}, lane))
	r.decisionCertified = func(DecisionCertKey) bool { return true }
	return decideFixture{router: r, store: store, meter: meter}
}

func (f decideFixture) decide(t *testing.T, dreq decision.Request) (DecideOutcome, RouteInfo, error) {
	t.Helper()
	return f.router.Decide(wsContext(t), TaskSiteTriage, "triage", dreq, triageLLMRequest, acceptAnything, floorGate)
}

// rowShape is what each case asserts of one ai_call row.
type rowShape struct {
	kind   string
	tier   Tier
	reason string
}

func shapes(calls []Call) []rowShape {
	out := make([]rowShape, len(calls))
	for i, c := range calls {
		out[i] = rowShape{kind: c.Kind, tier: c.Tier, reason: c.AttemptReason}
	}
	return out
}

// assertOneLogicalCall holds the row invariants every case shares: one
// logical call, numbered attempts, and exactly the last one terminal.
func assertOneLogicalCall(t *testing.T, calls []Call) {
	t.Helper()
	for i, c := range calls {
		if c.LogicalCallID != calls[0].LogicalCallID || c.Attempt != i+1 || c.IsTerminal != (i == len(calls)-1) {
			t.Errorf("row %d: logical=%v attempt=%d terminal=%v — not one logical call", i, c.LogicalCallID, c.Attempt, c.IsTerminal)
		}
	}
}

func TestDecideServesTheDecisionOrSaysWhyTheLadderAnswered(t *testing.T) {
	decisionRow := rowShape{kind: callKindDecision, tier: TierDecideLane}
	completion := func(reason string) rowShape {
		return rowShape{kind: callKindCompletion, tier: TierCheapCloud, reason: reason}
	}
	oversize := triageQuestion
	oversize.State = json.RawMessage(`{"page":{"text":"` + strings.Repeat("x", 49_000) + `"}}`)
	cases := []struct {
		name        string
		replies     []decisionReply
		uncertified bool
		dreq        decision.Request
		decided     bool
		calls       int
		rows        []rowShape
	}{
		{"accepted", []decisionReply{answered("parked", 0.95)}, false, triageQuestion, true, 1, []rowShape{decisionRow}},
		{"no certification row", []decisionReply{answered("parked", 0.95)}, true, triageQuestion, false, 0, []rowShape{completion(attemptReasonDecisionUncertified)}},
		{"below the floor", []decisionReply{answered("parked", 0.5)}, false, triageQuestion, false, 1, []rowShape{decisionRow, completion(attemptReasonDecisionBelowFloor)}},
		{"a label the question never offered", []decisionReply{answered("personal", 0.99)}, false, triageQuestion, false, 1, []rowShape{decisionRow, completion(attemptReasonDecisionOffEnum)}},
		{"no answer to the question", []decisionReply{{resp: decision.Response{Answers: map[string]decision.Answer{}}}}, false, triageQuestion, false, 1, []rowShape{decisionRow, completion(attemptReasonDecisionOffEnum)}},
		{"a provider error", []decisionReply{{err: errors.New("down")}}, false, triageQuestion, false, 1, []rowShape{decisionRow, completion(attemptReasonDecisionError)}},
		{"a 429", []decisionReply{{err: fmt.Errorf("%w: busy", errProviderRefused)}}, false, triageQuestion, false, 1, []rowShape{decisionRow, completion(attemptReasonDecisionError)}},
		{"a rejected request", []decisionReply{{err: fmt.Errorf("%w: %w", errDecisionRejected, rejectedRequest(errors.New("400")))}}, false, triageQuestion, false, 1, []rowShape{decisionRow, completion(attemptReasonDecisionError)}},
		{"a state too large to send", []decisionReply{answered("parked", 0.95)}, false, oversize, false, 0, []rowShape{completion(attemptReasonDecisionStateTooLarge)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			decider := &scriptedDecider{replies: tc.replies}
			f := newDecideFixture(t, decider, 0)
			if tc.uncertified {
				f.router.decisionCertified = func(DecisionCertKey) bool { return false }
			}
			out, info, err := f.decide(t, tc.dreq)
			if err != nil {
				t.Fatal(err)
			}
			if out.Decided != tc.decided || len(decider.calls) != tc.calls {
				t.Fatalf("decided=%v after %d decider calls, want %v after %d", out.Decided, len(decider.calls), tc.decided, tc.calls)
			}
			if got := shapes(f.store.recorded); !reflect.DeepEqual(got, tc.rows) {
				t.Fatalf("rows = %+v, want %+v", got, tc.rows)
			}
			assertOneLogicalCall(t, f.store.recorded)
			if tc.decided && (info.Tier != TierDecideLane || out.Answer.Choice != "parked" || out.Response.Text != "") {
				t.Errorf("a decided call: info=%+v outcome=%+v", info, out)
			}
			if !tc.decided && out.Response.Text != "ladder answer" {
				t.Errorf("the ladder did not answer: %+v", out)
			}
		})
	}
}

// Requirement 3: with no lane bound, a decision site's call is exactly the
// call it made before — the same rows, field for field.
func TestAnUnboundLaneLeavesEveryRowAsCompleteStructuredWroteIt(t *testing.T) {
	control := newDecideFixture(t, nil, 0)
	if _, _, err := control.router.CompleteStructured(wsContext(t), TaskSiteTriage, triageLLMRequest, acceptAnything); err != nil {
		t.Fatal(err)
	}
	f := newDecideFixture(t, nil, 0)
	out, _, err := f.decide(t, triageQuestion)
	if err != nil || out.Decided || out.Response.Text != "ladder answer" {
		t.Fatalf("outcome=%+v err=%v", out, err)
	}
	normalize := func(calls []Call) []Call {
		out := append([]Call(nil), calls...)
		for i := range out {
			out[i].LogicalCallID, out[i].RequestFingerprint = ids.UUID{}, ""
		}
		return out
	}
	if got, want := normalize(f.store.recorded), normalize(control.store.recorded); !reflect.DeepEqual(got, want) {
		t.Fatalf("rows moved:\n got  %+v\n want %+v", got, want)
	}
}

func TestAnAcceptedDecisionIsTracedAndMeteredOnTheDecideTier(t *testing.T) {
	decider := &scriptedDecider{replies: []decisionReply{answered("parked", 0.95)}}
	f := newDecideFixture(t, decider, 0)
	if _, _, err := f.decide(t, triageQuestion); err != nil {
		t.Fatal(err)
	}
	row := f.store.recorded[0]
	if row.Provider != providerJevCompatible || row.ModelID != jevLane.Model || row.TokensIn != 400 || row.TokensOut != 0 ||
		row.ServedModel != "typesafe/jev-1.13-20260917" || row.ServedIdentitySource != servedIdentitySourceEcho || row.ServedProvider != "TypeSafe" {
		t.Errorf("decision row = %+v", row)
	}
	if len(f.meter.records) != 1 || f.meter.records[0] != (Usage{Task: TaskSiteTriage, Tier: TierDecideLane, TokensIn: 400}) {
		t.Errorf("metered %+v, want one decide-tier usage of 400 input tokens", f.meter.records)
	}
	if got := decider.calls[0].Model; got != jevLane.Model {
		t.Errorf("the decider was asked for model %q, want the lane's", got)
	}
}

// A decision row keeps what the lane answered whether or not the answer
// stood: a below-floor or off-enum answer is the one a site's floor is tuned
// from. Only an attempt that got no answer at all, and every row that is not a
// decision, carries none.
func TestEveryDecisionRowKeepsTheAnswerItRead(t *testing.T) {
	cases := []struct {
		name  string
		reply decisionReply
		want  *DecisionAnswer
	}{
		{"accepted", answered("parked", 0.95), &DecisionAnswer{Choice: "parked", Confidence: 0.95}},
		{"below the floor", answered("company", 0.62), &DecisionAnswer{Choice: "company", Confidence: 0.62}},
		{"a label the question never offered", answered("personal", 0.99), &DecisionAnswer{Choice: "personal", Confidence: 0.99}},
		{"no answer to the question", decisionReply{resp: decision.Response{Answers: map[string]decision.Answer{}}}, nil},
		{"a provider error", decisionReply{err: errors.New("down")}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newDecideFixture(t, &scriptedDecider{replies: []decisionReply{tc.reply}}, 0)
			if _, _, err := f.decide(t, triageQuestion); err != nil {
				t.Fatal(err)
			}
			if got := f.store.recorded[0].DecisionAnswer; !reflect.DeepEqual(got, tc.want) {
				t.Errorf("decision row answer = %+v, want %+v", got, tc.want)
			}
			for _, row := range f.store.recorded[1:] {
				if row.DecisionAnswer != nil {
					t.Errorf("the %s row carries a decision answer %+v", row.Kind, row.DecisionAnswer)
				}
			}
		})
	}
}

func TestADecisionUnderBudgetPressure(t *testing.T) {
	t.Run("a degraded band keeps the decision reason and marks the walk degraded", func(t *testing.T) {
		f := newDecideFixture(t, &scriptedDecider{replies: []decisionReply{answered("parked", 0.5)}}, 90)
		if _, _, err := f.decide(t, triageQuestion); err != nil {
			t.Fatal(err)
		}
		last := f.store.recorded[len(f.store.recorded)-1]
		if last.AttemptReason != attemptReasonDecisionBelowFloor || !last.Degraded {
			t.Errorf("fallback row reason=%q degraded=%v", last.AttemptReason, last.Degraded)
		}
	})
	t.Run("a queued background task is deferred with no call and no row", func(t *testing.T) {
		decider := &scriptedDecider{replies: []decisionReply{answered("parked", 0.95)}}
		f := newDecideFixture(t, decider, 100)
		if _, _, err := f.decide(t, triageQuestion); !errors.Is(err, ErrBudgetDeferred) {
			t.Fatalf("err = %v, want a deferral", err)
		}
		if len(decider.calls) != 0 || len(f.store.recorded) != 0 {
			t.Errorf("a deferral made %d calls and %d rows", len(decider.calls), len(f.store.recorded))
		}
	})
	t.Run("a degraded schema retry still reads budget_degrade", func(t *testing.T) {
		f := newDecideFixture(t, nil, 90)
		invalidOnce := 0
		validate := func(string) error {
			invalidOnce++
			if invalidOnce == 1 {
				return errors.New("not yet")
			}
			return nil
		}
		if _, _, err := f.router.CompleteStructured(wsContext(t), TaskSiteTriage, triageLLMRequest, validate); err != nil {
			t.Fatal(err)
		}
		if got := f.store.recorded[1].AttemptReason; got != attemptReasonBudgetDegrade {
			t.Errorf("the degraded retry reads %q, want budget_degrade", got)
		}
	})
}

func TestACachedAnswerSkipsTheDecisionCall(t *testing.T) {
	decider := &scriptedDecider{replies: []decisionReply{answered("parked", 0.95)}}
	f := newDecideFixture(t, decider, 0)
	ctx := wsContext(t)
	if _, _, err := f.router.CompleteStructured(ctx, TaskSiteTriage, triageLLMRequest, acceptAnything); err != nil {
		t.Fatal(err)
	}
	primed := len(f.store.recorded)
	out, info, err := f.router.Decide(ctx, TaskSiteTriage, "triage", triageQuestion, triageLLMRequest, acceptAnything, floorGate)
	if err != nil || out.Decided || !info.Cached {
		t.Fatalf("outcome=%+v info=%+v err=%v, want the cached ladder answer", out, info, err)
	}
	rows := f.store.recorded[primed:]
	if len(decider.calls) != 0 || len(rows) != 1 || !rows[0].CacheHit || rows[0].Kind != callKindCompletion {
		t.Fatalf("%d decider calls, rows %+v; want none and one traced cache hit", len(decider.calls), rows)
	}
}

func TestAHungDecisionFallsBackAtItsTimeout(t *testing.T) {
	decider := &scriptedDecider{block: true}
	f := newDecideFixture(t, decider, 0)
	f.router.decisionTimeout = 10 * time.Millisecond
	out, _, err := f.decide(t, triageQuestion)
	if err != nil || out.Decided {
		t.Fatalf("outcome=%+v err=%v", out, err)
	}
	if got := shapes(f.store.recorded); len(got) != 2 || got[1].reason != attemptReasonDecisionError {
		t.Fatalf("rows = %+v, want a decision row and a decision_error walk", got)
	}
}

func TestTheDecisionStateIsStrippedBeforeItLeaves(t *testing.T) {
	decider := &scriptedDecider{replies: []decisionReply{answered("parked", 0.95)}}
	f := newDecideFixture(t, decider, 0)
	planted := triageQuestion
	planted.State = json.RawMessage(`{"page":{"text":"login AKIAIOSFODNN7EXAMPLE region eu-central-1"}}`)
	if _, _, err := f.decide(t, planted); err != nil {
		t.Fatal(err)
	}
	if sent := string(decider.calls[0].State); strings.Contains(sent, "AKIAIOSFODNN7EXAMPLE") || !json.Valid(decider.calls[0].State) {
		t.Fatalf("the decider received %s", sent)
	}
	// What was removed is the audit trail's to answer, on the decision row as
	// on a completion row.
	if row := f.store.recorded[0]; row.SecretsRemoved != 1 || !reflect.DeepEqual(row.SecretKinds, []string{"aws_access_key"}) {
		t.Errorf("decision row records %d removed of kinds %v, want 1 aws_access_key", row.SecretsRemoved, row.SecretKinds)
	}
}

func TestADecisionRowCarriesItsPayloadInTheDecisionForm(t *testing.T) {
	f := newDecideFixture(t, &scriptedDecider{replies: []decisionReply{answered("parked", 0.95)}}, 0)
	f.router.capturePayloads = true
	if _, _, err := f.decide(t, triageQuestion); err != nil {
		t.Fatal(err)
	}
	p := f.store.recorded[0].Payload
	if p == nil {
		t.Fatal("a capturing router stored no decision payload")
	}
	var req struct {
		State     string                     `json:"state"`
		Questions map[string]json.RawMessage `json:"questions"`
	}
	if err := json.Unmarshal(p.Request, &req); err != nil || !strings.Contains(req.State, "Example Domain.") || req.Questions["kind"] == nil {
		t.Errorf("payload request = %s (%v)", p.Request, err)
	}
	if !strings.Contains(string(p.Response), `"parked"`) {
		t.Errorf("payload response = %s", p.Response)
	}
}

// verdictRouter serves a local-only verdict task from a local rung, with lane
// as its decisions lane and payload capture on.
func verdictRouter(lane *DecisionsConfig, decider decision.Client, store *fakeCallStore) *Router {
	r := assembleRouter(map[Tier]model.Client{TierLocalSmall: stubClient{resp: model.Response{Text: "local answer"}}}, NewFakeClient(),
		ProfileCloudFrontier, &memMeter{}, StaticBudget(100), store,
		map[Tier]routeMeta{TierLocalSmall: {provider: providerOllama, model: "gemma3"}}, true, nil)
	r.install(r.binding().withConfig(RoutingConfig{}, &decisionLane{client: decider, meta: lane.routeMeta()}))
	r.decisionCertified = func(DecisionCertKey) bool { return true }
	return r
}

func TestALocalOnlyTaskNeverReachesACloudLane(t *testing.T) {
	decider := &scriptedDecider{replies: []decisionReply{answered("parked", 0.99)}}
	store := &fakeCallStore{}
	out, _, err := verdictRouter(jevLane, decider, store).Decide(wsContext(t), TaskCaptureCounterpartyVerdict, "verdict",
		triageQuestion, triageLLMRequest, acceptAnything, floorGate)
	if err != nil || out.Decided || len(decider.calls) != 0 {
		t.Fatalf("outcome=%+v err=%v calls=%d", out, err, len(decider.calls))
	}
	if got := shapes(store.recorded); len(got) != 1 || got[0].reason != attemptReasonDecisionLocalOnly {
		t.Fatalf("rows = %+v, want one decision_local_only walk", got)
	}
}

// A local lane may answer a local-only task, and the task's no_payload
// contract holds on its decision row as on any other.
func TestALocalLaneAnswersALocalOnlyTaskAndCapturesNothing(t *testing.T) {
	if !NoPayload(TaskCaptureCounterpartyVerdict) {
		t.Fatal("the verdict task is expected to be no_payload; this case would prove nothing")
	}
	decider := &scriptedDecider{replies: []decisionReply{answered("parked", 0.99)}}
	store := &fakeCallStore{}
	out, _, err := verdictRouter(selfHostedLane, decider, store).Decide(wsContext(t), TaskCaptureCounterpartyVerdict, "verdict",
		triageQuestion, triageLLMRequest, acceptAnything, floorGate)
	if err != nil || !out.Decided {
		t.Fatalf("outcome=%+v err=%v", out, err)
	}
	if len(store.recorded) != 1 || store.recorded[0].Payload != nil {
		t.Fatalf("rows = %+v, want one decision row with no payload", store.recorded)
	}
}

func TestTheRailOpensOnceAcrossTheDecisionAndTheLadder(t *testing.T) {
	for _, tc := range []struct {
		name         string
		reply        decisionReply
		wantRenewals int
	}{
		{"a decision that stands", answered("parked", 0.95), 0},
		{"a decision that falls back", answered("parked", 0.4), 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			starter := &countingStarter{claim: aClaim()}
			r := railRouter(stubClient{resp: model.Response{Text: "answer"}}, starter)
			r.install(r.binding().withConfig(RoutingConfig{}, &decisionLane{
				client: &scriptedDecider{replies: []decisionReply{tc.reply}}, meta: jevLane.routeMeta(),
			}))
			r.decisionCertified = func(DecisionCertKey) bool { return true }
			ctx := principal.WithCorrelationID(wsCtx(), ids.NewV7())
			if _, _, err := r.Decide(ctx, TaskCaptureClassify, "classify", triageQuestion, model.Request{}, acceptAnything, floorGate); err != nil {
				t.Fatal(err)
			}
			if len(starter.starts) != 1 || len(starter.renewals) != tc.wantRenewals {
				t.Errorf("%d starts and %d renewals, want 1 and %d", len(starter.starts), len(starter.renewals), tc.wantRenewals)
			}
		})
	}
}

func TestDecideProbeAsksTheLaneAlone(t *testing.T) {
	decider := &scriptedDecider{replies: []decisionReply{answered("parked", 0.5)}}
	f := newDecideFixture(t, decider, 0)
	probe, err := f.router.DecideProbe(wsContext(t), TaskSiteTriage, "triage", triageQuestion, floorGate)
	if err != nil {
		t.Fatal(err)
	}
	if !probe.Asked || probe.Decided || probe.Answer.Choice != "parked" || probe.Reason != attemptReasonDecisionBelowFloor {
		t.Errorf("probe = %+v", probe)
	}
	if got := shapes(f.store.recorded); len(got) != 1 || got[0].kind != callKindDecision {
		t.Errorf("rows = %+v, want the decision row alone", got)
	}
	if _, err := newDecideFixture(t, nil, 0).router.DecideProbe(wsContext(t), TaskSiteTriage, "triage", triageQuestion, floorGate); err == nil {
		t.Error("a probe with no lane bound answered")
	}
}

func TestAZeroVerdictIsNeverAccepted(t *testing.T) {
	resp := answered("parked", 0.99).resp
	if _, verdict := readDecision(triageQuestion, resp, func(decision.Answer) DecisionVerdict { return 0 }); verdict != DecisionOffEnum {
		t.Errorf("a gate that answered nothing read as %v", verdict)
	}
	twoQuestions := triageQuestion
	twoQuestions.Questions = map[string]decision.Question{"kind": triageQuestion.Questions["kind"], "other": triageQuestion.Questions["kind"]}
	if _, verdict := readDecision(twoQuestions, resp, floorGate); verdict != DecisionOffEnum {
		t.Errorf("a two-question request read as %v", verdict)
	}
}

// The DB-less router takes a fake decider under the config's lane, and a
// certification run may treat every site as certified; without the lane the
// fake answers nothing.
func TestTheLocalRouterTakesAFakeDecider(t *testing.T) {
	cfg := FakeRoutingConfig()
	cfg.Decisions = jevLane
	decider := &scriptedDecider{replies: []decisionReply{answered("parked", 0.95)}}
	r, err := NewLocalRouter(cfg, WithFakeDecider(decider), WithEveryDecisionCertified())
	if err != nil {
		t.Fatal(err)
	}
	out, _, err := r.Decide(wsContext(t), TaskSiteTriage, "triage", triageQuestion, triageLLMRequest, acceptAnything, floorGate)
	if err != nil || !out.Decided || len(decider.calls) != 1 {
		t.Fatalf("outcome=%+v err=%v calls=%d", out, err, len(decider.calls))
	}
	production, err := NewLocalRouter(cfg, WithFakeDecider(decider))
	if err != nil {
		t.Fatal(err)
	}
	if production.decisionCertified(DecisionCertKey{Task: TaskSiteTriage, Site: "triage", Provider: jevLane.Provider, Model: jevLane.Model}) {
		t.Error("an uncertified model is served without WithEveryDecisionCertified")
	}
}

// The decision half reads the binding its caller loaded, not whatever is
// installed by the time it runs: a Rebind between Decide's entry and the
// decision call must not split one logical call across two routings.
func TestTheDecisionHalfReadsTheBindingItWasHanded(t *testing.T) {
	decider := &scriptedDecider{replies: []decisionReply{answered("parked", 0.95)}}
	f := newDecideFixture(t, decider, 0)
	snapshot := f.router.binding()
	f.router.install(snapshot.withConfig(RoutingConfig{}, nil))
	try, err := f.router.decideFirst(wsContext(t), newLogicalCall(), snapshot, TaskSiteTriage, "triage", triageQuestion, triageLLMRequest, floorGate)
	if err != nil || !try.decided || len(decider.calls) != 1 {
		t.Fatalf("try=%+v err=%v calls=%d, want the handed binding's lane to answer", try, err, len(decider.calls))
	}
}
