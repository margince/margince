// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"os"
	"strconv"
	"strings"
	"testing"
)

// served is one attempt's worth of the labels every AI family is keyed by, so
// a test that cares about one dimension does not restate the other four.
func served(mut ...func(*Call)) Call {
	c := Call{
		Task: "cold_start", Tier: "cheap_cloud", Provider: "openai",
		ModelID: "gpt-5-mini", ServedModel: "gpt-5-mini",
		ServedIdentitySource: servedIdentitySourceResponse,
	}
	for _, m := range mut {
		m(&c)
	}
	return c
}

const servedLabels = `provider="openai",model="gpt-5-mini",served_identity_source="response",task="cold_start",tier="cheap_cloud"`

func render(m *callMetrics) string {
	var b strings.Builder
	m.WritePrometheus(&b)
	return b.String()
}

// mustContain matches each want as a WHOLE line. Without the terminator
// `...} 1` is satisfied by `...} 100`, and a spec that cannot tell one call
// from a hundred is pinning nothing.
func mustContain(t *testing.T, out string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(out, want+"\n") {
			t.Errorf("missing from the exposition:\n\t%s\ngot:\n%s", want, out)
		}
	}
}

// The route identity every family carries. Provider alone folds a frontier
// model and a cheap one into one series, which is the shape this replaced.
func TestEveryFamilyIsKeyedByProviderAndModel(t *testing.T) {
	m := newCallMetrics()
	m.observeAttempt(served(func(c *Call) { c.TokensIn, c.TokensOut, c.LatencyMS = 10, 5, 1200 }))
	m.observe(served())

	mustContain(t, render(m),
		"margince_ai_calls_total{"+servedLabels+"} 1",
		"margince_ai_call_attempts_total{"+servedLabels+"} 1",
		"margince_ai_tokens_total{"+servedLabels+`,class="prompt",direction="in"} 10`,
		"margince_ai_tokens_total{"+servedLabels+`,class="completion",direction="out"} 5`,
		"margince_ai_call_duration_seconds_count{"+servedLabels+"} 1",
	)
}

// The subtraction PriceCall owns, asserted from the metrics side.
//
// TokensIn is cache-inclusive, so a split that reported it whole beside the two
// cache classes would count cached input twice — and it would do so inside
// sum by (direction), which is what the token panel actually queries. 100 in,
// of which 30 were a cache read and 20 a cache write, is 50 plain tokens.
func TestTheTokenClassesAreDisjointSoDirectionDoesNotDoubleCount(t *testing.T) {
	m := newCallMetrics()
	m.observeAttempt(served(func(c *Call) {
		c.TokensIn, c.CachedTokens, c.CacheWriteTokens = 100, 30, 20
		c.TokensOut, c.ReasoningTokens = 40, 15
	}))

	out := render(m)
	mustContain(t, out,
		"margince_ai_tokens_total{"+servedLabels+`,class="prompt",direction="in"} 50`,
		"margince_ai_tokens_total{"+servedLabels+`,class="cached_read",direction="in"} 30`,
		"margince_ai_tokens_total{"+servedLabels+`,class="cache_write",direction="in"} 20`,
		// 40 output of which 15 was reasoning: OutputTokens is
		// reasoning-inclusive, so the plain completion bucket is 25.
		"margince_ai_tokens_total{"+servedLabels+`,class="completion",direction="out"} 25`,
		"margince_ai_tokens_total{"+servedLabels+`,class="reasoning",direction="out"} 15`,
	)
	if strings.Contains(out, `class="prompt",direction="in"} 100`) {
		t.Error("the prompt class reported TokensIn whole; cached input is counted twice in sum by (direction)")
	}
}

// A provider reporting more cached tokens than it reported input is defensive,
// not a contract violation to trust blindly — and the floor is what stops it
// from becoming a negative counter, which Prometheus reads as a reset.
func TestAnOverReportedCacheNeverCountsNegativePromptTokens(t *testing.T) {
	m := newCallMetrics()
	m.observeAttempt(served(func(c *Call) { c.TokensIn, c.CachedTokens = 10, 40 }))

	if strings.Contains(render(m), `class="prompt",direction="in"} -`) {
		t.Errorf("a negative prompt-token count reached the exposition:\n%s", render(m))
	}
}

