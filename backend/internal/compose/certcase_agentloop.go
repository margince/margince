// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The certification case for one agent_loop site — one scheduled agent's run on
// the Surface-B runner, from its first turn to the answer that ends it.
//
// WHAT IT CERTIFIES IS WHAT RUNS. agent_loop is the engine and each site is one
// scheduled agent. The goal and the tool allowlist are that agent's own,
// resolved by ScheduledAgentSpecByName exactly as the runner service resolves
// them, and the runner narrows the registry to the allowlist itself. A scenario
// supplies only what varies between runs: the occurrence, what retrieval seeded,
// and what each tool answers.
//
// WHAT IT EXERCISES. The loop, through runner.Run on the agent's own budget:
// every turn's window, every observation fed back, the step protocol's retries
// and the run's end. The tools are the fixture's (certcase_agentloop_tools.go),
// because the lane has no database; the write is checked as its store checks it
// and acknowledged, never performed. Both agents' writes execute without an
// approval in production, so a run never suspends and Resume is not on their
// path.
//
// WHAT IT GRADES. The run's writes against the list its goal tells it to read
// first (certcase_agentloop_work.go): every listed subject written about, none
// outside the list, and a run that ends. A scenario expecting `final` expects
// the run to end without writing. What a write SAYS is the judge's to grade,
// from the run Output records: every step, its observation, and the answer.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/compose/promptlang"
	"github.com/margince/margince/backend/internal/modules/agents/runner"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// agentLoopSite names the task in every refusal the case writes; the site's own
// name follows wherever the refusal is about one agent.
const agentLoopSite = "agent_loop"

// agentLoopFinalStep is the expectation token for the step that ends a run. It
// is the protocol's own word for it, and it is the one name a fixture may not
// give a tool — the two would be indistinguishable in an expectation.
const agentLoopFinalStep = "final"

// agentLoopFixture is ONE scheduled run, in the three things that vary between
// runs of one agent: the occurrence that started it, the seed context retrieval
// returned for it, and what each tool the run calls answers. Everything else the
// window carries is the agent's own and is looked up, never supplied.
type agentLoopFixture struct {
	TriggerRef string                `json:"trigger_ref"`
	Grounding  []agentLoopGrounding  `json:"grounding"`
	Tools      []agentLoopToolAnswer `json:"tools"`
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

// Prepare turns one scheduled run and the end the scenario expects into a
// runnable case. The expectation is the agent's write tool, which the run must
// use on its whole worklist, or `final` for a run that must end without writing.
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
			"trigger_ref, its grounding and its tools' answers, nothing else: %w", agentLoopSite, err)
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
	surface, err := c.toolSurface(f, job.Narrow(agentLoopRegistry()))
	if err != nil {
		return nil, err
	}
	var want string
	if err := json.Unmarshal(expected, &want); err != nil {
		return nil, fmt.Errorf("%s: the expected answer is not a step name: %w", agentLoopSite, err)
	}
	if want != agentLoopFinalStep && want != surface.work.write {
		return nil, fmt.Errorf("%s/%s: the scenario expects %q, and a run of this agent ends either by writing with %s or as %q",
			agentLoopSite, c.agent, want, surface.work.write, agentLoopFinalStep)
	}
	return &agentLoopCase{job: job, tools: surface, expected: want}, nil
}

// toolSurface binds the fixture's answers to this agent's worklist and write.
func (c agentLoopCases) toolSurface(f agentLoopFixture, specs []mcp.ToolSpec) (agentLoopToolSurface, error) {
	work, known := agentLoopWorks[c.agent]
	if !known {
		return agentLoopToolSurface{}, fmt.Errorf("%s/%s: no worklist names what this agent's run is for, so no write of it can be graded",
			agentLoopSite, c.agent)
	}
	offered := make([]string, 0, len(specs))
	for _, spec := range specs {
		offered = append(offered, spec.Name)
	}
	write, err := writeTool(offered, specs)
	if err != nil {
		return agentLoopToolSurface{}, err
	}
	work.write = write
	if err := refuseUnanswerableTools(f.Tools, specs); err != nil {
		return agentLoopToolSurface{}, err
	}
	raw, err := listAnswer(f.Tools, work.list)
	if err != nil {
		return agentLoopToolSurface{}, err
	}
	list, err := work.read(raw)
	if err != nil {
		return agentLoopToolSurface{}, fmt.Errorf("%s/%s: %w", agentLoopSite, c.agent, err)
	}
	return agentLoopToolSurface{work: work, list: list, answers: f.Tools}, nil
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
			agentLoopSite, c.agent,
		)
	}
	if strings.TrimSpace(f.TriggerRef) == "" {
		return runner.AgentSpec{}, fmt.Errorf(
			"%s/%s: the fixture names no trigger, and every run the scheduler starts is one named occurrence",
			agentLoopSite, c.agent,
		)
	}
	return agent, refuseUnmintableTriggerRef(agent, f.TriggerRef)
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

