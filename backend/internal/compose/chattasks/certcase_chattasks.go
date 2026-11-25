// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chattasks

// The chat_tasks site's certification case. A scenario names the chat, the
// open tasks and today, and the action each task must get. Run and Evaluate
// call the production Request, Parse and Decide.

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

const certSite = "chat_task_reading/chat_tasks"

type certTask struct {
	Subject string `json:"subject"`
	DueOn   string `json:"due_on,omitempty"`
}

type certFixture struct {
	Today string     `json:"today"`
	Chat  string     `json:"chat"`
	Tasks []certTask `json:"tasks"`
}

// certExpectation is the action each task must get, in fixture order, and
// for a new date the day it must name.
type certExpectation struct {
	Actions []string `json:"actions"`
	DueOn   []string `json:"due_on,omitempty"`
}

// CertCases serves the chat_task_reading/chat_tasks site to the certification lane.
type CertCases struct{}

// Site names the chat_task_reading/chat_tasks site this case certifies.
func (CertCases) Site() aitasks.Site {
	return aitasks.Site{Task: ai.TaskChatTaskReading, Variant: "chat_tasks", Kind: ai.SiteKindOneShot}
}

// Prepare refuses a scenario no answer could pass: no tasks, or an action
// list that does not name one action per task.
//
//nolint:ireturn // PreparedCase IS the seam: one implementation per site behind the one interface the cert lane runs.
func (CertCases) Prepare(fixture, expected json.RawMessage) (aitasks.PreparedCase, error) {
	var f certFixture
	if err := json.Unmarshal(fixture, &f); err != nil {
		return nil, fmt.Errorf("%s: the fixture is not the shape this site takes: %w", certSite, err)
	}
	today, err := time.Parse(time.DateOnly, f.Today)
	if err != nil || len(f.Tasks) == 0 {
		return nil, fmt.Errorf("%s: the fixture needs a today date and at least one task", certSite)
	}
	var want certExpectation
	if err := json.Unmarshal(expected, &want); err != nil || len(want.Actions) != len(f.Tasks) {
		return nil, fmt.Errorf("%s: the expected value names one action per task: %v", certSite, err)
	}
	in := Input{Chat: f.Chat, Today: today}
	for _, t := range f.Tasks {
		task := Task{ID: ids.NewV7(), Subject: t.Subject}
		if due, err := time.Parse(time.DateOnly, t.DueOn); err == nil {
			task.DueAt = &due
		}
		in.Tasks = append(in.Tasks, task)
	}
	return &certCase{in: in, expected: want}, nil
}

type certCase struct {
	in       Input
	expected certExpectation
}

func (c *certCase) Run(ctx context.Context, completer aitasks.Completer) (aitasks.Trace, error) {
	req := Request(c.in, model.AboutNoContact("certification corpus"))
	trace := aitasks.Trace{Requests: []model.Request{req}}
	res, err := completer.Complete(ctx, req)
	if err != nil {
		return trace, fmt.Errorf("%s: %w", certSite, err)
	}
	trace.Output = res.Text
	return trace, nil
}

// Evaluate reads the reply through the production parser and Decide, then
// asks whether each task got the action, and day, the scenario names.
func (c *certCase) Evaluate(trace aitasks.Trace) aitasks.Outcome {
	answers, err := Parse(trace.Output)
	if err != nil {
		return aitasks.Outcome{Result: aitasks.OutcomeInvalid, Detail: err.Error()}
	}
	decided := map[ids.UUID]Decision{}
	for _, d := range Decide(c.in, answers) {
		decided[d.Task] = d
	}
	for i, task := range c.in.Tasks {
		got := ActionNone
		d, ok := decided[task.ID]
		switch {
		case ok && d.Done:
			got = ActionDone
		case ok:
			got = ActionNewDate
		}
		if got != c.expected.Actions[i] {
			return aitasks.Outcome{Result: aitasks.OutcomeWrongAnswer, Detail: fmt.Sprintf("task %d got %s, want %s", i+1, got, c.expected.Actions[i])}
		}
		if got == ActionNewDate && i < len(c.expected.DueOn) && d.DueOn.Format(time.DateOnly) != c.expected.DueOn[i] {
			return aitasks.Outcome{Result: aitasks.OutcomeWrongAnswer, Detail: fmt.Sprintf("task %d moved to %s, want %s", i+1, d.DueOn.Format(time.DateOnly), c.expected.DueOn[i])}
		}
	}
	return aitasks.Outcome{Result: aitasks.OutcomeAccepted}
}
