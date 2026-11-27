// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/agents/runner"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

const (
	agentLoopRecoveryBand  = "refusal_recovery"
	agentLoopRecoverySteps = 5
)

// agentLoopToolRefusal is one real refusal type a fixture's backend returns.
// Args on the answer select the refused call; a repaired call reaches its
// ordinary answer. No counter or mutable sequence changes between replays.
type agentLoopToolRefusal struct {
	Kind  string `json:"kind"`
	Field string `json:"field,omitempty"`
	Want  string `json:"want,omitempty"`
	Got   string `json:"got,omitempty"`
}

func (s agentLoopToolSurface) refusalFor(tool string, args json.RawMessage) *agentLoopToolRefusal {
	for _, answer := range s.answers {
		if answer.Tool == tool && answer.Refusal != nil && len(agentLoopArgDisagreements(answer.Args, args)) == 0 {
			return answer.Refusal
		}
	}
	return nil
}

func (s agentLoopToolSurface) refusalError(tool string, refusal agentLoopToolRefusal) error {
	var fields []string
	for _, spec := range agentLoopRegistry() {
		if spec.Name == tool {
			for key := range agentLoopSchemaProperties(spec.InputSchema) {
				fields = append(fields, key)
			}
		}
	}
	slices.Sort(fields)
	guidance := "use the declared arguments: " + strings.Join(fields, ", ")
	switch refusal.Kind {
	case "bad_arguments":
		return &agents.BadArgsError{Cause: errors.New("the argument value was not accepted"), Field: refusal.Field, Code: "invalid", Guidance: guidance}
	case "unknown_field":
		return &agents.BadArgsError{Cause: &datasource.UnknownFieldError{Fields: []string{refusal.Field}, Hint: guidance}}
	case "field_shape":
		return &agents.BadArgsError{Cause: &datasource.FieldShapeError{Field: refusal.Field, Want: refusal.Want, Got: refusal.Got}, Field: refusal.Field, Code: "invalid"}
	case "unsupported":
		return fmt.Errorf("record reader: %w", apperrors.ErrUnsupportedBySoR)
	default:
		return fmt.Errorf("unknown recovery refusal %q", refusal.Kind)
	}
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
		switch answer.Refusal.Kind {
		case "bad_arguments", "unknown_field", "field_shape":
			if _, pinned := answer.Args[answer.Refusal.Field]; !pinned {
				return errors.New("agent_loop: a repairable refusal must pin its named field")
			}
			if answer.Refusal.Kind == "field_shape" && (answer.Refusal.Want == "" || answer.Refusal.Got == "") {
				return errors.New("agent_loop: a field-shape refusal needs the expected and received shapes")
			}
			if strings.TrimSpace(answer.Refusal.Field) == "" || len(answer.Args) == 0 {
				return errors.New("agent_loop: a repairable refusal must pin the refused argument and name its field")
			}
		case "unsupported":
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

// agentLoopRecoveryMeasure keeps first-step accuracy apart from the final
// outcome. A run that avoided the bad call reports zero observed refusals;
// it is a completion measurement, not evidence that a refusal was recovered.
type agentLoopRecoveryMeasure = aitasks.LoopRecoveryMeasure

func (c *agentLoopCase) recoveryMeasure(res runner.Result, replies []string) *agentLoopRecoveryMeasure {
	if c.band != agentLoopRecoveryBand {
		return nil
	}
	out := &agentLoopRecoveryMeasure{}
	firstTool, validFirst := "", false
	if len(replies) > 0 {
		firstTool, validFirst = runner.ToolStepName(replies[0])
	}
	terminal := map[string]bool{}
	repairable := map[string]bool{}
	for i, step := range res.Steps {
		if i == 0 {
			out.FirstStepCorrect = validFirst && firstTool == c.tools.work.list && step.Tool == firstTool && step.Admission == agentLoopExecuted
		}
		if terminal[step.Tool] {
			out.TerminalRepeated = true
		}
		if step.Admission == runner.AdmissionRefused {
			out.Refusals++
			refusal := c.tools.refusalFor(step.Tool, step.Args)
			if refusal != nil && refusal.Kind == "unsupported" {
				terminal[step.Tool] = true
			} else {
				repairable[step.Tool] = true
			}
		} else if step.Admission == agentLoopExecuted && repairable[step.Tool] {
			out.Recovered = true
		}
	}
	out.Completed = res.Outcome == runner.OutcomeCompleted && c.gradeRun(res).Result == aitasks.OutcomeAccepted && !out.TerminalRepeated
	out.Recovered = out.Recovered && out.Completed
	return out
}

func (c *agentLoopCase) withRecoveryMeasure(outcome aitasks.Outcome, res runner.Result, replies []string) aitasks.Outcome {
	measure := c.recoveryMeasure(res, replies)
	if measure == nil {
		return outcome
	}
	outcome.Recovery = measure
	if measure.TerminalRepeated {
		outcome.Result = aitasks.OutcomeWrongAnswer
		outcome.Detail = "a terminally unsupported tool was called again"
	}
	encoded, err := json.Marshal(measure)
	if err != nil {
		return aitasks.Outcome{Result: aitasks.OutcomeInvalid, Detail: err.Error()}
	}
	outcome.Detail += "; recovery=" + string(encoded)
	return outcome
}

// RecoveryMeasure keeps failed or withheld recovery runs in the denominator.
// Replaying a captured loop is local; it never asks a model for another reply.
func (c *agentLoopCase) RecoveryMeasure(trace aitasks.Trace) *aitasks.LoopRecoveryMeasure {
	if c.band != agentLoopRecoveryBand {
		return nil
	}
	if measure := c.Evaluate(trace).Recovery; measure != nil {
		return measure
	}
	return &aitasks.LoopRecoveryMeasure{}
}
