// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build e2e_llm

package aicert_test

// The certification lane's PAID, LIVE run: drives a real candidate
// router (and a real judge router) over whatever the corpus holds, on a
// real routing config. Excluded from every ordinary lane (unit,
// integration, `make check`) by the e2e_llm build tag — the same
// convention compose/sitereade2e_test.go's TestSiteReadE2EGradionQualityFloor
// uses — so it never runs, let alone silently "passes" by doing
// nothing, in a lane that has not explicitly opted into real network
// and real spend. A `make e2e-ai` target invokes this with `-tags
// e2e_llm` plus the env vars below.
//
// Once entered (the build tag is set), MARGINCE_AICERT is still checked
// at runtime and its absence FAILS the test — t.Skip is forbidden by
// this repo's test culture: a skipped gate must never read the same as
// a passing one. The remaining env vars name what to certify.

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/aicert"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/runtimeenv"
)

// TestE2ECertify runs one certification pass against the configured
// routing file. MARGINCE_AI_ROUTING is the only hard requirement beyond
// the lane gate itself; the rest default to "certify everything the
// corpus covers, on the routing file's own bindings, Run's own default
// repeat count."
// logResolvedBindings prints what a routed run decided BEFORE it spends: which
// rung each shipped task landed on and the model bound there. A paid run that
// measured the wrong deployment is only obvious from this list.
func logResolvedBindings(t *testing.T, routing ai.RoutingConfig) {
	t.Helper()
	for _, task := range ai.AllTasks() {
		if ai.Status(task) != ai.StatusShipped {
			continue
		}
		lead := ai.LeadingTier(task)
		if b, ok := routing.Tiers[lead]; ok {
			t.Logf("routed: %-34s %-12s %s:%s", task, lead, b.Provider, b.Model)
		}
	}
}

// judgeBaseURL is the host root the grader is reached on.
//
// It falls back to the CANDIDATE's host only for an openai_compatible judge,
// which fails closed without one and in practice rides the same broker. A native
// vendor is left empty on purpose: inheriting a broker host would point, say, an
// anthropic judge at OpenRouter and grade through a serving path the record does
// not name. JUDGE_BASE_URL= still overrides both.
func judgeBaseURL() string {
	if own := os.Getenv("MARGINCE_AICERT_JUDGE_BASE_URL"); own != "" {
		return own
	}
	if strings.HasPrefix(os.Getenv("MARGINCE_AICERT_JUDGE_MODEL"), "openai_compatible:") {
		return os.Getenv("MARGINCE_AICERT_BASE_URL")
	}
	return ""
}

// TestE2ECertify is the paid certification lane: it drives the shipped corpus
// against real models and writes one record per task.
//
// What to certify is stated, never inherited from a file on the runner's disk:
// ROUTING= names a deployment and certifies the model bound at each task's first
// bound ladder rung, MODEL= names one candidate. Exactly one of the two.
func TestE2ECertify(t *testing.T) {
	if os.Getenv("MARGINCE_AICERT") == "" {
		t.Fatal("TestE2ECertify requires MARGINCE_AICERT=1 (set by `make e2e-ai`) — " +
			"this lane costs real tokens and real network, so it never runs implicitly")
	}
	// Two ways to say what to certify, and they are different questions.
	//
	// ROUTING= names a DEPLOYMENT: its seeds.ai_routing binds a model per tier,
	// and each task is certified against the model at its leading rung — the one
	// that would actually serve it. MODEL= names one candidate and binds it to
	// every tier, which is how a prompt fix is A/B'd against a single model.
	//
	// Neither is a default read off the runner's disk. A verdict recorded against
	// whatever a file happened to bind was never comparable with anybody else's,
	// so the path is stated and the resolved model is printed below.
	var routing *ai.RoutingConfig
	var binding ai.ProviderConfig
	routingPath, modelSpec := os.Getenv("MARGINCE_AICERT_ROUTING"), os.Getenv("MARGINCE_AICERT_MODEL")
	// Refused HERE, because the runner's own both-are-set check cannot see this:
	// under ROUTING= the branch below never parses MODEL=, so cfg.Binding stays
	// zero and the refusal never fires. An engineer who passed both would believe
	// they were A/B-ing one model while paying to certify a whole deployment.
	if routingPath != "" && modelSpec != "" {
		t.Fatalf("both ROUTING=%s and MODEL=%s were given — the first certifies the models a deployment binds, "+
			"the second one model you name; a run cannot report both, so pass one", routingPath, modelSpec)
	}
	if routingPath != "" && os.Getenv("MARGINCE_AICERT_UPSTREAM") != "" {
		t.Fatal("MARGINCE_AICERT_UPSTREAM is only read alongside MODEL=, and ROUTING= was given — " +
			"a deployment's tiers carry their own bindings, so the preferences would have been " +
			"accepted and then applied to nothing")
	}
	if routingPath != "" {
		resolved, rerr := compose.RoutingFromDeployConfig(routingPath, runtimeenv.Development)
		if rerr != nil {
			t.Fatalf("MARGINCE_AICERT_ROUTING=%s: %v", routingPath, rerr)
		}
		routing = &resolved
		logResolvedBindings(t, resolved)
	} else {
		parsed, perr := ai.ParseBinding(modelSpec, os.Getenv("MARGINCE_AICERT_BASE_URL"))
		if perr != nil {
			t.Fatalf("MARGINCE_AICERT_MODEL: %v", perr)
		}
		parsed.Routing = candidateRouting(t)
		binding = parsed
	}

	// The judge is a SECOND model on purpose: one grading itself is certified by
	// construction. Run refuses the two being equal before a call is paid for.
	judge, err := ai.ParseBinding(os.Getenv("MARGINCE_AICERT_JUDGE_MODEL"), judgeBaseURL())
	if err != nil {
		t.Fatalf("MARGINCE_AICERT_JUDGE_MODEL: %v", err)
	}

	// repeats stays 0 (Run's own "default to 3" per RunnerConfig.Repeats'
	// own doc) when MARGINCE_AICERT_RUNS is unset — this lane restates no
	// default the runner already owns.
	var repeats int
	if raw := os.Getenv("MARGINCE_AICERT_RUNS"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			t.Fatalf("MARGINCE_AICERT_RUNS=%q is not an integer: %v", raw, err)
		}
		repeats = n
	}

	// The census this build ships, not a stand-in: a run drives the
	// certification case each registered site binds, so the lane certifies the
	// same sites the process roles wire.
	census, err := compose.NewTaskCensus()
	if err != nil {
		t.Fatalf("building the task census: %v", err)
	}

	cfg := aicert.RunnerConfig{
		Census:       census,
		Binding:      binding,
		Routing:      routing,
		JudgeBinding: judge,
		// Under ROUTING= the profile is the FILE's own, not the environment's: it
		// is part of a record's identity and part of what ValidateTierBinding
		// enforces, so taking it from anywhere but the config that names the
		// models would file a record under an environment class nobody declared.
		// Under ROUTING= this is ignored: Run takes the profile from the routing
		// itself (RunnerConfig.recordProfile), so the record's environment class
		// comes from the same document that named the models.
		Profile:    ai.Profile(os.Getenv("MARGINCE_AICERT_PROFILE")),
		TaskFilter: os.Getenv("MARGINCE_AICERT_TASK"),
		Repeats:    repeats,
		CorpusDir:  "corpus",
		RecordDir:  "records",
		// MARGINCE_AICERT_TRACE names a directory for the payload trace
		// (every candidate+judge request/response, ai_call_payload shape);
		// unset = no trace. `make e2e-ai` sets it to the repo-root
		// .tmp/aicert default (TRACE=1); pass TRACE= to disable.
		TraceDir: os.Getenv("MARGINCE_AICERT_TRACE"),
		// MARGINCE_AICERT_RESUME names a directory for the resume journal;
		// unset = every run is paid for. `make e2e-ai` sets it to the repo-root
		// .tmp/aicert/resume default (RESUME=1); pass RESUME= to disable.
		ResumeDir: os.Getenv("MARGINCE_AICERT_RESUME"),
	}

	records, runErr := aicert.Run(context.Background(), cfg, slog.Default())
	if runErr != nil {
		t.Fatalf("certification run failed: %v", runErr)
	}
	if len(records) == 0 {
		t.Fatal("the run produced no records — check MARGINCE_AICERT_TASK against the corpus")
	}
	for _, r := range records {
		t.Logf("%s: %s (reliability=%.2f score_p50=%d self_judged=%v)",
			r.Task, r.Verdict, r.Reliability, r.ScoreP50, r.SelfJudged)
	}
}

