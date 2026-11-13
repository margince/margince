// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatsummary

// The episode site's certification case. A scenario names the record and the
// messages, whether the talk matters, and the facts a relevant note must keep.
// Run and Evaluate call the production Request, Parse and Check.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

const certSite = "chat_summary/episode"

type certMessage struct {
	Author string    `json:"author"`
	At     time.Time `json:"at"`
	Text   string    `json:"text"`
}

type certFixture struct {
	Record   string        `json:"record"`
	Facts    []string      `json:"facts"`
	Channel  string        `json:"channel"`
	Messages []certMessage `json:"messages"`
}

// certExpectation is whether the talk matters and, when it does, the spans
// the note must carry and the ones it must not.
type certExpectation struct {
	Relevant bool     `json:"relevant"`
	Keeps    []string `json:"keeps"`
	Omits    []string `json:"omits"`
}

// CertCases serves the chat_summary/episode site to the certification lane.
type CertCases struct{}

// Site names the chat_summary/episode site this case certifies.
func (CertCases) Site() aitasks.Site {
	return aitasks.Site{Task: ai.TaskChatSummary, Variant: "episode", Kind: ai.SiteKindOneShot}
}

// Prepare refuses a scenario no answer could pass: no messages, or a span to
// keep that no message says.
//
//nolint:ireturn // PreparedCase IS the seam: one implementation per site behind the one interface the cert lane runs.
func (CertCases) Prepare(fixture, expected json.RawMessage) (aitasks.PreparedCase, error) {
	var f certFixture
	if err := json.Unmarshal(fixture, &f); err != nil {
		return nil, fmt.Errorf("%s: the fixture is not the shape this site takes: %w", certSite, err)
	}
	if len(f.Messages) == 0 {
		return nil, fmt.Errorf("%s: the fixture has no messages; an episode always has one", certSite)
	}
	var want certExpectation
	if err := json.Unmarshal(expected, &want); err != nil {
		return nil, fmt.Errorf("%s: the expected value is not {relevant, keeps, omits}: %w", certSite, err)
	}
	in := Input{Record: f.Record, Facts: f.Facts, Channel: f.Channel}
	var said strings.Builder
	for _, m := range f.Messages {
		in.Messages = append(in.Messages, Message(m))
		said.WriteString(m.Text + "\n")
	}
	for _, span := range want.Keeps {
		if !strings.Contains(said.String(), span) {
			return nil, fmt.Errorf("%s: the expected span %q is in no message, so no note can keep it", certSite, span)
		}
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

// Evaluate reads the reply through the production parser and validator, then
// asks whether it judged the talk right and kept what the scenario names.
func (c *certCase) Evaluate(trace aitasks.Trace) aitasks.Outcome {
	reply, err := Parse(trace.Output)
	if err == nil {
		err = Check(reply)
	}
	if err != nil {
		return aitasks.Outcome{Result: aitasks.OutcomeInvalid, Detail: err.Error()}
	}
	if reply.Relevant != c.expected.Relevant {
		return aitasks.Outcome{Result: aitasks.OutcomeWrongAnswer,
			Detail: fmt.Sprintf("relevant = %t, want %t", reply.Relevant, c.expected.Relevant)}
	}
	note := strings.Join(append(append([]string{reply.Title, reply.Summary}, reply.Decisions...), reply.OpenQuestions...), "\n")
	for _, span := range c.expected.Keeps {
		if !strings.Contains(note, span) {
			return aitasks.Outcome{Result: aitasks.OutcomeWrongAnswer, Detail: fmt.Sprintf("the note lost %q", span)}
		}
	}
	lowered := strings.ToLower(note)
	for _, span := range c.expected.Omits {
		if strings.Contains(lowered, strings.ToLower(span)) {
			return aitasks.Outcome{Result: aitasks.OutcomeWrongAnswer, Detail: fmt.Sprintf("the note carries %q", span)}
		}
	}
	return aitasks.Outcome{Result: aitasks.OutcomeAccepted}
}
