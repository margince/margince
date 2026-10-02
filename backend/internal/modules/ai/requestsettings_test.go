// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// blockingClient answers only when its context ends, with the context's error,
// so a deadline is observed without a sleep.
type blockingClient struct {
	stubClient
	deadlines []time.Duration
}

func (b *blockingClient) Complete(ctx context.Context, _ model.Request) (model.Response, error) {
	if deadline, ok := ctx.Deadline(); ok {
		b.deadlines = append(b.deadlines, time.Until(deadline))
	}
	<-ctx.Done()
	return model.Response{}, fmt.Errorf("ai: provider call: %w", ctx.Err())
}

// deadlineClient answers at once and notes the deadline it was given.
type deadlineClient struct {
	stubClient
	deadline time.Duration
}

func (d *deadlineClient) Complete(ctx context.Context, _ model.Request) (model.Response, error) {
	if deadline, ok := ctx.Deadline(); ok {
		d.deadline = time.Until(deadline)
	}
	return model.Response{Text: "ok", InputTokens: 1, OutputTokens: 1}, nil
}

func TestADeadlineIsATimeoutAndACancelIsNot(t *testing.T) {
	if got := classifyError(fmt.Errorf("call: %w", context.DeadlineExceeded)); got != sentinelTimeout {
		t.Fatalf("deadline = %q", got)
	}
	if got := classifyError(&timeoutError{}); got != sentinelTimeout {
		t.Fatalf("an HTTP client timeout = %q", got)
	}
	if got := classifyError(fmt.Errorf("call: %w", context.Canceled)); got != "provider_error" {
		t.Fatalf("cancel = %q", got)
	}
}

// timeoutError is how net/http reports its own Client.Timeout.
type timeoutError struct{}

func (*timeoutError) Error() string { return "Client.Timeout exceeded while awaiting headers" }
func (*timeoutError) Timeout() bool { return true }

func twoRungRouter(t *testing.T, slow, fast model.Client, fcs *fakeCallStore) *Router {
	t.Helper()
	r := assembleRouter(
		map[Tier]model.Client{TierLocalSmall: slow, TierCheapCloud: fast},
		fast, ProfileCloudFrontier, stubMeter{}, unlimitedBudget{}, fcs,
		map[Tier]routeMeta{TierLocalSmall: {provider: "ollama", model: "m"}, TierCheapCloud: {provider: "openai", model: "gpt-x"}},
		false, nil,
	)
	r.now = func() time.Time { return time.Unix(0, 0) }
	return r
}

func TestEachRungGetsItsOwnDeadline(t *testing.T) {
	fcs := &fakeCallStore{}
	slow := &blockingClient{}
	r := twoRungRouter(t, slow, stubClient{resp: model.Response{Text: "ok", InputTokens: 1, OutputTokens: 1}}, fcs)
	// Below the bound a save accepts: the router serves what it is given.
	r.SetTaskOverrides(TaskOverrides{TaskColdStart: {AttemptTimeoutMs: 1}})

	resp, info, err := r.serveCompletion(wsCtx(), TaskColdStart, []Tier{TierLocalSmall, TierCheapCloud}, model.Request{})

	if err != nil || resp.Text != "ok" || info.Tier != TierCheapCloud {
		t.Fatalf("resp %+v info %+v err %v", resp, info, err)
	}
	if len(fcs.recorded) != 2 || fcs.recorded[0].ErrorSentinel != sentinelTimeout || fcs.recorded[1].ErrorSentinel != "" {
		t.Fatalf("rows %+v, want a timeout on the slow rung and the answer above it", fcs.recorded)
	}
	if got := fcs.recorded[1].AttemptReason; got != attemptReasonTimeout {
		t.Errorf("the rung above says it ran because of %q, want %q", got, attemptReasonTimeout)
	}
}

func TestACanceledCallIsNotATimeout(t *testing.T) {
	fcs := &fakeCallStore{}
	slow := &blockingClient{}
	r := newTracingRouter(t, slow, fcs)
	ctx, cancel := context.WithCancel(wsCtx())
	cancel()

	_, _, err := r.serveCompletion(ctx, TaskColdStart, []Tier{TierCheapCloud}, model.Request{})

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
	if got := fcs.recorded[0].ErrorSentinel; got == sentinelTimeout {
		t.Fatalf("a caller's own cancellation was recorded as %q", got)
	}
}

