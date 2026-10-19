// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The certification case for transcript_propose/next_steps.
//
// It certifies the shipped path: the request comes from transcriptRequest and
// the reply is judged by validateTranscriptPayload, the same builder and the
// same validator the engine uses. A case that rebuilt either would measure a
// copy, and a copy stays green through the change that breaks the original.
//
// What the expectation MEANS here: the set of next steps the transcript states,
// each named by the LINE it is stated on. The line number is an identity the
// corpus can name because the transcript itself supplies it — unlike the thread
// read, this site mints nothing, so a scenario author counts lines in the
// fixture and writes the number down. An answer can only carry a line number by
// having read this call's prompt, which is what the citation check exists to
// prove.
//
// The empty expectation is the important one. Most meetings state no
// commitment anyone could act on, and a site that invents one puts a question in
// front of a rep that the transcript never asked.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// transcriptProposeFixture is ONE transcript, in order, as the transcript read
// hands it to the engine: one string per line, numbered from 1 by the prompt.
type transcriptProposeFixture []string

// transcriptProposeExpectation identifies the cited line and verified promiser.
type transcriptProposeExpectation struct {
	Promiser string `json:"promiser,omitempty"`
	Line     int    `json:"line"`
	DueDate  string `json:"due_date"`
}

// transcriptProposeCases serves the one site that reads next steps out of a
// meeting transcript.
type transcriptProposeCases struct{}

func (transcriptProposeCases) Site() aitasks.Site {
	return aitasks.Site{
		Task:    ai.TaskTranscriptPropose,
		Variant: "next_steps",
		Kind:    ai.SiteKindOneShot,
	}
}

// CertifiedScope is the site's own kind: reading a transcript IS one call.
// There is no solo re-ask here — a proposal under the floor is dropped, not
// asked about again — so the call this case makes is the whole path.
func (transcriptProposeCases) CertifiedScope() string { return aitasks.ScopeFullInvocation }

// Prepare turns one transcript and the next steps the scenario expects into a
// runnable case.
//
//nolint:ireturn // PreparedCase IS the seam: one implementation per site behind the one interface the cert lane runs.
func (transcriptProposeCases) Prepare(fixture, expected json.RawMessage) (aitasks.PreparedCase, error) {
	var input struct {
		Lines        []string              `json:"lines"`
		Participants []certCommitmentParty `json:"participants"`
	}
	if bytes.HasPrefix(bytes.TrimSpace(fixture), []byte("[")) {
		if err := json.Unmarshal(fixture, &input.Lines); err != nil {
			return nil, fmt.Errorf("transcript_propose/next_steps: invalid fixture: %w", err)
		}
	} else if err := json.Unmarshal(fixture, &input); err != nil {
		return nil, fmt.Errorf("transcript_propose/next_steps: invalid fixture: %w", err)
	}
	lines := input.Lines
	parties, references, err := prepareCommitmentParties(input.Participants, map[string]ids.UUID{})
	if err != nil {
		return nil, err
	}
	if err := refuseUnreadableTranscript(lines); err != nil {
		return nil, err
	}
	var want []transcriptProposeExpectation
	if err := json.Unmarshal(expected, &want); err != nil {
		return nil, fmt.Errorf(
			"transcript_propose/next_steps: the expected answer is not a list of next steps: %w", err)
	}
	if err := refuseUnreachableSteps(want, len(lines)); err != nil {
		return nil, err
	}
	for i := range want {
		if want[i].Promiser == "" {
			continue
		}
		_, found := references[want[i].Promiser]
		if !found {
			return nil, fmt.Errorf("expected promiser %q is not a verified participant", want[i].Promiser)
		}
	}
	return &transcriptProposeCase{lines: lines, parties: parties, expected: want, promiserKeys: certPromiserKeys(references)}, nil
}

// refuseUnreadableTranscript names a transcript the read could never have been
// given: the engine refuses one past withinReadingBounds outright, and every
// line is citable evidence the approval quotes back — a blank one would be
// offered to a human as the text a commitment was read from.
func refuseUnreadableTranscript(lines transcriptProposeFixture) error {
	if len(lines) == 0 {
		return errors.New(
			"transcript_propose/next_steps: the fixture supplies no line, so there is no transcript to read")
	}
	if err := activities.WithinReadingBounds(lines); err != nil {
		return fmt.Errorf("transcript_propose/next_steps: %w", err)
	}
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			return fmt.Errorf(
				"transcript_propose/next_steps: line %d is blank, and a proposal citing it would quote nothing as its evidence",
				i+1)
		}
	}
	return nil
}

// refuseUnreachableSteps names an expectation the validator can never satisfy,
// which would measure nothing for as long as it stayed in the corpus. An EMPTY
// expectation is not one of them — it is the abstention scenario, and it is the
// one most transcripts deserve.
func refuseUnreachableSteps(want []transcriptProposeExpectation, lineCount int) error {
	if len(want) > maxTranscriptProposals {
		return fmt.Errorf(
			"transcript_propose/next_steps: the scenario expects %d next steps, but this site proposes at most %d",
			len(want), maxTranscriptProposals)
	}
	for i, step := range want {
		if step.Line < 1 || step.Line > lineCount {
			return fmt.Errorf(
				"transcript_propose/next_steps: expectation %d cites line %d, and this transcript has lines 1 to %d",
				i+1, step.Line, lineCount)
		}
	}
	return nil
}