// Attempts against terminals is the whole reason both are counted: a tier that
// fails over on every call is otherwise identical to one that never does.
func TestAttemptsCountEveryRungAndCallsCountOnlyTheTerminal(t *testing.T) {
	m := newCallMetrics()
	failed := served(func(c *Call) { c.ErrorSentinel = "provider_error" })
	m.observeAttempt(failed)
	m.observeAttempt(served())
	m.observe(served())

	mustContain(t, render(m),
		"margince_ai_calls_total{"+servedLabels+"} 1",
		"margince_ai_call_attempts_total{"+servedLabels+"} 2",
		"margince_ai_call_errors_total{"+servedLabels+`,sentinel="provider_error"} 1`,
	)
}

// A rate-limited provider and a broken one produced the same graph before the
// sentinel label, which is the difference an operator reads the panel for.
func TestErrorsAreSplitByTheSentinelTheRouterClassified(t *testing.T) {
	m := newCallMetrics()
	m.observeAttempt(served(func(c *Call) { c.ErrorSentinel = "rate_limited" }))
	m.observeAttempt(served(func(c *Call) { c.ErrorSentinel = "timeout" }))
	m.observeAttempt(served(func(c *Call) { c.ErrorSentinel = "timeout" }))

	mustContain(t, render(m),
		"margince_ai_call_errors_total{"+servedLabels+`,sentinel="rate_limited"} 1`,
		"margince_ai_call_errors_total{"+servedLabels+`,sentinel="timeout"} 2`,
	)
}

// A cache hit measures a map lookup. Recording it would drag every percentile
// toward zero as the cache warmed — the latency panel would improve while
// nothing about the provider had.
func TestACacheHitIsCountedButNotTimed(t *testing.T) {
	m := newCallMetrics()
	m.observeAttempt(served(func(c *Call) { c.CacheHit, c.LatencyMS = true, 0 }))
	m.observeAttempt(served(func(c *Call) { c.LatencyMS = 2000 }))

	mustContain(t, render(m),
		"margince_ai_call_cache_hits_total{"+servedLabels+"} 1",
		// One timed sample, not two: the hit is absent from the histogram.
		"margince_ai_call_duration_seconds_count{"+servedLabels+"} 1",
		"margince_ai_call_duration_seconds_sum{"+servedLabels+"} 2",
	)
}

func TestADegradedAttemptIsCounted(t *testing.T) {
	m := newCallMetrics()
	m.observeAttempt(served(func(c *Call) { c.Degraded = true }))

	mustContain(t, render(m), "margince_ai_call_degraded_total{"+servedLabels+"} 1")
}

func TestFinishReasonsAreCounted(t *testing.T) {
	m := newCallMetrics()
	m.observeAttempt(served(func(c *Call) { c.FinishReason = "length" }))

	mustContain(t, render(m), "margince_ai_call_finish_reasons_total{"+servedLabels+`,reason="length"} 1`)
}

// The model label is the one value here an operator types: PUT /ai/routing
// accepts any string and ValidateTierBinding does not constrain Model. Go's %q
// would render a tab as \t, which the Prometheus text parser rejects — and it
// rejects the WHOLE scrape, so one stray byte in one binding would take every
// family in the process off the dashboard at once.
func TestAHostileModelIdCannotBreakTheWholeScrape(t *testing.T) {
	m := newCallMetrics()
	m.observeAttempt(served(func(c *Call) { c.ServedModel = "evil\t\x01model\"\\" }))

	out := render(m)
	for _, illegal := range []string{`\t`, `\x`, `\u`} {
		if strings.Contains(out, illegal) {
			t.Errorf("the exposition carries %s, which Prometheus refuses as an invalid escape:\n%s", illegal, out)
		}
	}
	if !strings.Contains(out, "model=\"evil\uFFFD\uFFFDmodel\\\"\\\\\"") {
		t.Errorf("the hostile identity did not survive escaping intact:\n%s", out)
	}
}

