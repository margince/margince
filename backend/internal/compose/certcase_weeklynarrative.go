// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The weekly narrative's certification site.
//
// The request is built by the narrative package's own writer and the reply is
// read by its own parser: a case that rebuilt either would measure a copy of
// the prompt rather than the prompt.
//
// What a case asserts is what the SENTENCE may claim. The lane's whole safety
// argument is that it adds nothing — every fact it may state is already in the
// deterministic review — so the thing worth certifying is that it does not
// reach past the summary it was given.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/compose/weekly/narrative"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// weeklyNarrativeFixture is one week, as a case states it.
type weeklyNarrativeFixture struct {
	WeekStart string           `json:"week_start"`
	Counts    narrative.Counts `json:"counts"`
	Deals     []narrative.Deal `json:"deals"`
}

// weeklyNarrativeExpectation is what the sentence must and must not say.
type weeklyNarrativeExpectation struct {
	// MustMention are substrings the sentence has to carry — a deal that was
	// won, a number that is the week's headline. Matched case-insensitively,
	// because the sentence is prose and its capitalisation is the model's.
	MustMention []string `json:"must_mention"`
	// MustNotMention are the words that would mean the model reached past the
	// summary: a company nobody gave it, a judgement nobody asked for.
	MustNotMention []string `json:"must_not_mention"`
}

// weeklyNarrativeCases serves the one site that writes a week's sentence.
type weeklyNarrativeCases struct{}

func (weeklyNarrativeCases) Site() aitasks.Site {
	return aitasks.Site{
		Task:    ai.TaskWeeklyReview,
		Variant: "narrative",
		Kind:    ai.SiteKindOneShot,
	}
}

// Prepare turns one week and its expectation into a runnable case.
//
//nolint:ireturn // PreparedCase IS the seam: one implementation per site behind the one interface the cert lane runs.
func (weeklyNarrativeCases) Prepare(fixture, expected json.RawMessage) (aitasks.PreparedCase, error) {
	var f weeklyNarrativeFixture
	if err := json.Unmarshal(fixture, &f); err != nil {
		return nil, fmt.Errorf("weekly_review/narrative: the fixture is not the shape this site takes: %w", err)
	}
	if f.WeekStart == "" {
		return nil, fmt.Errorf("weekly_review/narrative: the fixture names no week")
	}
	var want weeklyNarrativeExpectation
	if err := json.Unmarshal(expected, &want); err != nil {
		return nil, fmt.Errorf(
			"weekly_review/narrative: the expected answer is not a must/must-not pair: %w", err)
	}
	if len(want.MustMention) == 0 && len(want.MustNotMention) == 0 {
		// A case that asserts nothing scores every reply as correct, which is
		// worse than no case: it reports a certified site nobody measured.
		return nil, fmt.Errorf("weekly_review/narrative: the expectation asserts nothing")
	}
	return &weeklyNarrativeCase{
		in: narrative.Input{
			WeekStart: f.WeekStart, Counts: f.Counts, Deals: f.Deals,
		},
		want: want,
	}, nil
}

type weeklyNarrativeCase struct {
	in   narrative.Input
	want weeklyNarrativeExpectation
}

// Run issues the one request this site sends.
//
// English is pinned rather than taken from an installation: the scores have to
// be comparable across deployments, and a case scored in one language and
// re-scored in another is measuring the translation.
func (c *weeklyNarrativeCase) Run(ctx context.Context, completer aitasks.Completer) (aitasks.Trace, error) {
	req := narrative.Request(c.in, string(textlang.English))
	trace := aitasks.Trace{Requests: []model.Request{req}}
	resp, err := completer.Complete(ctx, req)
	if err != nil {
		return trace, fmt.Errorf("weekly_review/narrative: %w", err)
	}
	trace.Output = resp.Text
	return trace, nil
}

// Evaluate applies the lane's own parser first, then asks whether the sentence
// says what the week actually held.
//
// The order is the meaning: a reply the lane could not read has no sentence to
// judge, and scoring it against the expectation would credit or blame a model
// for prose the product would have thrown away.
func (c *weeklyNarrativeCase) Evaluate(trace aitasks.Trace) aitasks.Outcome {
	sentence, err := narrative.Parse(trace.Output, c.in)
	if err != nil {
		return aitasks.Outcome{
			Result: aitasks.OutcomeInvalid,
			Detail: fmt.Sprintf("the lane refused the reply: %v", err),
		}
	}
	if sentence == "" {
		// A blank sentence is a real answer the lane stores, but it is an
		// abstention rather than a reading: it asserted nothing about the week.
		return aitasks.Outcome{
			Result: aitasks.OutcomeAbstained,
			Detail: "the reply carried no sentence",
		}
	}
	lower := strings.ToLower(sentence)
	for _, must := range c.want.MustMention {
		if !strings.Contains(lower, strings.ToLower(must)) {
			return aitasks.Outcome{
				Result: aitasks.OutcomeWrongAnswer,
				Detail: fmt.Sprintf("the sentence does not mention %q: %q", must, sentence),
			}
		}
	}
	for _, never := range c.want.MustNotMention {
		if strings.Contains(lower, strings.ToLower(never)) {
			return aitasks.Outcome{
				Result: aitasks.OutcomeWrongAnswer,
				Detail: fmt.Sprintf("the sentence reaches past the week it was given (%q): %q", never, sentence),
			}
		}
	}
	return aitasks.Outcome{Result: aitasks.OutcomeAccepted}
}
