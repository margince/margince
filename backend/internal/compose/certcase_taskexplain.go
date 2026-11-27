// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The certification case for summarize/task_explanation. It sends the request
// taskexplain.ExplainRequest builds and reads the reply with
// taskexplain.ParseExplanation, the two the service calls.
//
// It measures what the grounding filter cannot. The explanation must read the
// message the task turns on and name what only that message says. It must end
// on one next step, never a sentence any task could carry.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/compose/claims"
	"github.com/margince/margince/backend/internal/compose/taskexplain"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

type taskExplainCases struct{}

func (taskExplainCases) Site() aitasks.Site {
	return aitasks.Site{Task: ai.TaskSummarize, Variant: "task_explanation", Kind: ai.SiteKindOneShot}
}

// taskExplainFixture is one task on one relationship. SourceLabel names the
// relationship message the task was filed from.
type taskExplainFixture struct {
	Task         taskExplainTask     `json:"task"`
	SourceLabel  string              `json:"source_label"`
	SourceText   string              `json:"source_text"`
	Relationship contactBriefFixture `json:"relationship"`
}

type taskExplainTask struct {
	Type   string `json:"type"`
	Title  string `json:"title"`
	Text   string `json:"text"`
	Origin string `json:"origin"`
}

type taskExplainExpectation struct {
	CitesLabel string   `json:"cites_label"`
	NamesToken string   `json:"names_token"`
	Avoids     []string `json:"avoids"`
}

// Prepare builds the input production assembles, with minted ids.
//
//nolint:ireturn // PreparedCase IS the seam: one implementation per site behind the one interface the cert lane runs.
func (taskExplainCases) Prepare(fixture, expected json.RawMessage) (aitasks.PreparedCase, error) {
	var f taskExplainFixture
	if err := json.Unmarshal(fixture, &f); err != nil {
		return nil, fmt.Errorf("summarize/task_explanation: the fixture is not the shape this site takes: %w", err)
	}
	var want taskExplainExpectation
	if err := json.Unmarshal(expected, &want); err != nil {
		return nil, fmt.Errorf("summarize/task_explanation: the expected answer is not this site's shape: %w", err)
	}
	if want.CitesLabel == "" || want.NamesToken == "" {
		return nil, errors.New("summarize/task_explanation: a scenario must name the message to cite and a token only it says")
	}
	relationship, byLabel, err := contactBriefInput(f.Relationship)
	if err != nil {
		return nil, fmt.Errorf("summarize/task_explanation: %w", err)
	}
	in := taskexplain.Input{
		Task: taskexplain.TaskIn{
			ID: ids.NewV7().String(), Type: f.Task.Type, Title: f.Task.Title, Text: f.Task.Text,
			Origin: f.Task.Origin, Created: "2026-09-03",
		},
		Relationship: &relationship,
	}
	if sourceID, ok := byLabel[f.SourceLabel]; ok {
		for i := range relationship.Recent {
			if relationship.Recent[i].ID == sourceID {
				source := relationship.Recent[i]
				in.Source, in.SourceText = &source, f.SourceText
			}
		}
	}
	return &taskExplainCase{
		in: in, mustCite: byLabel[want.CitesLabel], citeLabel: want.CitesLabel,
		mustName: want.NamesToken, mustAvoid: want.Avoids,
	}, nil
}

type taskExplainCase struct {
	in        taskexplain.Input
	mustCite  string
	citeLabel string
	mustName  string
	mustAvoid []string
}

// Run sends the production request, in English so scores compare across installations.
func (c *taskExplainCase) Run(ctx context.Context, completer aitasks.Completer) (aitasks.Trace, error) {
	req := taskexplain.ExplainRequest(c.in, string(textlang.English))
	trace := aitasks.Trace{Requests: []model.Request{req}}
	resp, err := completer.Complete(ctx, req)
	if err != nil {
		return trace, fmt.Errorf("summarize/task_explanation: %w", err)
	}
	trace.Output = resp.Text
	return trace, nil
}

// Evaluate runs the production filter, then asks what production cannot.
func (c *taskExplainCase) Evaluate(trace aitasks.Trace) aitasks.Outcome {
	sentences, err := taskexplain.ParseExplanation(trace.Output, c.in)
	if err != nil {
		return aitasks.Outcome{Result: aitasks.OutcomeInvalid, Detail: fmt.Sprintf("the grounding filter refused the reply: %v", err)}
	}
	if len(sentences) == 0 {
		return aitasks.Outcome{Result: aitasks.OutcomeAbstained, Detail: "no sentence cited a record of this task"}
	}
	var wrong []string
	if !cites(sentences, c.mustCite) {
		wrong = append(wrong, fmt.Sprintf("the explanation never cites %q, the message the task turns on", c.citeLabel))
	}
	folded := strings.ToLower(claims.Prose(sentences))
	if !strings.Contains(folded, strings.ToLower(c.mustName)) {
		wrong = append(wrong, fmt.Sprintf("the explanation never names %q, so it would read the same about any task", c.mustName))
	}
	if !hasNature(sentences, string(crmcontracts.CompanyBriefSentenceNatureRecommendation)) {
		wrong = append(wrong, "the explanation names no next step")
	}
	leaks := false
	for _, avoided := range c.mustAvoid {
		if strings.Contains(folded, strings.ToLower(avoided)) {
			wrong = append(wrong, fmt.Sprintf("the explanation says %q, which this reader was not shown", avoided))
			leaks = true
		}
	}
	if len(wrong) > 0 {
		return aitasks.Outcome{Result: aitasks.OutcomeWrongAnswer, Detail: strings.Join(wrong, "; "), Leak: leaks}
	}
	return aitasks.Outcome{Result: aitasks.OutcomeAccepted}
}

func hasNature(sentences []claims.Sentence, nature string) bool {
	for _, sentence := range sentences {
		if sentence.Nature == nature {
			return true
		}
	}
	return false
}
