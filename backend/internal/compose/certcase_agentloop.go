// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The certification case for one agent_loop site — one scheduled agent's model
// turn on the Surface-B runner: the window the loop builds, and the step the
// loop will accept back.
//
// WHAT IT CERTIFIES IS WHAT RUNS. agent_loop is the engine and each site is one
// scheduled agent. The goal and the tool allowlist are that agent's own,
// resolved by ScheduledAgentSpecByName exactly as the runner service resolves
// them, and the runner narrows the registry to the allowlist itself. A scenario
// supplies only what varies between runs — the occurrence and what retrieval
// seeded — so no scenario can offer a goal or a surface no run is given.
//
// WHAT IT EXERCISES. One turn, driven through the shipped entry point. Run calls
// runner.Run with the runner's own brain and tool seams, so the prompt is the
// loop's own — the system frame carrying this run's boundary rule and the offered
// tool list, and the goal turn carrying the seed grounding fenced by trust tier —
// and the reply is graded by parseStep, the step protocol the loop admits a
// proposal through. Nothing here re-creates any of it.
//
// WHAT IT DOES NOT EXERCISE, which is the loop. The run is bounded to the single
// turn it grades, so no observation is ever fed back, no second prompt is ever
// built, no window is ever elided, and nothing is ever executed, suspended or
// resumed. Site.CertifiedScope() already reports single_turn for this kind; this
// case is that scope and not one step more, and a record built from it may not be
// read as a claim about a run.
//
// WHY A FRESH WINDOW AND NOT A RESUMED ONE. Resume is the loop's other entry
// point and it cannot be driven honestly from a fixture. Its window is rebuilt
// from a stored transcript plus the fence that transcript's spans were written
// with, and the two must be the same fence: a case that minted a fresh marker and
// handed it a snapshot written under another would be telling the model to honour
// a boundary its own text does not carry — the exact failure windowFromSnapshot
// refuses an unminted fence to prevent. A corpus-supplied fence would put a fixed
// nonce back in the corpus, and building a correctly bounded snapshot here would
// mean re-creating the window's own observation format in this file, which is a
// copy of the thing being certified. Seed grounding is how a fresh run is given
// prior context anyway, so a window seeded that way is the shape the loop's own
// scenarios already describe. This case therefore never reaches Resume, and it
// claims nothing about resuming.
//
// WHAT THE EXPECTATION MEANS. The one thing a turn of this site decides: which
// step the model takes — call one of the tools this window offers, or answer the
// goal. That is what the step protocol itself distinguishes, so it is what a
// scenario can assert; the reasoning behind the step is prose, and pinning it
// would fail every model that reached the same step differently, which is what
// the rubric and the judge are for.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/compose/promptlang"
	"github.com/margince/margince/backend/internal/modules/agents/runner"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
	"github.com/margince/margince/backend/internal/shared/ports/model"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

// agentLoopSite names the task in every refusal the case writes; the site's own
// name follows wherever the refusal is about one agent.
const agentLoopSite = "agent_loop"

// agentLoopFinalStep is the expectation token for the step that ends a run. It
// is the protocol's own word for it, and it is the one name a fixture may not
// give a tool — the two would be indistinguishable in an expectation.
const agentLoopFinalStep = "final"

// agentLoopFixture is ONE scheduled run's first turn, in the two things that
// vary between runs of one agent: the occurrence that started it and the seed
// context retrieval returned for it. Everything else the window carries is the
// agent's own and is looked up, never supplied.
//
// The grounding arrives already retrieved, because the certified thing is the
// window built from it, not the search that produced it. What retrieval
// guarantees about it is enforced at Prepare instead.
type agentLoopFixture struct {
	TriggerRef string               `json:"trigger_ref"`
	Grounding  []agentLoopGrounding `json:"grounding"`
}

// agentLoopGrounding is one piece of retrieved evidence, in the shape retrieval
// hands it over: its source and its snippet. It carries no trust tier because
// a run's seed never chooses one — retrievedSeed stamps every seed alike.
type agentLoopGrounding struct {
	SourceID string `json:"source_id"`
	Content  string `json:"content"`
}

// agentLoopCases serves one agent_loop site: the scheduled agent it names.
type agentLoopCases struct{ agent string }

func (c agentLoopCases) Site() aitasks.Site {
	return aitasks.Site{
		Task:    ai.TaskAgentLoop,
		Variant: c.agent,
		Kind:    ai.SiteKindAgentLoop,
	}
}