func TestNoOverrideReproducesTodaysDeadlines(t *testing.T) {
	client := &deadlineClient{}
	r := newTracingRouter(t, client, &fakeCallStore{})
	if got := r.taskSettings(TaskColdStart); got != (EffectiveTask{AttemptTimeout: CallCeiling, DecisionTimeout: DecisionCallTimeout}) {
		t.Fatalf("defaults = %+v", got)
	}
	if _, _, err := r.serveCompletion(wsCtx(), TaskColdStart, []Tier{TierCheapCloud}, model.Request{}); err != nil {
		t.Fatal(err)
	}
	if client.deadline <= CallCeiling-time.Minute || client.deadline > CallCeiling {
		t.Fatalf("the rung was given %s, want the call ceiling %s", client.deadline, CallCeiling)
	}
}

func TestTaskOverrideBoundsAreTheSpecs(t *testing.T) {
	ok := TaskOverrides{TaskCaptureConfidentialityVerdict: {DecisionTimeoutMs: 30000, AttemptTimeoutMs: 120000, Thinking: "low"}}
	if err := validateTaskOverrides(ok); err != nil {
		t.Fatal(err)
	}
	bad := map[string]TaskOverrides{
		"capture_confidentiality_verdict.decision_timeout_ms": {TaskCaptureConfidentialityVerdict: {DecisionTimeoutMs: 3000}},
		"capture_classify.decision_timeout_ms":                {TaskCaptureClassify: {DecisionTimeoutMs: 15000}},
		"capture_classify.attempt_timeout_ms":                 {TaskCaptureClassify: {AttemptTimeoutMs: 301000}},
		"cold_start.attempt_timeout_ms":                       {TaskColdStart: {AttemptTimeoutMs: 9999}},
		// Wraps to 10 s if converted to a Duration before it is compared.
		"site_triage.attempt_timeout_ms": {TaskSiteTriage: {AttemptTimeoutMs: 288230376151721744}},
		"capture_classify.thinking":      {TaskCaptureClassify: {Thinking: "max"}},
	}
	for path, v := range bad {
		var f routingFaults
		if err := validateTaskOverrides(v); !errors.As(err, &f) || f[0].Path != path {
			t.Errorf("%s: %v", path, err)
		}
	}
}

func TestAnOverrideForARemovedTaskIsIgnoredAndReportedStale(t *testing.T) {
	retired := Task("retired_task")
	v := TaskOverrides{retired: {Thinking: "low"}, TaskColdStart: {Thinking: "high"}}
	if got := v.Effective(retired); got != TaskDefaults() {
		t.Fatalf("a removed task is sent %+v, want the defaults", got)
	}
	if stale := v.Stale(); len(stale) != 1 || stale[0] != retired {
		t.Fatalf("stale = %v", stale)
	}
	if err := validateTaskOverrides(v); err != nil {
		t.Fatalf("a stored override for a removed task refused the rest: %v", err)
	}
	if err := v.refusedAgainst(v); err != nil {
		t.Fatalf("re-saving an untouched stale override was refused: %v", err)
	}
	changed := TaskOverrides{retired: {Thinking: "high"}}
	var f routingFaults
	if err := changed.refusedAgainst(v); !errors.As(err, &f) || f[0].Path != "retired_task" {
		t.Fatalf("a changed override for a removed task = %v", err)
	}
}

func TestAResetTaskLeavesNoRow(t *testing.T) {
	v := TaskOverrides{TaskColdStart: {}, TaskSiteTriage: {Thinking: "low"}}.withoutEmpty()
	if _, kept := v[TaskColdStart]; kept || len(v) != 1 {
		t.Fatalf("stored %+v", v)
	}
}

