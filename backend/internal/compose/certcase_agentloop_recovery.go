// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/agents/runner"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

const (
	agentLoopRecoveryBand  = aitasks.RefusalRecoveryBand
	agentLoopRecoverySteps = 6
)

// agentLoopToolRefusal selects a seeded real refusal before the first candidate
// turn. Argument refusals come from the production decoder; unsupported is a
// declared backend capability gap. Replays execute the same seeded read.
type agentLoopToolRefusal struct {
	Kind string `json:"kind"`
}

func (s agentLoopToolSurface) refusalFor(tool string, args json.RawMessage) *agentLoopToolRefusal {
	for _, answer := range s.answers {
		if answer.Tool == tool && answer.Refusal != nil && len(agentLoopArgDisagreements(answer.Args, args)) == 0 {
			return answer.Refusal
		}
	}
	return nil
}

// recoveryRefusalKind classifies the real decoder's refusal, not a guessed
// fixture sentence. A successful or missing-record read is no argument refusal.
func (s agentLoopToolSurface) recoveryRefusalKind(tool string, args json.RawMessage) string {
	if tool != "read_record" {
		return ""
	}
	_, err := agents.DecodeReadRecordArguments(args)
	if err != nil {
		if _, ok := errors.AsType[*datasource.UnknownFieldError](err); ok {
			return "unknown_field"
		}
		if _, ok := errors.AsType[*json.UnmarshalTypeError](err); ok {
			return "field_shape"
		}
		if shape, ok := errors.AsType[*datasource.FieldShapeError](err); ok && shape.Got != "" {
			return "field_shape"
		}
		return "bad_arguments"
	}
	if refusal := s.refusalFor(tool, args); refusal != nil && refusal.Kind == "unsupported" {
		return "unsupported"
	}
	return ""
}

func prepareAgentLoopRecovery(f agentLoopFixture, job *runner.Job) error {
	var refusals int
	for _, answer := range f.Tools {
		if answer.Refusal == nil {
			continue
		}
		refusals++
		if len(answer.Result) > 0 {
			return errors.New("agent_loop: a tool answer cannot be both a result and a refusal")
		}
		if answer.Tool != "read_record" || len(answer.Args) != 0 {
			return errors.New("agent_loop: recovery names read_record's refusal class, not a pinned fake response")
		}
		switch answer.Refusal.Kind {
		case "bad_arguments", "unknown_field", "field_shape", "unsupported":
		default:
			return fmt.Errorf("agent_loop: unknown refusal kind %q", answer.Refusal.Kind)
		}

	}
	if f.Band == "" && refusals == 0 {
		return nil
	}
	if f.Band != agentLoopRecoveryBand {
		return fmt.Errorf("agent_loop: refusal fixtures need band %q", agentLoopRecoveryBand)
	}
	if refusals != 1 {
		return errors.New("agent_loop: a recovery scenario needs exactly one typed refusal")
	}
	if job.Budget.MaxSteps <= 0 {
		job.Budget.MaxSteps = agentLoopRecoverySteps
	} else {
		job.Budget.MaxSteps = min(job.Budget.MaxSteps, agentLoopRecoverySteps)
	}
	return nil
}

// recoverySeed runs one fixture-owned failed read through the real decoder.
// It is not a candidate model turn; the candidate is measured on what it does next.
func (s agentLoopToolSurface) recoverySeed() (string, error) {
	if s.recoveryKind == "" {
		return "", nil
	}
	var brief agents.ReadBriefResult
	if err := json.Unmarshal(s.list.normalized, &brief); err != nil || len(brief.Items) == 0 || len(brief.Items[0].EvidenceIDs) == 0 {
		return "", errors.New("agent_loop: a recovery scenario needs a brief with cited evidence")
	}
	args := map[string]any{"record_type": "deal", "id": brief.Items[0].EvidenceIDs[0].String()}
	switch s.recoveryKind {
	case "bad_arguments":
		args["id"] = "not-a-record-id"
	case "unknown_field":
		args["obsolete_field"] = true
	case "field_shape":
		args["id"] = []string{}
	}
	raw, err := json.Marshal(args)
	if err != nil {
		return "", err
	}
	if kind := s.recoveryRefusalKind("read_record", raw); kind != s.recoveryKind {
		return "", fmt.Errorf("agent_loop: the real decoder returned %q, not the seeded %q refusal", kind, s.recoveryKind)
	}
	step, err := json.Marshal(struct {
		Tool string          `json:"tool"`
		Args json.RawMessage `json:"args"`
	}{Tool: "read_record", Args: raw})
	return string(step), err
}