// Prepare turns one seeded window and the step the scenario expects into a
// runnable case.
//
//nolint:ireturn // PreparedCase IS the seam: one implementation per site behind the one interface the cert lane runs.
func (c agentLoopCases) Prepare(fixture, expected json.RawMessage) (aitasks.PreparedCase, error) {
	// Strict, because a key this shape does not hold is a scenario still
	// supplying a goal or a tool surface of its own, and ignoring it would
	// certify the agent's window while the scenario's author reads theirs.
	var f agentLoopFixture
	decoder := json.NewDecoder(bytes.NewReader(fixture))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&f); err != nil {
		return nil, fmt.Errorf("%s: the fixture is not the shape this site takes — a scheduled agent's "+
			"trigger_ref and its grounding, nothing else: %w", agentLoopSite, err)
	}
	agent, err := c.refuseUnrunnableAgentJob(f)
	if err != nil {
		return nil, err
	}
	job := runner.Job{
		Goal:         agent.Goal,
		TriggerRef:   f.TriggerRef,
		Budget:       agent.Budget,
		Tools:        agent.Tools,
		Grounding:    agentLoopSeedContext(f.Grounding),
		LanguageRule: promptlang.Rule(string(textlang.English)),
	}
	specs := job.Narrow(agentLoopRegistry())
	// A turn that took the right step differs from one that took the wrong step
	// in that step alone, so the expectation is that token — plus, only where a
	// scenario means it, the arguments the step had to be called with.
	var want agentLoopStep
	if err := json.Unmarshal(expected, &want); err != nil {
		return nil, err
	}
	if err := refuseUnreachableAgentStep(want.name, specs); err != nil {
		return nil, err
	}
	if err := refuseUnaskableArguments(want, specs); err != nil {
		return nil, err
	}
	return &agentLoopCase{job: job, expected: want}, nil
}

// refuseUnrunnableAgentJob names a job the scheduler could never have handed the
// runner for this site, and so a window the product never builds, and otherwise
// returns the agent as production resolves it: every run the scheduler starts is
// one named occurrence of one agent.
func (c agentLoopCases) refuseUnrunnableAgentJob(f agentLoopFixture) (runner.AgentSpec, error) {
	agent, ok := ScheduledAgentSpecByName(c.agent)
	if !ok {
		return runner.AgentSpec{}, fmt.Errorf(
			"%s/%s: no scheduled agent carries this site's name, so no run ever builds its window",
			agentLoopSite, c.agent)
	}
	if strings.TrimSpace(f.TriggerRef) == "" {
		return runner.AgentSpec{}, fmt.Errorf(
			"%s/%s: the fixture names no trigger, and every run the scheduler starts is one named occurrence",
			agentLoopSite, c.agent)
	}
	return agent, refuseUnmintableTriggerRef(agent, f.TriggerRef)
}

// refuseUnreachableAgentStep names an expectation no reply to this window could
// satisfy. The window lists the tools it offers and the model proposes from that
// list, so an expectation naming a tool this run was never offered could only
// ever be measured as a wrong answer, for a reason that is the scenario's rather
// than the model's.
func refuseUnreachableAgentStep(want string, specs []mcp.ToolSpec) error {
	if want == agentLoopFinalStep {
		return nil
	}
	if strings.TrimSpace(want) == "" {
		return fmt.Errorf(
			"%s: the scenario names no step the turn must take, so it asserts nothing", agentLoopSite)
	}
	offered := make([]string, 0, len(specs))
	for _, spec := range specs {
		if spec.Name == want {
			return nil
		}
		offered = append(offered, spec.Name)
	}
	return fmt.Errorf(
		"%s: the scenario expects the turn to call %q, and this window offers %s (or %q to answer the goal)",
		agentLoopSite, want, strings.Join(offered, ", "), agentLoopFinalStep)
}

// agentLoopSeedContext seeds the window the way the runner service does, through
// the same helper, so the tier rule and the provenance-ref shape gate are
// applied by the window to exactly what a production run would carry.
func agentLoopSeedContext(items []agentLoopGrounding) []runner.Grounding {
	seed := make([]runner.Grounding, 0, len(items))
	for _, item := range items {
		seed = append(seed, retrievedSeed(item.SourceID, item.Content))
	}
	return seed
}

