// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The certification case for summarize/person_brief.
//
// It certifies the shipped path: the request comes from personbrief.BriefRequest
// and the reply is read by personbrief.ParseBrief — the two the service itself
// calls. A case that rebuilt either would measure a copy, and a copy stays green
// through the change that breaks the original.
//
// WHAT THIS MEASURES, and why it is not "the brief parsed". ParseBrief already
// drops anything ungrounded, so a reply saying nothing survives it and comes
// back as an empty brief; a scenario asserting only that it parsed would pass
// forever without the model contributing. The two things production cannot
// guarantee are the ones measured here:
//
//  1. Did the brief read the message that MATTERS? The fixture plants several
//     and names one whose id a right answer must cite. This is the entailment
//     check: a citation is only grounded against the record SET, so a confident
//     sentence hung on an unrelated-but-known record passes every deterministic
//     filter in the tree.
//  2. Is it about THIS relationship? The fixture names a phrase only this
//     contact produced, and a brief whose prose never contains it would read
//     the same about anybody — which is the complaint the model lane exists to
//     answer. A scenario may also forbid phrases, which is how silence about a
//     withheld section is checked rather than merely asked of a judge.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/compose/personbrief"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// personBriefCases serves the person page's standing relationship brief.
type personBriefCases struct{}

func (personBriefCases) Site() aitasks.Site {
	return aitasks.Site{Task: ai.TaskSummarize, Variant: "person_brief", Kind: ai.SiteKindOneShot}
}

// Prepare builds the input production assembles, minting an id per record.
//
//nolint:ireturn // PreparedCase IS the seam: one implementation per site behind the one interface the cert lane runs.
func (personBriefCases) Prepare(fixture, expected json.RawMessage) (aitasks.PreparedCase, error) {
	var f personBriefFixture
	if err := json.Unmarshal(fixture, &f); err != nil {
		return nil, fmt.Errorf("summarize/person_brief: the fixture is not the shape this site takes: %w", err)
	}
	var want personBriefExpectation
	if err := json.Unmarshal(expected, &want); err != nil {
		return nil, fmt.Errorf("summarize/person_brief: the expected answer is not this site's shape: %w", err)
	}
	if err := refuseUnpreparableBrief(f, want); err != nil {
		return nil, err
	}
	in, byLabel := personBriefInput(f)
	return &personBriefCase{
		in: in, personID: ids.NewV7().String(),
		mustCite: byLabel[want.CitesLabel], citeLabel: want.CitesLabel,
		mustName: want.NamesToken, mustAvoid: want.Avoids,
	}, nil
}

// personBriefCase is one prepared relationship.
type personBriefCase struct {
	in        personbrief.Input
	personID  string
	mustCite  string
	citeLabel string
	mustName  string
	mustAvoid []string
}

// Run issues the one request this site sends, through the production writer's
// own request builder.
//
// English, pinned, rather than the installation's base language: a certification
// record grades a fixed corpus, and a score that moved with a settings row would
// not be comparable between installations.
func (c *personBriefCase) Run(ctx context.Context, completer aitasks.Completer) (aitasks.Trace, error) {
	req := personbrief.BriefRequest(c.in, string(textlang.English))
	trace := aitasks.Trace{Requests: []model.Request{req}}
	resp, err := completer.Complete(ctx, req)
	if err != nil {
		return trace, fmt.Errorf("summarize/person_brief: %w", err)
	}
	trace.Output = resp.Text
	return trace, nil
}

// Evaluate runs the production grounding filter and then asks the two questions
// production cannot answer for itself.
//
// A reply the filter refused outright is OutcomeInvalid; one whose every
// sentence was dropped for citing nothing is OutcomeAbstained, which is what
// production shows as a card with no prose. Neither is a wrong answer, and
// reporting them as one would hide which of the three actually happened.
func (c *personBriefCase) Evaluate(trace aitasks.Trace) aitasks.Outcome {
	sentences, err := personbrief.ParseBrief(trace.Output, c.personID, c.in)
	if err != nil {
		return aitasks.Outcome{
			Result: aitasks.OutcomeInvalid,
			Detail: fmt.Sprintf("the grounding filter refused the reply: %v", err),
		}
	}
	if len(sentences) == 0 {
		return aitasks.Outcome{
			Result: aitasks.OutcomeAbstained,
			Detail: "no sentence cited a record of this relationship",
		}
	}
	if wrong := c.faults(sentences); len(wrong) > 0 {
		return aitasks.Outcome{Result: aitasks.OutcomeWrongAnswer, Detail: strings.Join(wrong, "; ")}
	}
	return aitasks.Outcome{Result: aitasks.OutcomeAccepted}
}

// faults names everything a right brief would not have done, all of it at once:
// a run that reports only the first fault costs a second paid run to learn the
// second.
func (c *personBriefCase) faults(sentences []personbrief.Sentence) []string {
	var wrong []string
	if !cites(sentences, c.mustCite) {
		wrong = append(wrong, fmt.Sprintf(
			"the brief never cites %q, the message this relationship turns on", c.citeLabel))
	}
	// Folded once: the phrases are the scenario author's own words and the prose
	// is the model's, so both comparisons are on the reader's terms rather than
	// on either side's capitalisation. Prepare folds the same way, so a token it
	// cleared as beyond the floor is the token measured here.
	folded := strings.ToLower(personbrief.Prose(sentences))
	if !strings.Contains(folded, strings.ToLower(c.mustName)) {
		wrong = append(wrong, fmt.Sprintf(
			"the brief never names %q, so it would read the same about any contact", c.mustName))
	}
	for _, avoided := range c.mustAvoid {
		if strings.Contains(folded, strings.ToLower(avoided)) {
			wrong = append(wrong, fmt.Sprintf(
				"the brief says %q, which this reader was not shown", avoided))
		}
	}
	return wrong
}

// cites reports whether any surviving sentence points at one record.
func cites(sentences []personbrief.Sentence, entityID string) bool {
	for _, sentence := range sentences {
		for _, evidence := range sentence.Evidence {
			if evidence.EntityID == entityID {
				return true
			}
		}
	}
	return false
}
