// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

// The runner: the one part of this package that actually drives model
// calls. Everything else (scenario.go, promptversion.go, score.go, record.go)
// is a pure library callable without a network or a database; this file
// wires that library to TWO DB-less ai.Router instances, assembled via
// compose.NewLocalRouterForCert (ai.NewLocalRouter over a CallRecorder
// this package supplies, called through brain.go so the raw
// model-client construction stays inside the one seam arch_test.go's
// TestNoModelClientOutsideTheGate enforces) — one
// serving the task under certification (optionally MODEL=-overridden on
// just that task's ladder), one serving the fixed cert_judge task on the
// UNMODIFIED routing config, so a candidate can never grade its own
// homework by construction.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// corpusVersionV1 is this generation's fixed corpus-format stamp: the
// scenario format carries no version field of its own yet, so every
// Record names the same one until a versioning scheme arrives alongside
// a real second version.
const (
	corpusVersionV1 = "v1"
)

// nowFunc is the runner's injectable clock for Record.RanAt. Run's
// signature is pinned with no Clock parameter, so this is this file's
// own seam — the same pattern ai.Router's unexported `now func()
// time.Time` field uses for the same reason: production wants
// time.Now, a test wants a fixed instant.
var nowFunc = time.Now

// RunnerConfig configures one certification run.
type RunnerConfig struct {
	// Census is this build's invocation-site registry, and the only thing that
	// turns a scenario into something runnable: every site's certification case
	// is bound there, and a run drives those cases rather than prompts of its
	// own. Required — see Run.
	Census *aitasks.Registry
	// Binding is what this run certifies, stated outright rather than read from
	// a routing file: the model binding is a stored setting now, and this lane
	// opens no database, so a run says which model it measured instead of
	// inheriting whatever a file on the runner's disk happened to bind.
	//
	// BaseURL carries the broker case. openai_compatible fails closed without
	// one, so a run against OpenRouter or any other OpenAI-wire host supplies it
	// here; a native vendor leaves it empty and gets the vendor default.
	// Required — see Run.
	Binding ai.ProviderConfig // MARGINCE_AICERT_MODEL + MARGINCE_AICERT_BASE_URL
	// JudgeBinding grades the candidate, and is stated separately BECAUSE it
	// must differ: a judge on the candidate's own binding marks its own homework,
	// which is the one result a certification run must never be able to produce.
	// The retired routing file supplied this implicitly — the judge rode the
	// file's binding while MODEL= moved only the candidate — so deleting the file
	// without naming a judge would have collapsed the two silently.
	JudgeBinding ai.ProviderConfig // MARGINCE_AICERT_JUDGE_MODEL + _JUDGE_BASE_URL
	// Profile is the environment class each record is filed under — it is part of
	// a record's identity (its path and its sort key), not a label, so a run says
	// which one it measured rather than inheriting it from a file.
	// Routing, when set, resolves the candidate binding PER TASK from a
	// deployment's own tier→model map instead of Binding naming one model for
	// every task: each task is certified against the model bound at its LEADING
	// ladder rung (ai.LeadingTier), which is the model that would actually serve
	// it, and Profile comes from the file rather than the environment.
	//
	// It exists because a model is not a thing anybody deploys. A hand-typed
	// MODEL= measures whichever model an engineer chose; a deployment binds
	// gpt-oss-120b at local_small and claude-haiku-4.5 at premium, and those are
	// the answers a customer's install depends on. One ROUTING= run covers both.
	//
	// The resolved model still binds EVERY tier for the task under test, exactly
	// as a MODEL= override does — see ladderForTask. Binding each tier to its own
	// real model would let one run be served by two models and pool them into a
	// record with one ServedModel field.
	//
	// Mutually exclusive with Binding: one names a deployment, the other a
	// candidate, and a run that silently preferred one would report a model
	// nobody asked for.
	Routing    *ai.RoutingConfig // MARGINCE_AICERT_ROUTING
	Profile    ai.Profile        // MARGINCE_AICERT_PROFILE
	TaskFilter string            // MARGINCE_AICERT_TASK ("" = all tasks with a corpus)
	Repeats    int               // MARGINCE_AICERT_RUNS: each case's first round, default defaultRepeats
	RecordDir  string
	CorpusDir  string
	// TraceDir, when non-empty, turns on the opt-in payload trace
	// (MARGINCE_AICERT_TRACE): every candidate and judge call's
	// post-stripper request+response is dumped to a JSONL file under this
	// directory and its path printed to stdout. Empty = no trace.
	TraceDir string
	// ResumeDir, when non-empty, turns on the resume journal
	// (MARGINCE_AICERT_RESUME): every scored run is appended to a JSONL file
	// under this directory as it is scored, and a later run in the same
	// six-hour window replays the ones it can instead of paying for them
	// again. Empty = every run is paid for. See resume.go.
	ResumeDir string
}

