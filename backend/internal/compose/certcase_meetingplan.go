// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The certification case for summarize/meeting_plan.
//
// It certifies the shipped path: the request comes from
// meetingbrief.PlanRequest and the reply is read by meetingbrief.ParsePlan —
// the two the service itself calls. A case that rebuilt either would measure a
// copy, and a copy stays green through the change that breaks the original.
//
// WHAT THIS MEASURES, and why it is not "the plan parsed". ParsePlan already
// drops anything ungrounded, so a reply saying nothing survives it and comes
// back as an empty plan; a scenario asserting only that it parsed would pass
// forever without the model contributing. The two things production cannot
// guarantee are the ones measured here:
//
//   1. Did the plan reach the conversation that MATTERS? The fixture plants
//      several moments and names one whose message a right answer must cite.
//      This is the entailment check: a citation is only grounded against the
//      record SET, so a confident sentence hung on an unrelated-but-known
//      record passes every deterministic filter in the tree.
//   2. Is the question account-specific? The fixture names a token only this
//      account would produce, and a plan whose questions never mention it is a
//      questionnaire that would read the same about anybody.
//
// Ids are MINTED by Prepare and never written in the corpus. The model is asked
// to return ids it was handed, so a corpus-supplied id would be one whoever
// wrote the expected reply could copy in, and a model echoing it would be
// indistinguishable from one that read the right conversation.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/compose/meetingbrief"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// meetingPlanFixture is one meeting and the conversations behind it.
type meetingPlanFixture struct {
	Subject  string                `json:"subject"`
	Company  string                `json:"company"`
	Attendee string                `json:"attendee"`
	Messages []meetingPlanMessage  `json:"messages"`
	Claims   []meetingPlanClaimRow `json:"claims"`
}

// meetingPlanMessage is one captured conversation, labelled so the expectation
// can name it without naming an id.
type meetingPlanMessage struct {
	Label     string `json:"label"`
	DaysAgo   int    `json:"days_ago"`
	Direction string `json:"direction"`
	Subject   string `json:"subject"`
	Body      string `json:"body"`
}

// meetingPlanClaimRow is one extracted commitment or question.
type meetingPlanClaimRow struct {
	Kind      string `json:"kind"`
	Body      string `json:"body"`
	FromLabel string `json:"from_label"`
}

// meetingPlanExpectation is what a right answer looks like.
type meetingPlanExpectation struct {
	// CitesLabel is the conversation the plan must have read. Named by label,
	// resolved to a minted id at Prepare time.
	CitesLabel string `json:"cites_label"`
	// NamesToken is a word only this account produces. A plan whose prose
	// never contains it is generic, whatever else it got right.
	NamesToken string `json:"names_token"`
}

// meetingPlanCases serves the site that prepares a rep for one meeting.
type meetingPlanCases struct{}

func (meetingPlanCases) Site() aitasks.Site {
	return aitasks.Site{
		Task:    ai.TaskSummarize,
		Variant: "meeting_plan",
		Kind:    ai.SiteKindOneShot,
	}
}

// Prepare builds the input production assembles, minting an id per message.
//
//nolint:ireturn // PreparedCase IS the seam: one implementation per site behind the one interface the cert lane runs.
func (meetingPlanCases) Prepare(fixture, expected json.RawMessage) (aitasks.PreparedCase, error) {
	var f meetingPlanFixture
	if err := json.Unmarshal(fixture, &f); err != nil {
		return nil, fmt.Errorf("summarize/meeting_plan: the fixture is not the shape this site takes: %w", err)
	}
	var want meetingPlanExpectation
	if err := json.Unmarshal(expected, &want); err != nil {
		return nil, fmt.Errorf("summarize/meeting_plan: the expected answer is not this site's shape: %w", err)
	}
	if err := refuseUnpreparableMeeting(f, want); err != nil {
		return nil, err
	}
	in, byLabel := meetingPlanInput(f)
	return &meetingPlanCase{
		in:        in,
		mustCite:  byLabel[want.CitesLabel],
		mustName:  want.NamesToken,
		citeLabel: want.CitesLabel,
	}, nil
}

