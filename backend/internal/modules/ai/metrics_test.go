// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
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

func mustContain(t *testing.T, out string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(out, want) {
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
		"margince_ai_tokens_total{"+servedLabels+`,class="completion",direction="out"} 40`,
		// Reasoning is billed as output and must roll up with it, or the
		// direction split understates what the completion cost.
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
	mustContain(t, out, `model="evilmodel\"\\"`)
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

// WriteProcessMetrics is what both roles wire, and it must reach the same
// collector every Router increments — the indirection it replaced (a method on
// Router, wrapped by one on ModelPath) read as per-router and cost the worker
// its whole AI surface.
func TestWriteProcessMetricsRendersTheProcessWideCollector(t *testing.T) {
	sharedCallMetrics.observe(served(func(c *Call) { c.Task = "process_wide_probe" }))

	var b strings.Builder
	WriteProcessMetrics(&b)

	if !strings.Contains(b.String(), `task="process_wide_probe"`) {
		t.Errorf("WriteProcessMetrics did not render the collector every Router increments:\n%s", b.String())
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
	mustContain(t, out, `provider="fake",model="fake",served_identity_source="response"`)
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

	mustContain(t, render(m), `model="llama3.1:70b-q4",served_identity_source="echo"`)
}
