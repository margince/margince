// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The weekly learnings' certification site.
//
// The request is built by the learnings package's own writer and the reply is
// read by its own parser: a case that rebuilt either would measure a copy of
// the prompt rather than the prompt.
//
// WHAT THIS SITE CERTIFIES IS ABSTENTION, and that is what makes it different
// from the narrative's case beside it. A narrative is judged on whether it says
// what the week held; a learning is judged on whether the model would rather
// say nothing than say something it cannot point at. So the corpus carries
// weeks that INVITE a lesson and do not support one, and abstaining on those is
// the correct answer rather than a failure to score.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/compose/weekly/learnings"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// weeklyLearningsFixture is one week, as a case states it.
type weeklyLearningsFixture struct {
	WeekStart   string              `json:"week_start"`
	Counts      learnings.Counts    `json:"counts"`
	Deals       []learnings.Subject `json:"deals"`
	Commitments []learnings.Subject `json:"commitments"`
}

// weeklyLearningsExpectation is what the pass must and must not conclude.
type weeklyLearningsExpectation struct {
	// MustAbstain says the honest answer is no learnings at all. A week that
	// invites a lesson it cannot support is the case this site exists for, and
	// producing one for it is the failure — not a low score.
	MustAbstain bool `json:"must_abstain"`
	// MustCite are labels whose rows a grounded reply has to rest on. Matched
	// against the CITATIONS rather than the prose: a learning that names a deal
	// in its sentence and cites nothing is exactly the shape being refused.
	MustCite []string `json:"must_cite"`
	// MustNotMention are words that would mean the model reached past the week:
	// a company nobody gave it, a comparison to a week it cannot see.
	MustNotMention []string `json:"must_not_mention"`
}

// weeklyLearningsCases serves the one site that says what a week taught.
type weeklyLearningsCases struct{}

func (weeklyLearningsCases) Site() aitasks.Site {
	return aitasks.Site{
		Task:    ai.TaskWeeklyLearnings,
		Variant: "learn",
		Kind:    ai.SiteKindOneShot,
	}
}

// Prepare turns one week and its expectation into a runnable case.
//
//nolint:ireturn // PreparedCase IS the seam: one implementation per site behind the one interface the cert lane runs.
func (weeklyLearningsCases) Prepare(fixture, expected json.RawMessage) (aitasks.PreparedCase, error) {
	var f weeklyLearningsFixture
	if err := json.Unmarshal(fixture, &f); err != nil {
		return nil, fmt.Errorf("weekly_learnings/learn: the fixture is not the shape this site takes: %w", err)
	}
	if f.WeekStart == "" {
		return nil, fmt.Errorf("weekly_learnings/learn: the fixture names no week")
	}
	var want weeklyLearningsExpectation
	if err := json.Unmarshal(expected, &want); err != nil {
		return nil, fmt.Errorf(
			"weekly_learnings/learn: the expected answer is not an abstain/cite shape: %w", err)
	}
	if !want.MustAbstain && len(want.MustCite) == 0 && len(want.MustNotMention) == 0 {
		// A case that asserts nothing scores every reply as correct, which is
		// worse than no case: it reports a certified site nobody measured.
		return nil, fmt.Errorf("weekly_learnings/learn: the expectation asserts nothing")
	}
	in := learnings.Input{
		WeekStart: f.WeekStart, Counts: f.Counts,
		Deals: f.Deals, Commitments: f.Commitments,
	}
	// A fixture below the floor would never reach the model in production, so a
	// case built on one would certify a call the product does not make.
	if !learnings.Floor(in) {
		return nil, fmt.Errorf(
			"weekly_learnings/learn: the fixture holds %d citable rows, below the floor the lane checks before it calls",
			len(f.Deals)+len(f.Commitments))
	}
	return &weeklyLearningsCase{in: in, want: want}, nil
}

type weeklyLearningsCase struct {
	in   learnings.Input
	want weeklyLearningsExpectation
}

// Run issues the one request this site sends.
//
// English is pinned rather than taken from an installation: the scores have to
// be comparable across deployments, and a case scored in one language and
// re-scored in another is measuring the translation.
func (c *weeklyLearningsCase) Run(ctx context.Context, completer aitasks.Completer) (aitasks.Trace, error) {
	req := learnings.Request(c.in, string(textlang.English))
	trace := aitasks.Trace{Requests: []model.Request{req}}
	resp, err := completer.Complete(ctx, req)
	if err != nil {
		return trace, fmt.Errorf("weekly_learnings/learn: %w", err)
	}
	trace.Output = resp.Text
	return trace, nil
}

// Evaluate applies the lane's own parser first, then asks whether the pass drew
// what the week supports.
//
// A REFUSED REPLY IS INVALID, NOT AN ABSTENTION, and the difference matters:
// abstaining is the model declining to claim, while a refusal is the model
// having claimed something it could not point at. Scoring the second as the
// first would report restraint the model did not show.
func (c *weeklyLearningsCase) Evaluate(trace aitasks.Trace) aitasks.Outcome {
	items, err := learnings.Parse(trace.Output, c.in)
	if err != nil {
		return aitasks.Outcome{
			Result: aitasks.OutcomeInvalid,
			Detail: fmt.Sprintf("the lane refused the reply: %v", err),
		}
	}
	if len(items) == 0 {
		if c.want.MustAbstain {
			// The week invited a lesson it could not support, and the model
			// declined. That is the answer this site is here to measure.
			return aitasks.Outcome{Result: aitasks.OutcomeAccepted}
		}
		return aitasks.Outcome{
			Result: aitasks.OutcomeAbstained,
			Detail: "the reply carried no learnings",
		}
	}
	if c.want.MustAbstain {
		return aitasks.Outcome{
			Result: aitasks.OutcomeWrongAnswer,
			Detail: fmt.Sprintf(
				"the week supports no lesson and the reply drew %d: %q", len(items), items[0].Text),
		}
	}
	cited := citedLabels(items)
	for _, must := range c.want.MustCite {
		if !cited[strings.ToLower(must)] {
			return aitasks.Outcome{
				Result: aitasks.OutcomeWrongAnswer,
				Detail: fmt.Sprintf("no learning rests on %q", must),
			}
		}
	}
	prose := strings.ToLower(allText(items))
	for _, never := range c.want.MustNotMention {
		if strings.Contains(prose, strings.ToLower(never)) {
			return aitasks.Outcome{
				Result: aitasks.OutcomeWrongAnswer,
				Detail: fmt.Sprintf("a learning reaches past the week it was given (%q)", never),
			}
		}
	}
	return aitasks.Outcome{Result: aitasks.OutcomeAccepted}
}

// citedLabels folds the rows this reply rests on, for comparison against the
// labels a case asked for.
func citedLabels(items []learnings.Learning) map[string]bool {
	cited := map[string]bool{}
	for _, item := range items {
		for _, c := range item.Citations {
			cited[strings.ToLower(c.Label)] = true
		}
	}
	return cited
}

// allText is the reply's prose, for the must-not-mention check.
func allText(items []learnings.Learning) string {
	var b strings.Builder
	for _, item := range items {
		b.WriteString(item.Text)
		b.WriteString("\n")
	}
	return b.String()
}