// agentLoopCase is one scheduled run ready to be answered. The job carries the
// agent's allowlist, so the runner narrows the registry to it on every pass.
type agentLoopCase struct {
	job      runner.Job
	tools    agentLoopToolSurface
	expected string
}

// agentLoopRun is a run as Output carries it: what the judge grades, and the
// replies Evaluate replays to grade it again.
type agentLoopRun struct {
	Steps   []agentLoopRunStep `json:"steps"`
	Final   json.RawMessage    `json:"final,omitempty"`
	Replies []string           `json:"replies"`
}

// agentLoopRunStep is one call the run made and what it was told.
type agentLoopRunStep struct {
	Tool        string          `json:"tool"`
	Args        json.RawMessage `json:"args,omitempty"`
	Admission   string          `json:"admission"`
	Observation string          `json:"observation"`
}

// Run drives runner.Run on the agent's own budget and records every request it
// issued and every reply it read.
//
// The loop does no I/O of its own — the window is built from the job and the
// offered specs, and every outside reach is through the two seams handed to New —
// so a run needs no database: the brain forwards to the completer, and the tool
// surface answers from the fixture.
func (c *agentLoopCase) Run(ctx context.Context, completer aitasks.Completer) (aitasks.Trace, error) {
	recorder := &agentLoopRecorder{completer: completer}
	res, err := runner.New(c.tools, recorder).Run(ctx, c.job)
	trace := aitasks.Trace{Requests: recorder.requests}
	if recorder.failed != nil {
		return trace, fmt.Errorf("%s: the model call did not complete: %w", agentLoopSite, recorder.failed)
	}
	if err != nil {
		return trace, fmt.Errorf("%s: the run never reached a reply to measure: %w", agentLoopSite, err)
	}
	run := agentLoopRun{Final: res.Final, Replies: recorder.replies}
	for _, step := range res.Steps {
		run.Steps = append(run.Steps, agentLoopRunStep{
			Tool: step.Tool, Args: step.Args, Admission: step.Admission, Observation: step.Observation,
		})
	}
	output, err := json.Marshal(run)
	if err != nil {
		return trace, fmt.Errorf("%s: recording the run: %w", agentLoopSite, err)
	}
	trace.Output = string(output)
	return trace, nil
}

// agentLoopRecorder is the brain the loop reasons through: it prepares each
// request with the company context, as production's brain does, and records
// each request the loop issued and each reply it read.
//
// It reports an empty Meta because it knows nothing to put there. Meta is the
// served model identity the router stamps on a trace step, and the cert lane's
// completer is bound to one (provider, model, env) that the RECORD names — so a
// step claiming an identity this seam never resolved would be the one part of the
// trace that was invented.
type agentLoopRecorder struct {
	completer aitasks.Completer
	requests  []model.Request
	replies   []string
	// failed is the completer's own failure. It is kept apart from anything the
	// loop decided because a call that never completed is the lane's problem, not
	// a measurement of the reply.
	failed error
}

func (r *agentLoopRecorder) Complete(ctx context.Context, req model.Request) (model.Response, runner.Meta, error) {
	req, err := withCertCompanyContext(ctx, ai.TaskAgentLoop, req)
	if err != nil {
		r.failed = err
		return model.Response{}, runner.Meta{}, err
	}
	r.requests = append(r.requests, req)
	resp, err := r.completer.Complete(ctx, req)
	if err != nil {
		r.failed = err
		return model.Response{}, runner.Meta{}, err
	}
	r.replies = append(r.replies, resp.Text)
	return resp, runner.Meta{}, nil
}