// validateBindings refuses a run that could not produce a trustworthy verdict,
// before a single paid call is made.
//
// Both bindings are required because there is no routing file to fall back on,
// and the judge that grades the candidate must differ from it (judgeFor) because
// a model grading itself passes by construction. Nothing pairs the two for the
// run, so the difference is checked outright, over the tasks this run certifies.
func validateBindings(cfg RunnerConfig, tasks []ai.Task, log *slog.Logger) error {
	if cfg.Routing != nil && cfg.Binding.Provider != "" {
		return errors.New("both MARGINCE_AICERT_ROUTING and MARGINCE_AICERT_MODEL are set — " +
			"the first certifies the models a deployment binds, the second one model you name; " +
			"a run cannot report both, so pick one")
	}
	if err := refuseUnreachableUpstream(cfg); err != nil {
		return err
	}
	if cfg.Routing != nil {
		return validateRoutedBindings(cfg, tasks, log)
	}
	if cfg.Binding.Provider == "" || cfg.Binding.Model == "" {
		return errors.New("no candidate binding — set MARGINCE_AICERT_MODEL=provider:model " +
			"(and MARGINCE_AICERT_BASE_URL for an openai_compatible host, which fails closed without one)")
	}
	if cfg.JudgeBinding.Provider == "" || cfg.JudgeBinding.Model == "" {
		return errors.New("no judge binding — set MARGINCE_AICERT_JUDGE_MODEL=provider:model; " +
			"the judge is a SECOND model on purpose, and the run has no file to inherit one from")
	}
	if !cfg.Profile.Valid() {
		return fmt.Errorf("MARGINCE_AICERT_PROFILE=%q is not an environment class; a record is filed "+
			"under it, so a run states which one it measured", cfg.Profile)
	}
	// The rule a parsed eu_hosted config meets, asked of the candidate a MODEL=
	// run names: its record is filed under eu_hosted, so the binding has to keep
	// its text in the EU. A ROUTING= run met it when its file was parsed.
	if cfg.Profile == ai.ProfileEUHosted {
		if gap := ai.EURegionPinGap(cfg.Binding); gap != "" {
			return fmt.Errorf("the candidate %s:%s would be filed under eu_hosted, but %s — pin it with "+
				"UPSTREAM='{\"only\":[\"<vendor>/eu\"]}' (MARGINCE_AICERT_UPSTREAM), or run it as PROFILE=cloud_frontier",
				cfg.Binding.Provider, cfg.Binding.Model, gap)
		}
	}
	return refuseSelfJudgedTasks(cfg, tasks)
}