// A fixed meeting day makes relative deadline grades comparable across runs.
const certTranscriptMeetingDay = "2026-03-04"

type transcriptProposeCase struct {
	promiserKeys map[string]string
	parties      []commitmentParty
	lines        []string
	expected     []transcriptProposeExpectation
}

// Run issues the one request this site sends, bare: production wraps the same
// request in the shape-retry when the brain supports one, and a case that did
// too would certify the answer a model gives after being told to try again.
func (c *transcriptProposeCase) Run(ctx context.Context, completer aitasks.Completer) (aitasks.Trace, error) {
	// English, pinned, rather than the installation's base language: a
	// certification record grades a fixed corpus, and a score that moved with a
	// settings row would not be comparable between two installations or across
	// one that changed its mind. The rule is PRESENT in the graded request for
	// the same reason — production sends one, so a case that left it out would
	// grade a prompt the product does not send.
	// The meeting day is PINNED for the same reason the language is: a case
	// whose prompt carried today's date would grade a different request every
	// morning, and any deadline a corpus case states relative to the meeting
	// would resolve to a different day each run.
	req := transcriptRequest(c.lines, certTranscriptMeetingDay, string(textlang.English), c.parties)
	trace := aitasks.Trace{Requests: []model.Request{req}}
	resp, err := completer.Complete(ctx, req)
	if err != nil {
		return trace, fmt.Errorf("transcript_propose/next_steps: %w", err)
	}
	trace.Output = resp.Text
	return trace, nil
}

// Evaluate applies the engine's own checks in the engine's own order — parse,
// then validateTranscriptPayload against the transcript that was asked about —
// and only then asks whether the next steps are the ones the scenario expects. A
// reply that fails the validator has no proposals to disagree with.
//
// The confidence floor is deliberately not applied. It is the engine's decision
// about what to do with a reading it already believes, not a judgement on
// whether the reading is usable, and folding it in would report a hedged correct
// reading as a broken reply.
func (c *transcriptProposeCase) Evaluate(trace aitasks.Trace) aitasks.Outcome {
	var payload transcriptPayload
	if err := json.Unmarshal([]byte(ai.Unfence(trace.Output)), &payload); err != nil {
		return aitasks.Outcome{
			Result: aitasks.OutcomeInvalid,
			Detail: fmt.Sprintf("unparseable model output: %v", err),
		}
	}
	if msg := validateTranscriptPayload(payload, len(c.lines), c.parties); msg != "" {
		return aitasks.Outcome{Result: aitasks.OutcomeInvalid, Detail: msg}
	}
	if disagreements := c.disagreements(payload); len(disagreements) > 0 {
		return aitasks.Outcome{Result: aitasks.OutcomeWrongAnswer, Detail: strings.Join(disagreements, "; ")}
	}
	return aitasks.Outcome{Result: aitasks.OutcomeAccepted}
}

// AnswerConfidence is the confidence range over the next steps the reply
// proposes: each one is a commitment the task-creation threshold is set against.
func (c *transcriptProposeCase) AnswerConfidence(trace aitasks.Trace) (low, high float64, ok bool) {
	var payload transcriptPayload
	if err := json.Unmarshal([]byte(ai.Unfence(trace.Output)), &payload); err != nil {
		return 0, 0, false
	}
	var confidences []float64
	for _, step := range payload.proposals() {
		confidences = append(confidences, float64(step.Confidence))
	}
	return spanOf(confidences)
}

// disagreements names every next step the scenario expects and the reply
// missed, and every next step the reply proposes and the scenario does not — all
// of them, because a reading that found one of three is not the near miss one
// line would read as.
//
// A proposal answers an expectation when it cites the expected line, whatever
// else it cites alongside: a commitment stated across two lines is the same
// commitment. Only line numbers are named in the detail — the transcript's own
// words are a speaker's, and they have no business being echoed into a record.
func (c *transcriptProposeCase) disagreements(payload transcriptPayload) []string {
	wanted := make(map[string]string, len(c.expected))
	for _, step := range c.expected {
		wanted[fmt.Sprintf("%d:%s", step.Line, step.Promiser)] = step.DueDate
	}
	answered := map[string]bool{}
	var out []string
	for _, step := range payload.proposals() {
		matched := false
		for _, line := range step.SourceLines {
			key := fmt.Sprintf("%d:%s", line, c.promiserKeys[step.PromiserRef])
			if due, exists := wanted[key]; exists && step.DueDate == due {
				answered[key] = true
				matched = true
			}
		}
		if !matched {
			out = append(out, fmt.Sprintf(
				"the reply proposes a next step from %s, which the transcript does not state", citedLines(step)))
		}
	}
	for line := range wanted {
		if !answered[line] {
			out = append(out, fmt.Sprintf(
				"the transcript states a next step for %s and the reply does not propose it", line))
		}
	}
	// Map iteration is unordered, and a detail line that reshuffles between
	// runs reads as a different failure each time.
	sort.Strings(out)
	return out
}

// citedLines renders one proposal's citation as the reader of a record needs to
// see it: the lines to go and look at, in the order the reply gave them.
func citedLines(step proposedStep) string {
	rendered := make([]string, 0, len(step.SourceLines))
	for _, line := range step.SourceLines {
		rendered = append(rendered, strconv.Itoa(line))
	}
	return "line(s) " + strings.Join(rendered, ", ")
}