// candidateRouting reads MARGINCE_AICERT_UPSTREAM: the broker upstream-selection
// preferences to certify the candidate under, as the JSON of one
// ai.OpenRouterRouting. Nil when unset, which measures the broker's own default
// choice — the baseline every tuned run is compared against.
//
// An env var rather than a config field because that is exactly what is being
// decided: the field set worth making operator-facing is the OUTPUT of these
// runs, and freezing it into the routing schema first would be committing to a
// shape before measuring whether it is the right one.
//
// Unknown keys are refused. A misspelt preference would otherwise be dropped in
// silence and the run would report the baseline's numbers under a tuned run's
// name, which is the one way this measurement can lie without failing.
func candidateRouting(t *testing.T) *ai.OpenRouterRouting {
	t.Helper()
	raw := os.Getenv("MARGINCE_AICERT_UPSTREAM")
	if raw == "" {
		return nil
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	// Decoded through a POINTER so `null` is distinguishable. Into a value it
	// decodes happily and leaves the struct zero, and this function would then
	// hand back a non-nil pointer to an empty block — which is the deliberate
	// "take the broker's own price-weighted routing" opt-out. A run asking for
	// the default and silently getting the opposite is the same lie this
	// function's unknown-key refusal exists to prevent.
	var routing *ai.OpenRouterRouting
	if err := decoder.Decode(&routing); err != nil {
		t.Fatalf("MARGINCE_AICERT_UPSTREAM=%s is not one ai.OpenRouterRouting as JSON: %v", raw, err)
	}
	if routing == nil {
		t.Fatalf("MARGINCE_AICERT_UPSTREAM=%s decodes to null; unset the variable to measure the "+
			"broker's own choice, or write {} to say explicitly that this run wants no preferences", raw)
	}
	// And exactly ONE value. A second is otherwise ignored, so
	// `{"sort":"throughput"} {"sort":"price"}` would run under the first and
	// read as though both had been considered.
	if err := decoder.Decode(new(json.RawMessage)); !errors.Is(err, io.EOF) {
		t.Fatalf("MARGINCE_AICERT_UPSTREAM=%s carries more than one JSON value; pass exactly one object", raw)
	}
	// Unknown keys are refused above; the VALUES have to be checked too, and by
	// the same reasoning. A misspelt sort ("thruput") or quantization ("fp5")
	// decodes cleanly, the broker drops what it does not recognise in silence,
	// and the run then reports the baseline's numbers under a tuned run's name —
	// the exact way this measurement can lie without failing. ai.ParseRouting
	// applies the same check to a config file; this is the env var's door to it.
	if err := routing.Validate(); err != nil {
		t.Fatalf("MARGINCE_AICERT_UPSTREAM=%s: %v", raw, err)
	}
	t.Logf("candidate upstream preferences: %s", raw)
	return routing
}