// Run certifies every task named by cfg.TaskFilter (or, when empty,
// every task the corpus carries at least one scenario for): N repeats
// per scenario over a candidate router (MODEL=-overridden on just that
// task's ladder, when set) scored by a second, always-unmodified judge
// router. It writes one Record per task that reaches a verdict and
// returns every Record it wrote. A single task's certification failing
// (a corpus/config problem, or ANY candidate OR judge attempt coming
// back router-degraded) never aborts the others: that task gets no
// record, and its error is folded into the returned error (errors.Join)
// — heard, never swallowed — while every other task still gets its own
// record.
func Run(ctx context.Context, cfg RunnerConfig, log *slog.Logger) ([]Record, error) {
	// A missing census is refused here rather than tolerated per scenario: the
	// cases it binds ARE what a run certifies, so a run without one could only
	// report that it measured nothing, after paying for it.
	if cfg.Census == nil {
		return nil, errors.New("aicert: runner: no census supplied — a run drives the certification case each site binds, so RunnerConfig.Census is required")
	}
	repeats, err := repeatsOrDefault(cfg.Repeats)
	if err != nil {
		return nil, err
	}

	scenarios, err := LoadCorpus(cfg.CorpusDir, cfg.Census)
	if err != nil {
		return nil, fmt.Errorf("aicert: runner: %w", err)
	}

	byTask := groupByTask(scenarios, cfg.TaskFilter)
	if cfg.TaskFilter != "" && len(byTask) == 0 {
		return nil, fmt.Errorf("aicert: runner: task %q has no scenarios under %s", cfg.TaskFilter, cfg.CorpusDir)
	}

	// Refused here rather than per task, over the tasks this run certifies: with no
	// routing file to fall back on, a run with no binding could only report that it
	// measured nothing, after paying for it.
	if err := validateBindings(cfg, sortedTasks(byTask), log); err != nil {
		return nil, fmt.Errorf("aicert: runner: %w", err)
	}

	ctx = ensureWorkspace(ctx)
	// TraceDir empty ⇒ tracing off: trace stays nil and every method no-ops.
	var trace *payloadTrace
	if cfg.TraceDir != "" {
		trace, err = openPayloadTrace(cfg.TraceDir, nowFunc().UTC().Format("20060102T150405Z"))
		if err != nil {
			return nil, fmt.Errorf("aicert: runner: %w", err)
		}
	}
	defer func() {
		if cerr := trace.close(); cerr != nil {
			log.WarnContext(ctx, "aicert: closing payload trace", "err", cerr)
		}
	}()

	// ResumeDir empty ⇒ resuming off: journal stays nil and every method
	// no-ops. Opened here, before the first paid call, so a journal that
	// cannot be loaded or compacted costs nothing to find out about.
	var journal *runJournal
	if cfg.ResumeDir != "" {
		journal, err = openRunJournal(ctx, cfg.ResumeDir, cfg.recordProfile(), nowFunc(), log)
		if err != nil {
			return nil, fmt.Errorf("aicert: runner: %w", err)
		}
	}
	defer func() {
		if cerr := journal.close(); cerr != nil {
			log.WarnContext(ctx, "aicert: closing resume journal", "err", cerr)
		}
	}()

	if err := runPreflights(ctx, cfg, journal, byTask, repeats, log); err != nil {
		return nil, fmt.Errorf("aicert: runner: %w", err)
	}

	var records []Record
	var runErrs []error
	for _, task := range sortedTasks(byTask) {
		written, err := certifyAndWrite(ctx, cfg, task, byTask[task], repeats, trace, journal, log)
		records = append(records, written...)
		if err != nil {
			runErrs = append(runErrs, err)
		}
	}
	return records, errors.Join(runErrs...)
}

// certifyAndWrite certifies one task and writes its records: the LLM record,
// then — when the run binds a decisions lane — each decision site's beside it.
// It returns what it wrote even when a later step failed, so a decision leg
// that fails never costs the task the LLM record already on disk.
func certifyAndWrite(ctx context.Context, cfg RunnerConfig, task ai.Task, scenarios []Scenario, repeats int,
	trace *payloadTrace, journal *runJournal, log *slog.Logger,
) ([]Record, error) {
	binding, judge, err := taskBindings(ctx, cfg, task, log)
	if err != nil {
		return nil, err
	}
	hooks := &certifyHooks{trace: trace, journal: journal.forTask(task, binding, judge)}
	rec, err := certifyTask(ctx, task, scenarios, cfg.Census, binding, judge, cfg.recordProfile(), repeats, log, hooks)
	if err != nil {
		log.ErrorContext(ctx, "aicert: task certification failed — no record written", "task", string(task), "err", err)
		return nil, fmt.Errorf("task %s: %w", task, err)
	}
	if err := WriteRecord(cfg.RecordDir, rec); err != nil {
		return nil, fmt.Errorf("task %s: writing record: %w", task, err)
	}
	decisions, err := certifyDecisionsFor(ctx, cfg, task, scenarios, binding, rec, hooks, log)
	if err != nil {
		log.ErrorContext(ctx, "aicert: decision leg failed — no decision record written", "task", string(task), "err", err)
		return []Record{rec}, fmt.Errorf("task %s: %w", task, err)
	}
	return append([]Record{rec}, decisions...), nil
}

// runPreflights asks every binding the run will use one question before the
// corpus. After the journal is opened, because a restart replaying every run
// sends no LLM call and the chat pre-flight would be its one paid call. The
// decision pre-flight runs regardless: no decision run is journaled, so the
// leg is paid for on every run and its pre-flight is always worth the call.
func runPreflights(ctx context.Context, cfg RunnerConfig, journal *runJournal, byTask map[ai.Task][]Scenario, repeats int, log *slog.Logger) error {
	if journal.replaysEverything(ctx, cfg, byTask, repeats) {
		log.InfoContext(ctx, "aicert: pre-flight skipped — every run replays from the resume journal")
	} else if err := preflight(ctx, cfg, sortedTasks(byTask), nil, log); err != nil {
		return err
	}
	return preflightDecisions(ctx, cfg, byTask, nil, log)
}