func TestAnAdminThinkingLevelReachesTheRequestAndTheCacheKey(t *testing.T) {
	var sent model.Request
	r := newTracingRouter(t, requestRecorder{seen: &sent}, &fakeCallStore{})
	ws := ids.From[ids.WorkspaceKind](ids.NewV7())
	plain, err := cacheKey(ws, TaskColdStart, model.Request{})
	if err != nil {
		t.Fatal(err)
	}
	r.SetTaskOverrides(TaskOverrides{TaskColdStart: {Thinking: "minimal"}})
	if _, _, err := r.serveCompletion(wsCtx(), TaskColdStart, []Tier{TierCheapCloud}, model.Request{}); err != nil {
		t.Fatal(err)
	}
	if sent.ThinkingLevel != "minimal" {
		t.Fatalf("the adapter was asked for %q", sent.ThinkingLevel)
	}
	overridden, err := cacheKey(ws, TaskColdStart, model.Request{ThinkingLevel: "minimal"})
	if err != nil || overridden == plain {
		t.Fatalf("an override does not move the cache key (%v)", err)
	}
}

type requestRecorder struct {
	stubClient
	seen *model.Request
}

func (c requestRecorder) Complete(_ context.Context, req model.Request) (model.Response, error) {
	*c.seen = req
	return model.Response{Text: "ok", InputTokens: 1, OutputTokens: 1}, nil
}

// One table across adapters: an admin's level outranks the binding's and the
// site floor's on every wire that takes a level.
func TestAnAdminThinkingLevelOutranksTheBindingAndTheFloor(t *testing.T) {
	ask := model.Request{Messages: []model.Message{{Role: "user", Content: "hi"}}, ThinkingFloor: "low", ThinkingLevel: "minimal"}

	stub := &brokerStub{}
	high := &OpenRouterRouting{Reasoning: &OpenRouterReasoning{Effort: "high"}}
	if _, err := newBrokerClient(t, stub, "openai/gpt-oss-120b", high).Complete(context.Background(), ask); err != nil {
		t.Fatal(err)
	}
	// gpt-oss lists no minimal effort, so the lowest it lists above it is sent.
	if got := string(stub.chats[0]["reasoning"]); got != `{"effort":"low"}` {
		t.Errorf("broker reasoning = %s", got)
	}
	plain := &brokerStub{}
	if _, err := newBrokerClient(t, plain, "mistralai/ministral-8b-2512", nil).Complete(context.Background(), ask); err != nil {
		t.Fatal(err)
	}
	if got, sent := plain.chats[0]["reasoning"]; sent {
		t.Errorf("a model the broker lists as not reasoning was sent %s", got)
	}

	var body []byte
	gemini := geminiWithLevel(t, "high", func(w http.ResponseWriter, r *http.Request) {
		body = readBody(t, r.Body)
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"ok"}]},"finishReason":"STOP"}]}`))
	})
	if _, err := gemini.Complete(context.Background(), ask); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(body, []byte(`"thinkingLevel":"minimal"`)) {
		t.Errorf("gemini sent %s", body)
	}

	var openaiBody map[string]json.RawMessage
	openai := newOpenAIForTest(t, func(w http.ResponseWriter, r *http.Request) {
		if err := json.Unmarshal(readBody(t, r.Body), &openaiBody); err != nil {
			t.Error(err)
		}
		_, _ = w.Write([]byte(`{"status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"ok"}]}]}`))
	})
	medium := ask
	medium.Model, medium.ThinkingLevel = "gpt-5-mini", "high"
	if _, err := openai.Complete(context.Background(), medium); err != nil {
		t.Fatal(err)
	}
	if got := string(openaiBody["reasoning"]); got != `{"effort":"high"}` {
		t.Errorf("openai reasoning = %s", got)
	}

	ts := newThinkServer(t, `{"thinking":{"values":["low","medium","high"]}}`)
	graded := ask
	graded.ThinkingLevel = "high"
	if _, err := ts.client.Complete(context.Background(), graded); err != nil {
		t.Fatal(err)
	}
	if got := string(ts.chatWire["think"]); got != `"high"` {
		t.Errorf("ollama think = %s", got)
	}
}