// Two sources landing on one collector must render each family's header once
// per call: a duplicated family makes a strict scraper reject the whole scrape.
func TestEachFamilyHeaderIsRenderedExactlyOnce(t *testing.T) {
	m := newCallMetrics()
	m.observeAttempt(served(func(c *Call) { c.TokensIn = 10 }))
	m.observeAttempt(served(func(c *Call) {
		c.Task, c.Tier, c.Provider = "offer_draft", "premium", "anthropic"
		c.ModelID, c.ServedModel = "claude-opus-4-8", "claude-opus-4-8"
		c.TokensIn = 20
	}))

	out := render(m)
	for _, name := range []string{
		"margince_ai_calls_total",
		"margince_ai_call_attempts_total",
		"margince_ai_call_errors_total",
		"margince_ai_call_finish_reasons_total",
		"margince_ai_call_degraded_total",
		"margince_ai_call_cache_hits_total",
		"margince_ai_call_duration_seconds",
		"margince_ai_tokens_total",
		"margince_ai_company_context_bytes_total",
		"margince_ai_company_context_tokens_estimate_total",
	} {
		if n := strings.Count(out, "# HELP "+name+" "); n != 1 {
			t.Errorf("# HELP %s appeared %d times, want 1:\n%s", name, n, out)
		}
		if n := strings.Count(out, "# TYPE "+name+" "); n != 1 {
			t.Errorf("# TYPE %s appeared %d times, want 1:\n%s", name, n, out)
		}
	}
}

// Company context is keyed by task alone, and is counted once per logical call
// rather than once per rung: the same context is re-sent by every attempt, and
// counting each would report a retry as more context supplied.
func TestCompanyContextIsCountedPerLogicalCall(t *testing.T) {
	m := newCallMetrics()
	m.observeAttempt(served(func(c *Call) { c.ContextBytes, c.ContextTokensEstimate = 400, 100 }))
	m.observeAttempt(served(func(c *Call) { c.ContextBytes, c.ContextTokensEstimate = 400, 100 }))
	m.observe(served(func(c *Call) { c.ContextBytes, c.ContextTokensEstimate = 400, 100 }))

	mustContain(t, render(m),
		`margince_ai_company_context_bytes_total{task="cold_start"} 400`,
		`margince_ai_company_context_tokens_estimate_total{task="cold_start"} 100`,
	)
}

// TestSeparatelyAssembledRoutersShareOneCollector pins the production
// wiring itself: every Router assembled in this process observes into the
// same process-wide collector, so /metrics reports one honest total across
// separately-constructed lanes and renders it once. Without this, two
// wired surfaces regressing to private collectors would silently split
// the totals while the collector-level tests above kept passing.
func TestSeparatelyAssembledRoutersShareOneCollector(t *testing.T) {
	a := assembleRouter(nil, nil, ProfileCloudFrontier, stubMeter{}, unlimitedBudget{}, nil, nil, false, nil)
	b := assembleRouter(nil, nil, ProfileCloudFrontier, stubMeter{}, unlimitedBudget{}, nil, nil, false, nil)
	if a.metrics != b.metrics {
		t.Fatal("two assembled routers hold different collectors; /metrics totals would split across lanes")
	}
	if a.metrics != sharedCallMetrics {
		t.Fatal("assembled router does not observe into the process-wide collector")
	}
}

// WriteProcessMetrics is what both roles wire, and it must render the same
// collector every Router increments — a renderer bound to one router would let
// a role wire it and believe the process was covered.
//
// Asserted by IDENTITY rather than by observing into the global and reading it
// back: a probe series written here would outlive this test and make any later
// assertion over the shared collector depend on run order, which is the trap
// router_attempts_test.go already sidesteps with a private collector.
func TestWriteProcessMetricsRendersTheProcessWideCollector(t *testing.T) {
	var direct, viaExport strings.Builder
	sharedCallMetrics.WritePrometheus(&direct)
	WriteProcessMetrics(&viaExport)

	if direct.String() != viaExport.String() {
		t.Errorf("WriteProcessMetrics rendered something other than the process-wide collector:\n%s",
			viaExport.String())
	}
}