// certifyTask runs every scenario for one task over a fresh
// candidate/judge router pair and folds the outcome into one Record.
func certifyTask(ctx context.Context, task ai.Task, scenarios []Scenario, census *aitasks.Registry, binding, judgeBinding ai.ProviderConfig, profile ai.Profile, repeats int, log *slog.Logger, hooks *certifyHooks) (Record, error) {
	candidateCfg, err := ladderForTask("candidate (MARGINCE_AICERT_MODEL, or the rung MARGINCE_AICERT_ROUTING resolved)", binding, candidateRole.profileFor(profile), task)
	if err != nil {
		return Record{}, err
	}
	// The stamp is computed before the first paid call: it drives every case's
	// own request builder, so a corpus this build cannot build a request from is
	// a run that could never have produced a record — found for free rather than
	// after N repeats of real spend.
	scenarioStamps, err := ScenarioStamps(ctx, scenarios, census)
	if err != nil {
		return Record{}, err
	}
	// Folded here rather than re-derived by PromptVersion, which would build every
	// scenario's candidate and grader request a second time — and would leave the
	// one window where a record's task stamp could differ from the fold of its own
	// per-scenario stamps.
	promptVersion := FoldScenarioStamps(scenarioStamps)
	var candidateExtra, judgeExtra []ai.LocalOption
	var trace *payloadTrace
	var journal taskJournal
	maxRuns := adaptiveMaxRuns
	if hooks != nil {
		candidateExtra, judgeExtra, trace, journal = hooks.candidateOpts, hooks.judgeOpts, hooks.trace, hooks.journal
		if hooks.maxRuns > 0 {
			maxRuns = hooks.maxRuns
		}
	}
	// Capture the post-stripper bodies only when a trace will consume them —
	// otherwise the router pays the marshal+strip cost for content nothing reads.
	if trace != nil {
		candidateExtra = append(candidateExtra, ai.WithPayloadCapture())
		judgeExtra = append(judgeExtra, ai.WithPayloadCapture())
	}

	candidateRec := newTraceRecorder()
	candidateOpts := append([]ai.LocalOption{ai.WithoutResultCache(), ai.WithCallStore(candidateRec)}, candidateExtra...)
	candidateRouter, err := compose.NewLocalRouterForCert(candidateCfg, candidateOpts...)
	if err != nil {
		return Record{}, fmt.Errorf("aicert: task %s: candidate router: %w", task, err)
	}

	// The judge NEVER rides the candidate's binding — a model grading itself is
	// certified by construction, which defeats the whole point of a second
	// router. Run refuses the two being equal before a single call is paid for.
	judgeCfg, err := ladderForTask("judge (MARGINCE_AICERT_JUDGE_MODEL / _JUDGE_BASE_URL)", judgeBinding, judgeRole.profileFor(profile), task)
	if err != nil {
		return Record{}, err
	}
	judgeRec := newTraceRecorder()
	judgeOpts := append([]ai.LocalOption{ai.WithoutResultCache(), ai.WithCallStore(judgeRec)}, judgeTransport(judgeBinding)...)
	judgeOpts = append(judgeOpts, judgeExtra...)
	judgeRouter, err := compose.NewLocalRouterForCert(judgeCfg, judgeOpts...)
	if err != nil {
		return Record{}, fmt.Errorf("aicert: task %s: judge router: %w", task, err)
	}

	acc := &taskAccumulation{selfJudgedEveryRun: true}
	driver := taskDriver{
		task: task, census: census, candidateRouter: candidateRouter, candidateRec: candidateRec,
		judgeRouter: judgeRouter, judgeRec: judgeRec, log: log, acc: acc, trace: trace, journal: journal, maxRuns: maxRuns,
	}
	sets, err := driver.runScenarios(ctx, scenarios, scenarioStamps, repeats)
	if err != nil {
		return Record{}, err
	}
	taskVerdict, _ := Verdict(sets...)

	rec := buildRecord(task, taskVerdict, acc, profile, promptVersion)
	rec.CandidateUpstream, rec.JudgeUpstream = ai.UpstreamPreferencesFor(binding), ai.UpstreamPreferencesFor(judgeBinding)
	rec.ThinkingLevel = binding.ThinkingLevel
	rec.JudgeProvider = judgeBinding.Provider
	return rec, nil
}