func TestAnAnthropicAdminLevelSetsTheBudget(t *testing.T) {
	var body map[string]json.RawMessage
	client := newAnthropicForTest(t, func(w http.ResponseWriter, r *http.Request) {
		if err := json.Unmarshal(readBody(t, r.Body), &body); err != nil {
			t.Error(err)
		}
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn"}`))
	})
	if _, err := client.Complete(context.Background(), model.Request{
		Model: "claude-haiku-4-5", MaxTokens: 8192, ThinkingFloor: "low", ThinkingLevel: "medium",
		Messages: []model.Message{{Role: "user", Content: "hi"}},
	}); err != nil {
		t.Fatal(err)
	}
	if got := string(body["thinking"]); got != `{"type":"enabled","budget_tokens":4096}` {
		t.Fatalf("thinking = %s", got)
	}
}

func geminiWithLevel(t *testing.T, level string, handler http.HandlerFunc) model.Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	client, err := selectLocalBrain(ProviderConfig{Provider: providerGemini, BaseURL: srv.URL, Model: "gemini-3.5-flash", ThinkingLevel: level},
		cloudKeyFor("gemini", testGeminiKey))
	if err != nil {
		t.Fatal(err)
	}
	return client
}

// Each rung records the routing it was sent: two tiers, two snapshots, and a
// second call reuses the first's rows.
func TestTwoTiersRecordTheirOwnWireBlocks(t *testing.T) {
	fcs := &fakeCallStore{}
	r := twoRungRouter(t, stubClient{err: errors.New("down")}, stubClient{resp: model.Response{Text: "ok", InputTokens: 1, OutputTokens: 1}}, fcs)
	broker := func(sort string) ProviderConfig {
		return ProviderConfig{
			Provider: providerOpenAICompatible, BaseURL: "https://openrouter.ai/api", Model: "m",
			Routing: &OpenRouterRouting{Provider: OpenRouterProvider{Sort: &OpenRouterSort{By: sort}}},
		}
	}
	cfg := RoutingConfig{Tiers: map[Tier]ProviderConfig{TierLocalSmall: broker(SortThroughput), TierCheapCloud: broker(SortLatency)}}
	r.install(r.binding().withConfig(cfg, nil))

	for range 2 {
		if _, _, err := r.serveCompletion(wsCtx(), TaskColdStart, []Tier{TierLocalSmall, TierCheapCloud}, model.Request{}); err != nil {
			t.Fatal(err)
		}
	}

	if len(fcs.recorded) != 4 || *fcs.recorded[0].ConfigHash == *fcs.recorded[1].ConfigHash || *fcs.recorded[0].ConfigHash != *fcs.recorded[2].ConfigHash {
		t.Fatalf("rows %+v", fcs.recorded)
	}
	sorts := map[string]bool{}
	for _, snap := range fcs.configSnapshots {
		var params struct {
			Provider   struct{ Sort string } `json:"provider"`
			DeadlineMs int64                 `json:"deadline_ms"`
		}
		if err := json.Unmarshal(snap.ProviderParams, &params); err != nil {
			t.Fatal(err)
		}
		if params.DeadlineMs != CallCeiling.Milliseconds() {
			t.Errorf("deadline_ms = %d", params.DeadlineMs)
		}
		if bytes.Contains(snap.ProviderParams, []byte("embed_dimensions")) {
			t.Errorf("a completion records the embedding lane's dimensions: %s", snap.ProviderParams)
		}
		sorts[params.Provider.Sort] = true
	}
	if !sorts[SortThroughput] || !sorts[SortLatency] {
		t.Fatalf("snapshots %s", fcs.configSnapshots)
	}
}

// swappingClient fails, saving new overrides as it does: a save that lands
// while a call is between rungs.
type swappingClient struct {
	stubClient
	router *Router
	next   TaskOverrides
}

func (s swappingClient) Complete(context.Context, model.Request) (model.Response, error) {
	s.router.SetTaskOverrides(s.next)
	return model.Response{}, errors.New("down")
}

