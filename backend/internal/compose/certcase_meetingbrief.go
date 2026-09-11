// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The certification case for summarize/meeting_brief.
//
// The brief's SECTIONS and its PLAN are two calls under one request
// (meetingbrief.Service.write), two prompts, and two readers of the reply. The
// plan had a case and the sections did not, so this site's prompt reached a
// provider with nothing measuring what came back — which is the gap
// TestEveryPromptIsCertified now refuses.
//
// It certifies the shipped path: the request is meetingbrief.BriefRequest and
// the reply is read by meetingbrief.ParseBriefSections, because that parser is
// the grounding filter standing between a reader and a sentence about a record
// they cannot open. A case that rebuilt either would measure a copy, and a copy
// stays green through the change that breaks the original.
//
// The fixture and the expectation are the PLAN's, deliberately: both sites read
// one assembled meeting, so a second fixture shape would be two spellings of
// one scenario, free to drift until the two sites were certified against
// different meetings. What differs is the floor each degrades to, and each
// checks its own.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/compose/meetingbrief"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// meetingBriefCases serves the sections beside the plan.
type meetingBriefCases struct{}

func (meetingBriefCases) Site() aitasks.Site {
	return aitasks.Site{
		Task:    ai.TaskSummarize,
		Variant: "meeting_brief",
		Kind:    ai.SiteKindOneShot,
	}
}

// Prepare builds the input production assembles, minting an id per message.
//
//nolint:ireturn // PreparedCase IS the seam: one implementation per site behind the one interface the cert lane runs.
func (meetingBriefCases) Prepare(fixture, expected json.RawMessage) (aitasks.PreparedCase, error) {
	var f meetingPlanFixture
	if err := json.Unmarshal(fixture, &f); err != nil {
		return nil, fmt.Errorf("summarize/meeting_brief: the fixture is not the shape this site takes: %w", err)
	}
	var want meetingPlanExpectation
	if err := json.Unmarshal(expected, &want); err != nil {
		return nil, fmt.Errorf("summarize/meeting_brief: the expected answer is not this site's shape: %w", err)
	}
	if err := refuseUnpreparableMeetingBrief(f, want); err != nil {
		return nil, err
	}
	in, byLabel := meetingPlanInput(f)
	return &meetingBriefCase{
		in:        in,
		mustCite:  byLabel[want.CitesLabel],
		mustName:  want.NamesToken,
		citeLabel: want.CitesLabel,
	}, nil
}

// refuseUnpreparableMeetingBrief adds this site's own floor check to the shared one.
func refuseUnpreparableMeetingBrief(f meetingPlanFixture, want meetingPlanExpectation) error {
	if err := refuseUnusableMeetingFixture("summarize/meeting_brief", f, want); err != nil {
		return err
	}
	// The deterministic sections quote captured claims, so a token drawn from
	// one would be in the prose whatever the model returned — the scenario
	// would pass forever without the model contributing anything, which is the
	// one way a certification case fails silently.
	in, _ := meetingPlanInput(f)
	if strings.Contains(sectionProse(meetingbrief.Deterministic(in)), want.NamesToken) {
		return fmt.Errorf(
			"summarize/meeting_brief: the token %q is already in the deterministic sections' own prose, so a reply saying nothing would satisfy this scenario",
			want.NamesToken)
	}
	return nil
}

// sectionProse joins a brief's written lines, for asking whether a token
// appears anywhere in it.
func sectionProse(sections []meetingbrief.Section) string {
	var all strings.Builder
	for _, section := range sections {
		for _, sentence := range section.Sentences {
			all.WriteString(sentence.Text)
			all.WriteString(" ")
		}
	}
	return all.String()
}

// meetingBriefCase is one prepared meeting, read for its sections.
type meetingBriefCase struct {
	in        meetingbrief.Input
	mustCite  string
	mustName  string
	citeLabel string
}

// Run issues the one request this site sends, through the production writer's
// own request builder.
func (c *meetingBriefCase) Run(ctx context.Context, completer aitasks.Completer) (aitasks.Trace, error) {
	// English, pinned, rather than the installation's base language: a
	// certification record grades a fixed corpus, and a score that moved with a
	// settings row would not be comparable between installations.
	req := meetingbrief.BriefRequest(c.in, string(textlang.English))
	trace := aitasks.Trace{Requests: []model.Request{req}}
	resp, err := completer.Complete(ctx, req)
	if err != nil {
		return trace, fmt.Errorf("summarize/meeting_brief: %w", err)
	}
	trace.Output = resp.Text
	return trace, nil
}

// Evaluate runs the production grounding filter and asks whether the surviving
// sections are about the conversation this meeting is about, in this account's
// own words.
func (c *meetingBriefCase) Evaluate(trace aitasks.Trace) aitasks.Outcome {
	sections, err := meetingbrief.ParseBriefSections(trace.Output, c.in)
	if err != nil {
		return aitasks.Outcome{Result: aitasks.OutcomeInvalid, Detail: err.Error()}
	}
	if len(sections) == 0 {
		// Every sentence was dropped for citing nothing in the meeting: the
		// model wrote about something else, which production shows as the
		// deterministic floor rather than as prose.
		return aitasks.Outcome{
			Result: aitasks.OutcomeAbstained,
			Detail: "no sentence cited a record of this meeting",
		}
	}
	// ONE sentence has to do both. Checking the two independently accepts a
	// brief that cites the right conversation in a generic sentence and names
	// the account's own words in a sentence grounded somewhere else — two
	// half-right claims reading as one right one.
	cited, grounded := false, false
	for _, section := range sections {
		for _, sentence := range section.Sentences {
			citesTheThread := false
			for _, evidence := range sentence.Evidence {
				if evidence.EntityID == c.mustCite {
					cited = true
					citesTheThread = true
				}
			}
			if citesTheThread && strings.Contains(sentence.Text, c.mustName) {
				grounded = true
			}
		}
	}
	switch {
	case !cited:
		return aitasks.Outcome{
			Result: aitasks.OutcomeWrongAnswer,
			Detail: "never cited: " + c.citeLabel,
		}
	case !grounded:
		return aitasks.Outcome{
			Result: aitasks.OutcomeWrongAnswer,
			Detail: fmt.Sprintf(
				"no sentence both cited %s and named %q, so the brief is either generic or grounded elsewhere",
				c.citeLabel, c.mustName),
		}
	}
	return aitasks.Outcome{Result: aitasks.OutcomeAccepted}
}
