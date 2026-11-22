// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The certification case for extension_task/run, the one site every extension
// AI call rides. A declared task is data, so the fixture carries it. The case
// builds the request with extensionAIRequest, as a live call does.
//
// A scenario names the words a right answer holds. No one rule says two free
// texts, or two of a unit's JSON answers, mean the same.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/ports/model"
	"github.com/margince/margince/backend/pkg/extension"
)

const extensionTaskSite = "extension_task/run"

// extensionTaskFixture is one call of a declared task: the declaration's own
// prompt and schema, and the text it is given.
type extensionTaskFixture struct {
	Prompt    string `json:"prompt"`
	Schema    string `json:"schema,omitempty"`
	InputText string `json:"input_text"`
}

// extensionTaskExpectation names the words a right answer holds.
type extensionTaskExpectation struct {
	Contains []string `json:"contains"`
}

type extensionTaskCases struct{}

func (extensionTaskCases) Site() aitasks.Site {
	return aitasks.Site{Task: ai.TaskExtensionTask, Variant: "run", Kind: ai.SiteKindOneShot}
}

//nolint:ireturn // PreparedCase IS the seam: one implementation per site behind the one interface the cert lane runs.
func (extensionTaskCases) Prepare(fixture, expected json.RawMessage) (aitasks.PreparedCase, error) {
	var f extensionTaskFixture
	if err := json.Unmarshal(fixture, &f); err != nil {
		return nil, fmt.Errorf("%s: the fixture is not the shape this site takes: %w", extensionTaskSite, err)
	}
	var want extensionTaskExpectation
	if err := json.Unmarshal(expected, &want); err != nil {
		return nil, fmt.Errorf("%s: the expected answer is not {contains: [...]}: %w", extensionTaskSite, err)
	}
	if len(want.Contains) == 0 {
		return nil, fmt.Errorf("%s: the scenario names no words the answer must hold, so no answer could fail it", extensionTaskSite)
	}
	task := extension.AITask{Name: "certified", Prompt: f.Prompt, Schema: f.Schema}
	if err := task.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", extensionTaskSite, err)
	}
	req, err := extensionAIRequest(context.Background(), task, extension.AIInput{
		Text: f.InputText, NoContact: "a certification fixture",
	})
	if err != nil {
		return nil, fmt.Errorf("%s: %w", extensionTaskSite, err)
	}
	return &extensionTaskCase{task: task, req: req, want: want}, nil
}

type extensionTaskCase struct {
	task extension.AITask
	req  model.Request
	want extensionTaskExpectation
}

func (c *extensionTaskCase) Run(ctx context.Context, completer aitasks.Completer) (aitasks.Trace, error) {
	trace := aitasks.Trace{Requests: []model.Request{c.req}}
	resp, err := completer.Complete(ctx, c.req)
	if err != nil {
		return trace, fmt.Errorf("%s: %w", extensionTaskSite, err)
	}
	trace.Output = resp.Text
	return trace, nil
}

func (c *extensionTaskCase) Evaluate(trace aitasks.Trace) aitasks.Outcome {
	if err := extensionAnswerCheck(c.task)(trace.Output); err != nil {
		return aitasks.Outcome{Result: aitasks.OutcomeInvalid, Detail: err.Error()}
	}
	for _, word := range c.want.Contains {
		if !strings.Contains(strings.ToLower(trace.Output), strings.ToLower(word)) {
			return aitasks.Outcome{
				Result: aitasks.OutcomeWrongAnswer,
				Detail: fmt.Sprintf("the answer does not hold %q", word),
			}
		}
	}
	return aitasks.Outcome{Result: aitasks.OutcomeAccepted}
}