// A tier bound without an explicit model id — every --ai-fake deployment, and
// any operator who left the field blank — must not ship a literal model="" on
// every series it publishes. servedIdentity already resolved the best
// available identity; the label carries that, and the source grades it.
func TestABindingWithNoConfiguredModelStillNamesWhatServed(t *testing.T) {
	m := newCallMetrics()
	m.observe(served(func(c *Call) {
		c.Provider, c.ModelID = "fake", ""
		c.ServedModel, c.ServedIdentitySource = "fake", servedIdentitySourceResponse
	}))

	out := render(m)
	if strings.Contains(out, `model=""`) {
		t.Errorf("an empty model label reached the exposition:\n%s", out)
	}
	if !strings.Contains(out, `provider="fake",model="fake",served_identity_source="response"`) {
		t.Errorf("the served identity did not reach the label:\n%s", out)
	}
}

// The other half of the same rule: an OpenAI-compatible wire only echoes the
// requested model back, so the label must be graded "echo" rather than passed
// off as a vendor confirming what ran.
func TestAnEchoedIdentityIsLabelledAsAnEcho(t *testing.T) {
	m := newCallMetrics()
	m.observe(served(func(c *Call) {
		c.Provider = providerOpenAICompatible
		c.ServedModel, c.ServedIdentitySource = "llama3.1:70b-q4", servedIdentitySourceEcho
	}))

	if !strings.Contains(render(m), `model="llama3.1:70b-q4",served_identity_source="echo"`) {
		t.Errorf("an echoed identity was not graded as an echo:\n%s", render(m))
	}
}

// The model label is PROVIDER WIRE INPUT — every adapter reads it verbatim off
// the response body — so a broker answering with a fresh identity per call
// would otherwise mint a key per call, each retained for the process lifetime
// and re-rendered on every scrape across roughly twenty lines.
func TestAProviderCannotMintUnboundedSeries(t *testing.T) {
	m := newCallMetrics()
	for i := range maxRouteSeries * 3 {
		m.observeAttempt(served(func(c *Call) { c.ServedModel = "model-" + strconv.Itoa(i) }))
	}

	if held := len(m.attempts); held > maxRouteSeries+1 {
		t.Errorf("the collector holds %d label sets after %d distinct model identities; "+
			"the cap plus its one overflow key is %d", held, maxRouteSeries*3, maxRouteSeries+1)
	}
	if !strings.Contains(render(m), overflowLabel) {
		t.Errorf("nothing folded into the overflow key:\n%s", render(m))
	}
}

// The overflow key must be ONE series, not one per task: an overflow bucket
// that still carries a dimension the wire influences grows exactly as the thing
// it was meant to bound. httpserver's route histogram learned this twice.
func TestTheOverflowKeyCannotItselfGrow(t *testing.T) {
	m := newCallMetrics()
	for i := range maxRouteSeries * 2 {
		m.observeAttempt(served(func(c *Call) {
			c.ServedModel = "model-" + strconv.Itoa(i)
			c.Task = Task("task-" + strconv.Itoa(i))
			c.Tier = Tier("tier-" + strconv.Itoa(i))
		}))
	}

	overflowed := 0
	for k := range m.attempts {
		if k.model == overflowLabel {
			overflowed++
		}
	}
	if overflowed > 1 {
		t.Errorf("%d overflow series exist; the bucket past the cap grows with the traffic it bounds", overflowed)
	}
}