// agentLoopCase is one seeded window ready to be answered. The job carries the
// agent's allowlist, so the runner narrows the registry to it on every pass.
type agentLoopCase struct {
	job      runner.Job
	expected agentLoopStep
}

// agentLoopTurnBudget bounds a run to the single turn this site certifies.
//
// The step bound is the case's, not the agent's, because the scope is: one paid
// reply per scenario, never the two further attempts the loop makes when a reply
// will not parse — a case that let those run would certify the answer a model
// gives after being told what it got wrong rather than the answer it gives. The
// bound reaches nothing the model reads: the step count never enters a prompt,
// and the output budget it leaves defaulted is far above the per-call ceiling
// that actually sizes the request, so this turn's prompt is the one a shipped run
// sends first.
func agentLoopTurnBudget() runner.Budget { return runner.Budget{MaxSteps: 1} }

// Run drives runner.Run for one turn and records the request it issued.
//
// The loop does no I/O of its own — the window is built from the job and the
// offered specs, and every outside reach is through the two seams handed to New —
// so a run needs no database, and both seams here are the lane's: the brain
// forwards to the completer, and the tool surface stages.
//
// What the run made of the reply is not inherited: Evaluate re-runs the same
// entry point over the recorded text, so the verdict is measured through the
// shipped path rather than read off a result this file interpreted.
func (c *agentLoopCase) Run(ctx context.Context, completer aitasks.Completer) (aitasks.Trace, error) {
	recorder := &agentLoopRecorder{completer: completer}
	job := c.job
	job.Budget.MaxSteps = agentLoopTurnBudget().MaxSteps
	_, err := runner.New(agentLoopToolSurface{}, recorder).Run(ctx, job)
	trace := aitasks.Trace{Requests: recorder.requests}
	if recorder.failed != nil {
		return trace, fmt.Errorf("%s: the model call did not complete: %w", agentLoopSite, recorder.failed)
	}
	if err != nil {
		return trace, fmt.Errorf("%s: the run never reached a reply to measure: %w", agentLoopSite, err)
	}
	trace.Output = recorder.reply
	return trace, nil
}

// agentLoopRecorder is the brain the loop reasons through: it records the request
// the loop issued and the reply it read.
//
// It reports an empty Meta because it knows nothing to put there. Meta is the
// served model identity the router stamps on a trace step, and the cert lane's
// completer is bound to one (provider, model, env) that the RECORD names — so a
// step claiming an identity this seam never resolved would be the one part of the
// trace that was invented.
type agentLoopRecorder struct {
	completer aitasks.Completer
	requests  []model.Request
	reply     string
	// failed is the completer's own failure. It is kept apart from anything the
	// loop decided because a call that never completed is the lane's problem, not
	// a measurement of the reply.
	failed error
}

func (r *agentLoopRecorder) Complete(ctx context.Context, req model.Request) (model.Response, runner.Meta, error) {
	r.requests = append(r.requests, req)
	resp, err := r.completer.Complete(ctx, req)
	if err != nil {
		r.failed = err
		return model.Response{}, runner.Meta{}, err
	}
	r.reply = resp.Text
	return resp, runner.Meta{}, nil
}

// PromptWindow is the supported FLOOR, not the configured provider's window.
//
// A certification result has to mean the same thing on every installation, and
// a lane that elided its transcript at whatever the local routing happens to
// bind would measure the deployment rather than the build. The floor is also
// the strict case: a prompt that fits here fits wherever this product runs.
func (*agentLoopRecorder) PromptWindow() int { return runner.MinimumPromptWindow }

// agentLoopReplay answers with the reply the run recorded, so Evaluate reaches
// the step protocol the only way it is reachable: by running the loop.
type agentLoopReplay struct{ reply string }

func (r agentLoopReplay) Complete(context.Context, model.Request) (model.Response, runner.Meta, error) {
	return model.Response{Text: r.reply}, runner.Meta{}, nil
}

// PromptWindow matches the recorder's, so a replay elides exactly where the
// recording did — a different window here would replay a different prompt.
func (agentLoopReplay) PromptWindow() int { return runner.MinimumPromptWindow }

// agentLoopToolSurface is the tool surface a certification run is offered: it
// advertises the registered surface, as a passport admitting every scope would,
// and applies none of it. The runner narrows it to the agent's allowlist exactly
// as it narrows a production run's.
//
// Staging every proposal is the honest posture for a lane that holds no
// authority. A certification run has no seat, no passport and no workspace to act
// in, so nothing it proposes may execute — and staging is the loop's own answer
// for an action a human has not yet allowed, which suspends the run instead of
// pretending an outcome. It also keeps the graded turn to one turn: a refusal
// would be fed back as an observation and re-prompted, which is the loop, and the
// loop is not what this site's scenarios measure.
type agentLoopToolSurface struct{}