func TestOneCallKeepsTheSettingsItStartedWith(t *testing.T) {
	fcs := &fakeCallStore{}
	fast := &deadlineClient{}
	r := twoRungRouter(t, stubClient{}, fast, fcs)
	r.install(r.binding().withConfig(RoutingConfig{}, nil))
	r.SetTaskOverrides(TaskOverrides{TaskColdStart: {AttemptTimeoutMs: 120000}})
	r.install(binding{
		clients:   map[Tier]model.Client{TierLocalSmall: swappingClient{router: r, next: TaskOverrides{TaskColdStart: {AttemptTimeoutMs: 30000}}}, TierCheapCloud: fast},
		routeMeta: r.binding().routeMeta,
	}.withConfig(RoutingConfig{}, nil))

	if _, _, err := r.serveCompletion(wsCtx(), TaskColdStart, []Tier{TierLocalSmall, TierCheapCloud}, model.Request{}); err != nil {
		t.Fatal(err)
	}

	if fast.deadline <= 60*time.Second {
		t.Fatalf("the second rung ran under %s, the override saved mid-call", fast.deadline)
	}
	for _, snap := range fcs.configSnapshots {
		var params struct {
			DeadlineMs int64 `json:"deadline_ms"`
		}
		if err := json.Unmarshal(snap.ProviderParams, &params); err != nil {
			t.Fatal(err)
		}
		if params.DeadlineMs != 120000 {
			t.Errorf("recorded deadline_ms %d, want the 120000 the call was sent under", params.DeadlineMs)
		}
	}
}

// A field written empty is not the same as one left out, and a null body is
// not an empty one: both are refused rather than read as "clear".
func TestAnOverrideWrittenEmptyIsRefusedNotCleared(t *testing.T) {
	if _, err := TaskOverridesFromWire(nil); err == nil {
		t.Error("a null body was read as clearing every override")
	}
	zero, empty := 0, crmcontracts.AiTaskOverrideThinking("")
	_, err := TaskOverridesFromWire(crmcontracts.AiTaskOverrides{
		"cold_start": {AttemptTimeoutMs: &zero, Thinking: &empty},
	})
	var faults routingFaults
	if !errors.As(err, &faults) || len(faults) != 2 {
		t.Fatalf("err = %v, want a fault on each field written empty", err)
	}
	if got, err := TaskOverridesFromWire(crmcontracts.AiTaskOverrides{"cold_start": {}}); err != nil || got[TaskColdStart] != (TaskOverride{}) {
		t.Errorf("an override with nothing written = %+v, %v; want the task reset", got, err)
	}
}

// An admin's minimal on a family whose default is none is sent as low, the
// same rule a floor follows there.
func TestAMinimalOverrideOnANoneFamilyIsSentAsLow(t *testing.T) {
	var body map[string]json.RawMessage
	client := newOpenAIForTest(t, func(w http.ResponseWriter, r *http.Request) {
		if err := json.Unmarshal(readBody(t, r.Body), &body); err != nil {
			t.Error(err)
		}
		_, _ = w.Write([]byte(`{"status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"ok"}]}]}`))
	})
	ask := model.Request{Model: "gpt-5.1", ThinkingLevel: "minimal", Messages: []model.Message{{Role: "user", Content: "hi"}}}
	if _, err := client.Complete(context.Background(), ask); err != nil {
		t.Fatal(err)
	}
	if got := string(body["reasoning"]); got != `{"effort":"low"}` {
		t.Errorf("reasoning = %s, want low", got)
	}
}

// `max_price: {}` caps nothing and sends nothing; a reasoning budget in
// tokens replaces a site's floor as an effort would.
func TestAnEmptyPriceCapIsUnsetAndATokenBudgetTakesNoFloor(t *testing.T) {
	r, err := DecodeRouting("routing", []byte(`{"provider":{"max_price":{}}}`))
	if err != nil || r.Provider.MaxPrice != nil {
		t.Fatalf("max_price {} = %+v, %v; want unset", r, err)
	}
	budget := 2048
	binding := ProviderConfig{
		Provider: providerOpenAICompatible, Model: "m", BaseURL: "https://openrouter.ai/api",
		Routing: &OpenRouterRouting{Reasoning: &OpenRouterReasoning{MaxTokens: &budget}},
	}
	if openRouterTakesThinkingFloor(binding) {
		t.Error("a binding that sends its own reasoning budget was reported as taking the site floor")
	}
}