// agentLoopRecoveryMeasure keeps the first candidate turn after the seeded
// refusal apart from mechanical completion of the whole goal.
type agentLoopRecoveryMeasure = aitasks.LoopRecoveryMeasure

func (c *agentLoopCase) recoveryMeasure(res runner.Result, replies []string) *agentLoopRecoveryMeasure {
	if c.band != agentLoopRecoveryBand {
		return nil
	}
	out := &agentLoopRecoveryMeasure{}
	firstTool, validFirst := "", false
	if len(replies) > 1 {
		firstTool, validFirst = runner.ToolStepName(replies[1])
	}
	terminal := map[string]bool{}
	repairable := map[string]bool{}
	for i, step := range res.Steps {
		if i == 1 {
			out.FirstStepCorrect = validFirst && firstTool == c.tools.work.list && step.Tool == firstTool && step.Admission == agentLoopExecuted
		}
		if terminal[step.Tool] {
			out.TerminalRepeated = true
		}
		if step.TerminalRefusal {
			terminal[step.Tool] = true
		}
		if step.Admission == runner.AdmissionRefused {
			kind := c.tools.recoveryRefusalKind(step.Tool, step.Args)
			if kind == c.tools.recoveryKind {
				out.Refusals++
				if !step.TerminalRefusal {
					repairable[step.Tool] = true
				}
			}

		} else if step.Admission == agentLoopExecuted && repairable[step.Tool] {
			out.Recovered = true
		}
	}
	if len(replies) > 1 && c.expected == agentLoopFinalStep && runner.IsFinalStep(replies[1]) {
		out.FirstStepCorrect = true
	}
	out.Completed = res.Outcome == runner.OutcomeCompleted && c.gradeRun(res).Result == aitasks.OutcomeAccepted && !out.TerminalRepeated
	out.Recovered = out.Recovered && out.Completed
	return out
}

func (c *agentLoopCase) withRecoveryMeasure(outcome aitasks.Outcome, res runner.Result, replies []string) (aitasks.Outcome, *aitasks.LoopRecoveryMeasure) {
	measure := c.recoveryMeasure(res, replies)
	if measure == nil {
		return outcome, nil
	}
	if measure.TerminalRepeated && outcome.Result == aitasks.OutcomeAccepted {
		outcome.Result = aitasks.OutcomeWrongAnswer
		outcome.Detail = "a terminally unsupported tool was called again"
	}
	encoded, err := json.Marshal(measure)
	if err != nil {
		return aitasks.Outcome{Result: aitasks.OutcomeInvalid, Detail: err.Error()}, measure
	}
	outcome.Detail += "; recovery=" + string(encoded)
	return outcome, measure
}

// RecoveryMeasure keeps failed or withheld recovery runs in the denominator.
// Replaying a captured loop is local; it never asks a model for another reply.
func (c *agentLoopCase) RecoveryMeasure(trace aitasks.Trace) *aitasks.LoopRecoveryMeasure {
	if c.band != agentLoopRecoveryBand {
		return nil
	}
	if _, measure := c.EvaluateRecovery(trace); measure != nil {
		return measure
	}
	return &aitasks.LoopRecoveryMeasure{}
}