func (agentLoopToolSurface) Specs() []mcp.ToolSpec { return agentLoopRegistry() }

func (agentLoopToolSurface) Offered(context.Context) []mcp.ToolSpec { return agentLoopRegistry() }

func (agentLoopToolSurface) Invoke(context.Context, string, json.RawMessage) (json.RawMessage, error) {
	return nil, &workflow.StagedApprovalError{ApprovalID: ids.New[ids.ApprovalKind]()}
}

// Evaluate replays the recorded reply through the same loop, which runs the step
// protocol over it in the protocol's own order, and only then asks whether the
// step it took is the step the scenario expects. The order is the meaning: a
// reply the protocol refuses has no step to disagree with.
//
// The replay runs on the loop's default budget rather than the single-turn one,
// because a reply the protocol refuses is re-prompted and the loop's own message
// about WHY only reaches the result once it gives up on that text. Handing the
// same text back until it does is what makes that message reachable; it cannot
// turn a refusal into an acceptance, since a reply the protocol accepts ends the
// replay on the first pass. The replay does no I/O, so it needs nothing from the
// run's context.
func (c *agentLoopCase) Evaluate(trace aitasks.Trace) aitasks.Outcome {
	res, err := runner.New(agentLoopToolSurface{}, agentLoopReplay{reply: trace.Output}).
		Run(context.Background(), c.job)
	if err != nil {
		return aitasks.Outcome{
			Result: aitasks.OutcomeInvalid,
			Detail: agentLoopSite + ": the loop could not replay the reply: " + err.Error(),
		}
	}
	switch {
	case res.Outcome == runner.OutcomeCompleted:
		return c.gradeStep(agentLoopFinalStep, nil)
	case res.Pending != nil:
		// The only thing this lane's tool surface does with a proposal is stage
		// it, so a suspended replay is a well-formed tool call and the pending
		// call names which tool — and, for a scenario that pins them, the
		// arguments it was called with.
		return c.gradeStep(res.Pending.Tool, res.Pending.Args)
	case len(res.Steps) > 0 && res.Steps[0].Admission == runner.AdmissionRefused:
		// The first step named a tool outside this agent's allowlist, which the
		// runner refuses before the surface sees it — as it would in a run. The
		// replay then re-plans into the same text until its budget ends, so the
		// graded step is the first one, not the budget it exhausted.
		return c.gradeStep(res.Steps[0].Tool, res.Steps[0].Args)
	default:
		// The report is an operator artifact whose whole subject is what the
		// step protocol refused, so it takes the detail rather than the reason a
		// contact's panel gets.
		return aitasks.Outcome{Result: aitasks.OutcomeInvalid, Detail: res.DegradeDetail()}
	}
}

// gradeStep compares the step the turn took against the step the scenario
// expects, naming both in the protocol's own vocabulary — a tool name, or the
// word that ends a run.
//
// The step the turn took is safe to quote back into a record because the protocol
// bounds it before returning it: a tool name is a registry identifier, and one
// long enough to carry prose never becomes a step at all.
//
// The step is compared BEFORE the arguments, and a wrong step never reports an
// argument disagreement: the arguments of a call that should not have been made
// are not the thing that went wrong, and naming them would bury the step under
// the detail of a call the scenario never wanted.
func (c *agentLoopCase) gradeStep(step string, args json.RawMessage) aitasks.Outcome {
	if step != c.expected.name {
		return aitasks.Outcome{
			Result: aitasks.OutcomeWrongAnswer,
			Detail: fmt.Sprintf("the turn took the step %q where the scenario expects %q", step, c.expected.name),
		}
	}
	if disagreements := agentLoopArgDisagreements(c.expected.args, args); len(disagreements) > 0 {
		return aitasks.Outcome{
			Result: aitasks.OutcomeWrongAnswer,
			Detail: fmt.Sprintf("the turn took the step %q, but %s",
				step, strings.Join(disagreements, "; ")),
		}
	}
	return aitasks.Outcome{
		Result: aitasks.OutcomeAccepted,
		Detail: fmt.Sprintf("the turn took the step %q", step),
	}
}