// PromptWindow is the supported FLOOR, not the configured provider's window.
//
// A certification result has to mean the same thing on every installation, and
// a lane that elided its transcript at whatever the local routing happens to
// bind would measure the deployment rather than the build. The floor is also
// the strict case: a prompt that fits here fits wherever this product runs.
func (*agentLoopRecorder) PromptWindow() int { return runner.MinimumPromptWindow }

// agentLoopReplay answers with the replies the run recorded, in order, so
// Evaluate reaches the step protocol the only way it is reachable: by running
// the loop. A replay asked for more than was recorded fails the call, which the
// loop reports as a degraded run.
type agentLoopReplay struct {
	replies []string
	next    *int
}

func (r agentLoopReplay) Complete(context.Context, model.Request) (model.Response, runner.Meta, error) {
	if *r.next >= len(r.replies) {
		return model.Response{}, runner.Meta{}, errAgentLoopReplayExhausted
	}
	reply := r.replies[*r.next]
	*r.next++
	return model.Response{Text: reply}, runner.Meta{}, nil
}

// errAgentLoopReplayExhausted is a replay asked for a turn the run never took.
var errAgentLoopReplayExhausted = errors.New("the replay asked for a turn the recorded run never took")

// PromptWindow matches the recorder's, so a replay elides exactly where the
// recording did — a different window here would replay a different prompt.
func (agentLoopReplay) PromptWindow() int { return runner.MinimumPromptWindow }

// Evaluate replays the recorded replies through the same loop and the same tool
// answers, then grades what that run wrote. The order is the meaning: a run the
// loop did not bring to its end has no work to grade.
func (c *agentLoopCase) Evaluate(trace aitasks.Trace) aitasks.Outcome {
	var recorded agentLoopRun
	if err := json.Unmarshal([]byte(trace.Output), &recorded); err != nil {
		return aitasks.Outcome{Result: aitasks.OutcomeInvalid, Detail: agentLoopSite + ": the run was not recorded: " + err.Error()}
	}
	next := 0
	res, err := runner.New(c.tools, agentLoopReplay{replies: recorded.Replies, next: &next}).
		Run(context.Background(), c.job)
	if err != nil {
		return aitasks.Outcome{
			Result: aitasks.OutcomeInvalid,
			Detail: agentLoopSite + ": the loop could not replay the run: " + err.Error(),
		}
	}
	if res.Outcome != runner.OutcomeCompleted {
		// The report is an operator artifact whose whole subject is why the run
		// did not end, so it takes the detail rather than the reason a contact's
		// panel gets.
		return aitasks.Outcome{Result: aitasks.OutcomeInvalid, Detail: "the run did not end: " + res.DegradeDetail()}
	}
	return c.gradeRun(res)
}

// gradeRun compares what a completed run wrote with what the scenario expects.
func (c *agentLoopCase) gradeRun(res runner.Result) aitasks.Outcome {
	work := c.tools.work
	var writes []json.RawMessage
	for _, step := range res.Steps {
		if step.Tool == work.write && step.Admission == agentLoopExecuted {
			writes = append(writes, step.Args)
		}
	}
	if c.expected == agentLoopFinalStep {
		if len(writes) > 0 {
			return aitasks.Outcome{
				Result: aitasks.OutcomeWrongAnswer,
				Detail: fmt.Sprintf("the run wrote %d time(s) with %s where it had nothing to do", len(writes), work.write),
			}
		}
		return aitasks.Outcome{Result: aitasks.OutcomeAccepted, Detail: "the run ended without writing"}
	}
	if len(writes) == 0 {
		return aitasks.Outcome{
			Result: aitasks.OutcomeWrongAnswer,
			Detail: fmt.Sprintf("the run ended without writing with %s", work.write),
		}
	}
	if problems := work.gradeWrites(writes, c.tools.list); len(problems) > 0 {
		return aitasks.Outcome{Result: aitasks.OutcomeWrongAnswer, Detail: strings.Join(problems, "; ")}
	}
	return aitasks.Outcome{
		Result: aitasks.OutcomeAccepted,
		Detail: fmt.Sprintf("the run wrote with %s on every item of its %s list and nothing else", work.write, work.list),
	}
}

// agentLoopExecuted is the admission a step the tool surface answered carries;
// a refused write changed nothing and is not graded as written.
const agentLoopExecuted = "executed"