// A megabyte model identity would be carried back on every scrape, on every
// line of every family keyed by it, for the life of the process.
func TestALongModelIdentityIsCutAndSaysSo(t *testing.T) {
	m := newCallMetrics()
	m.observeAttempt(served(func(c *Call) { c.ServedModel = strings.Repeat("m", 5000) }))

	out := render(m)
	if len(out) > 10_000 {
		t.Errorf("one attempt rendered %d bytes of exposition; the identity was not bounded", len(out))
	}
	if !strings.Contains(out, strings.Repeat("m", maxLabelLen)+`..."`) {
		t.Errorf("the identity was neither cut nor marked as cut:\n%s", out)
	}
}

// A provider's stop reason is a string it chooses, so the label folds to a
// closed set. The raw value stays on the ai_call row, where no series count
// depends on it.
func TestAnUnknownFinishReasonFoldsRatherThanMintingASeries(t *testing.T) {
	m := newCallMetrics()
	m.observeAttempt(served(func(c *Call) { c.FinishReason = "stop" }))
	m.observeAttempt(served(func(c *Call) { c.FinishReason = "vendor_specific_novelty" }))

	out := render(m)
	mustContain(t, out,
		"margince_ai_call_finish_reasons_total{"+servedLabels+`,reason="stop"} 1`,
		"margince_ai_call_finish_reasons_total{"+servedLabels+`,reason="other"} 1`,
	)
	if strings.Contains(out, "vendor_specific_novelty") {
		t.Errorf("an unfolded provider stop reason became a label:\n%s", out)
	}
}

// The constraint metricsrender.go's header states, held rather than asserted in
// prose: every family this package emits must also appear as a string LITERAL
// in the renderer's source.
//
// backend/gates/metricsuffix_test.go reads those literals to decide whether
// `_total` means counter. A name that reached its header through a variable
// would be invisible there, and the tree-wide floor would still pass — so the
// families would silently stop being judged, which is precisely the shape the
// header warns about. Without this test that warning was only a comment.
func TestEveryEmittedFamilyNameIsALiteralInTheRenderer(t *testing.T) {
	source, err := os.ReadFile("metricsrender.go")
	if err != nil {
		t.Fatalf("reading the renderer: %v", err)
	}

	var out strings.Builder
	newCallMetrics().WritePrometheus(&out)

	emitted := 0
	for _, line := range strings.Split(out.String(), "\n") {
		name, found := strings.CutPrefix(line, "# TYPE ")
		if !found {
			continue
		}
		emitted++
		name = strings.Fields(name)[0]
		if !strings.Contains(string(source), `"`+name+`"`) {
			t.Errorf("%s is emitted but never spelled as a literal in metricsrender.go, so the "+
				"suffix census cannot see it and _total stops meaning counter for it", name)
		}
	}
	if emitted == 0 {
		t.Fatal("no families were emitted, so this test proves nothing about the ones that are")
	}
}

// The output side of the same invariant the prompt class carries.
// model.Response's OutputTokens is reasoning-INCLUSIVE — gemini.go adds
// thinking tokens into it so the budget meter charges true spend — so reporting
// it whole beside the reasoning class counts reasoning twice, inside
// sum by (direction) on `out`.
func TestReasoningIsNotCountedTwiceOnTheOutputSide(t *testing.T) {
	m := newCallMetrics()
	m.observeAttempt(served(func(c *Call) { c.TokensOut, c.ReasoningTokens = 100, 40 }))

	out := render(m)
	mustContain(t, out,
		"margince_ai_tokens_total{"+servedLabels+`,class="completion",direction="out"} 60`,
		"margince_ai_tokens_total{"+servedLabels+`,class="reasoning",direction="out"} 40`,
	)
	if strings.Contains(out, `class="completion",direction="out"} 100`) {
		t.Error("the completion class reported TokensOut whole; reasoning is counted twice in sum by (direction)")
	}
}

func TestAnOverReportedReasoningNeverCountsNegativeCompletionTokens(t *testing.T) {
	m := newCallMetrics()
	m.observeAttempt(served(func(c *Call) { c.TokensOut, c.ReasoningTokens = 10, 40 }))

	if strings.Contains(render(m), `class="completion",direction="out"} -`) {
		t.Errorf("a negative completion-token count reached the exposition:\n%s", render(m))
	}
}