// refuseUnpreparableMeeting names a fixture or expectation that would measure
// nothing, at parse time rather than after a paid run.
func refuseUnpreparableMeeting(f meetingPlanFixture, want meetingPlanExpectation) error {
	if len(f.Messages) < 2 {
		return fmt.Errorf(
			"summarize/meeting_plan: the fixture supplies %d conversation(s); with fewer than two there is no wrong one to cite",
			len(f.Messages))
	}
	if strings.TrimSpace(want.NamesToken) == "" {
		return errors.New(
			"summarize/meeting_plan: the expectation names no account-specific token, so a generic plan would satisfy it")
	}
	seen := map[string]bool{}
	for i, message := range f.Messages {
		if strings.TrimSpace(message.Label) == "" {
			return fmt.Errorf("summarize/meeting_plan: the message at position %d carries no label", i+1)
		}
		if seen[message.Label] {
			return fmt.Errorf(
				"summarize/meeting_plan: two messages are labelled %q, so an expectation naming it means neither",
				message.Label)
		}
		seen[message.Label] = true
	}
	if !seen[want.CitesLabel] {
		return fmt.Errorf(
			"summarize/meeting_plan: the expectation names %q, which the fixture does not carry — no reply could satisfy it",
			want.CitesLabel)
	}
	if !strings.Contains(bodiesOf(f), want.NamesToken) {
		return fmt.Errorf(
			"summarize/meeting_plan: the expectation's token %q appears in no message, so only an invented plan could name it",
			want.NamesToken)
	}
	// And the token must be something the FLOOR does not already say. The
	// deterministic plan quotes captured claims, so a token drawn from one
	// would be in the prose whatever the model returned — the scenario would
	// pass forever without the model contributing anything, which is the one
	// way a certification case fails silently.
	in, _ := meetingPlanInput(f)
	if strings.Contains(meetingbrief.PlanProse(meetingbrief.DeterministicPlanFor(in)), want.NamesToken) {
		return fmt.Errorf(
			"summarize/meeting_plan: the token %q is already in the deterministic plan's own prose, so a reply saying nothing would satisfy this scenario",
			want.NamesToken)
	}
	return nil
}

func bodiesOf(f meetingPlanFixture) string {
	var all strings.Builder
	for _, message := range f.Messages {
		all.WriteString(message.Subject)
		all.WriteString(" ")
		all.WriteString(message.Body)
		all.WriteString(" ")
	}
	return all.String()
}

// meetingPlanInput assembles what the service assembles, with minted ids.
func meetingPlanInput(f meetingPlanFixture) (meetingbrief.Input, map[string]string) {
	now := time.Date(2026, time.August, 4, 12, 0, 0, 0, time.UTC)
	meeting := ids.NewV7().String()
	person := ids.NewV7().String()
	in := meetingbrief.Input{
		ActivityID: meeting,
		Subject:    f.Subject,
		StartsAt:   now.Add(24 * time.Hour),
		Now:        now,
		Company:    f.Company,
		Attendees: []meetingbrief.AttendeeIn{
			{PersonID: person, FullName: f.Attendee},
		},
	}
	byLabel := map[string]string{}
	for _, message := range f.Messages {
		id := ids.NewV7().String()
		byLabel[message.Label] = id
		at := now.AddDate(0, 0, -message.DaysAgo)
		in.History = append(in.History, meetingbrief.HistoryIn{
			ID: id, Kind: "email", Subject: message.Subject,
			Direction: message.Direction, At: at,
		})
		in.Excerpts = append(in.Excerpts, meetingbrief.ExcerptIn{
			ActivityID: id, Subject: message.Subject,
			Direction: message.Direction, At: at, Text: message.Body,
		})
	}
	for _, claim := range f.Claims {
		in.Commitments = append(in.Commitments, meetingbrief.ClaimIn{
			PersonName: f.Attendee, Kind: claim.Kind, Body: claim.Body,
			Status: "open", SourceID: byLabel[claim.FromLabel],
		})
	}
	return in, byLabel
}

// meetingPlanCase is one prepared meeting.
type meetingPlanCase struct {
	in        meetingbrief.Input
	mustCite  string
	mustName  string
	citeLabel string
}

func (c *meetingPlanCase) Run(ctx context.Context, completer aitasks.Completer) (aitasks.Trace, error) {
	floor := meetingbrief.DeterministicPlanFor(c.in)
	req := meetingbrief.PlanRequest(meetingbrief.PlanPromptFor(c.in, floor), "en")
	trace := aitasks.Trace{Requests: []model.Request{req}}
	resp, err := completer.Complete(ctx, req)
	if err != nil {
		return trace, fmt.Errorf("summarize/meeting_plan: %w", err)
	}
	trace.Output = resp.Text
	return trace, nil
}

// Evaluate runs production's own parser and then asks the two questions
// production cannot answer for itself.
//
// A reply the parser refused is OutcomeInvalid rather than a wrong answer: the
// model produced something unusable, which is a different event from producing
// a usable plan about the wrong conversation.
func (c *meetingPlanCase) Evaluate(trace aitasks.Trace) aitasks.Outcome {
	floor := meetingbrief.DeterministicPlanFor(c.in)
	plan, err := meetingbrief.ParsePlan(trace.Output, c.in, floor)
	if err != nil {
		return aitasks.Outcome{
			Result: aitasks.OutcomeInvalid,
			Detail: fmt.Sprintf("the plan validator refused the reply: %v", err),
		}
	}
	var wrong []string
	if !meetingbrief.PlanCites(plan, c.mustCite) {
		wrong = append(wrong, fmt.Sprintf(
			"the plan never cites %q, the conversation this meeting is about", c.citeLabel))
	}
	if !strings.Contains(meetingbrief.PlanProse(plan), c.mustName) {
		wrong = append(wrong, fmt.Sprintf(
			"the plan never names %q, so it would read the same about any account", c.mustName))
	}
	if len(wrong) > 0 {
		return aitasks.Outcome{Result: aitasks.OutcomeWrongAnswer, Detail: strings.Join(wrong, "; ")}
	}
	return aitasks.Outcome{Result: aitasks.OutcomeAccepted}
}
