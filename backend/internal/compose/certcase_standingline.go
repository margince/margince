// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The certification case for summarize/standing_line. It sends the request
// standingline.LineRequest builds and reads the reply with
// standingline.ParseLine, the two the background check calls.
//
// It measures what the grounding filter cannot: that the line says what the
// newest activity said, in a word only that activity carries.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/compose/standingline"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

type standingLineCases struct{}

func (standingLineCases) Site() aitasks.Site {
	return aitasks.Site{Task: ai.TaskSummarize, Variant: "standing_line", Kind: ai.SiteKindOneShot}
}

// standingLineFixture is one record's newest activities, newest first.
type standingLineFixture struct {
	RecordKind string                 `json:"record_kind"`
	Previous   string                 `json:"previous_line"`
	Activities []standingLineActivity `json:"activities"`
}

type standingLineActivity struct {
	Kind    string `json:"kind"`
	DaysAgo int    `json:"days_ago"`
	Author  string `json:"author"`
	Side    string `json:"side"`
	Subject string `json:"subject"`
	Text    string `json:"text"`
}

type standingLineExpectation struct {
	Keep       bool     `json:"keep"`
	NamesToken string   `json:"names_token"`
	Avoids     []string `json:"avoids"`
}

// standingLineToday anchors the fixtures' dates, so a score does not move
// with the calendar.
var standingLineToday = time.Date(2026, time.September, 3, 9, 0, 0, 0, time.UTC)

// Prepare builds the input the check assembles, with minted ids.
//
//nolint:ireturn // PreparedCase IS the seam: one implementation per site behind the one interface the cert lane runs.
func (standingLineCases) Prepare(fixture, expected json.RawMessage) (aitasks.PreparedCase, error) {
	var f standingLineFixture
	if err := json.Unmarshal(fixture, &f); err != nil {
		return nil, fmt.Errorf("summarize/standing_line: the fixture is not the shape this site takes: %w", err)
	}
	var want standingLineExpectation
	if err := json.Unmarshal(expected, &want); err != nil {
		return nil, fmt.Errorf("summarize/standing_line: the expected answer is not this site's shape: %w", err)
	}
	if !want.Keep && want.NamesToken == "" {
		return nil, errors.New("summarize/standing_line: a scenario must keep the line or name a token only the newest activity says")
	}
	in := standingline.Input{Kind: f.RecordKind, Previous: f.Previous}
	for _, act := range f.Activities {
		in.Recent = append(in.Recent, standingline.Act{
			ID: ids.NewV7().String(), Kind: act.Kind, Author: act.Author, Side: act.Side,
			At:      standingLineToday.AddDate(0, 0, -act.DaysAgo).Format(time.DateOnly),
			Subject: act.Subject, Text: act.Text,
		})
	}
	return &standingLineCase{in: in, want: want}, nil
}

type standingLineCase struct {
	in   standingline.Input
	want standingLineExpectation
}

// Run sends the production request, in English so scores compare across installations.
func (c *standingLineCase) Run(ctx context.Context, completer aitasks.Completer) (aitasks.Trace, error) {
	req := standingline.LineRequest(c.in, string(textlang.English))
	trace := aitasks.Trace{Requests: []model.Request{req}}
	resp, err := completer.Complete(ctx, req)
	if err != nil {
		return trace, fmt.Errorf("summarize/standing_line: %w", err)
	}
	trace.Output = resp.Text
	return trace, nil
}

// Evaluate runs the production filter, then asks what production cannot.
func (c *standingLineCase) Evaluate(trace aitasks.Trace) aitasks.Outcome {
	answer, err := standingline.ParseLine(trace.Output, c.in)
	if err != nil {
		return aitasks.Outcome{Result: aitasks.OutcomeInvalid, Detail: fmt.Sprintf("the grounding filter refused the reply: %v", err)}
	}
	if c.want.Keep {
		if answer.Keep {
			return aitasks.Outcome{Result: aitasks.OutcomeAccepted}
		}
		return aitasks.Outcome{Result: aitasks.OutcomeWrongAnswer, Detail: "the line still held and was rewritten"}
	}
	if answer.Text == "" {
		return aitasks.Outcome{Result: aitasks.OutcomeAbstained, Detail: "no line cited the record's activities"}
	}
	folded := strings.ToLower(answer.Text)
	var wrong []string
	if !strings.Contains(folded, strings.ToLower(c.want.NamesToken)) {
		wrong = append(wrong, fmt.Sprintf("the line never names %q, so it does not say what was said", c.want.NamesToken))
	}
	for _, avoided := range c.want.Avoids {
		if strings.Contains(folded, strings.ToLower(avoided)) {
			wrong = append(wrong, fmt.Sprintf("the line says %q", avoided))
		}
	}
	if len(wrong) > 0 {
		return aitasks.Outcome{Result: aitasks.OutcomeWrongAnswer, Detail: strings.Join(wrong, "; ")}
	}
	return aitasks.Outcome{Result: aitasks.OutcomeAccepted}
}