// taskAccumulation collects the pooled stats certifyTask folds across
// every scenario's repeats for buildRecord, plus the I2 served-identity
// uniformity state: the first run's candidate provider/model is the
// task's baseline, and every later run must match it exactly. A mid-set
// ladder fallback (a transient provider error on any repeat serving that
// run from a DIFFERENT rung's model) must void the whole record rather
// than let it certify "task x provider x model" over scores partly
// produced by another model.
type taskAccumulation struct {
	allResults []RunResult
	latencies  []int64
	// tokensInTotal/tokensOutTotal/cachedTokensTotal/cacheWriteTokensTotal
	// are the pooled per-bucket sums across every run. buildRecord derives
	// MeanTokens from tokensInTotal+tokensOutTotal (an exact sum, so it
	// matches this package's pre-bucketed MeanTokens arithmetic bit for
	// bit) and each MeanBucket from its own total, independently.
	tokensInTotal, tokensOutTotal                           int
	cachedTokensTotal, cacheWriteTokensTotal                int
	passed                                                  int
	provider, servedModel, identitySource, judgeServedModel string
	selfJudgedEveryRun                                      bool
	// anyRunGraded is false until a judge scores a run. selfJudgedEveryRun
	// starts true, so without this a task whose every run was cut off would
	// report SelfJudged beside an empty JudgeServedModel — "every run was
	// graded by the candidate itself" said of a set no judge ever saw.
	anyRunGraded bool
	identitySet  bool
	// contextServed counts the runs served the company context, so the record
	// claims it only when every run was.
	contextServed int
	// certifiedScope is the narrowest scope any run's site covered. A task is
	// one record but not always one site — cold_start ships a one-shot
	// extraction beside three multi-turn conversations — so the record may
	// claim only what its weakest site proved.
	certifiedScope string
	// scenarios is one row per scenario the task ran, in the order it ran
	// them. The pooled numbers above cannot say WHICH scenario failed, and
	// that is the question a failed task actually raises.
	scenarios []ScenarioRecord
}

// addRun folds one scored run into acc, first checking outcome's candidate
// identity against the task's baseline (the first run recorded). Returns
// an error — voiding the whole task's record — when a later run's
// provider or served model diverges from that baseline.
func (acc *taskAccumulation) addRun(task ai.Task, sc Scenario, runIndex int, outcome runOutcome) error {
	withheld := outcome.Withheld != ""
	if acc.identitySet && !withheld && (outcome.Provider != acc.provider || outcome.ServedModel != acc.servedModel) {
		return fmt.Errorf(
			"aicert: task %s scenario %s run %d: candidate served by %s:%s, but run 1 was served by %s:%s — refusing to certify a mixed run set",
			task, sc.Name, runIndex+1, outcome.Provider, outcome.ServedModel, acc.provider, acc.servedModel,
		)
	}
	acc.allResults = append(acc.allResults, outcome.RunResult)
	acc.latencies = append(acc.latencies, outcome.LatencyMS)
	acc.tokensInTotal += outcome.TokensIn
	acc.tokensOutTotal += outcome.TokensOut
	acc.cachedTokensTotal += outcome.CachedTokens
	acc.cacheWriteTokensTotal += outcome.CacheWriteTokens
	// A withheld run names the binding, which stands in only until a served run
	// supplies the identity that actually answered.
	if !withheld || !acc.identitySet {
		acc.provider, acc.servedModel, acc.identitySource = outcome.Provider, outcome.ServedModel, outcome.ServedIdentitySource
	}
	acc.identitySet = acc.identitySet || !withheld
	acc.certifiedScope = aitasks.NarrowerScope(acc.certifiedScope, outcome.CertifiedScope)
	if outcome.ContextApplied {
		acc.contextServed++
	}
	// A run no judge saw says nothing about the judge. Capturing its empty
	// identity would let one truncated run at the END of a set erase the grader
	// from the record, and its empty ServedModel would read as not-self-judged
	// for a run nobody judged at all.
	if !outcome.Ungraded {
		acc.anyRunGraded = true
		acc.judgeServedModel = outcome.JudgeServedModel
		if !selfJudged(outcome.ServedModel, outcome.JudgeServedModel) {
			acc.selfJudgedEveryRun = false
		}
	}
	if outcome.HardPass {
		acc.passed++
	}
	return nil
}

// ensureWorkspace mints a fixed, DB-less workspace principal when ctx
// carries none — the router's own precondition for tracing a call
// (Router.serveAttempt refuses outside a workspace context), mirrored
// from compose/sitereaddebug.go's identical DB-less debug-lane pattern.
func ensureWorkspace(ctx context.Context) context.Context {
	if _, ok := principal.WorkspaceID(ctx); ok {
		return ctx
	}
	return principal.WithWorkspaceID(ctx, ids.NewV7())
}

// buildRecord, seedRateFor, and percentile live in record.go alongside the
// Record type they build — that file already owns "the on-disk Record
// shape," so folding pooled run stats into one is that same concern, not
// this file's own "drive the routers" one.
