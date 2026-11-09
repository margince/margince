// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/model"
	"github.com/margince/margince/backend/internal/shared/schema"
)

// dealTopicStub answers each requested deal id, read from the request's own
// schema, with the verdict at its position.
type dealTopicStub struct {
	answer func(ids []string) string
}

func (s dealTopicStub) Complete(_ context.Context, req model.Request) (model.Response, error) {
	var shape struct {
		Properties struct {
			Results struct {
				Items struct {
					Properties struct {
						DealID struct {
							Enum []string `json:"enum"`
						} `json:"deal_id"`
					} `json:"properties"`
				} `json:"items"`
			} `json:"results"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(req.ResponseSchema, &shape); err != nil {
		return model.Response{}, err
	}
	return model.Response{Text: s.answer(shape.Properties.Results.Items.Properties.DealID.Enum)}, nil
}

func verdictsReply(confidence float64, verdicts ...string) func([]string) string {
	return func(ids []string) string {
		parts := make([]string, len(ids))
		for i, id := range ids {
			parts[i] = fmt.Sprintf(`{"deal_id":%q,"verdict":%q,"confidence":%v}`, id, verdicts[i], confidence)
		}
		return `{"results":[` + strings.Join(parts, ",") + `]}`
	}
}

const twoDealFixture = `{"subject":"Re: cut-over","body":"the revised schedule","deals":[
	{"name":"Rollout","project":"Warehouse","recent":["Kickoff"]},{"name":"Support renewal","project":"","recent":[]}]}`

func evaluateDealTopic(t *testing.T, fixture, expected string, stub dealTopicStub) aitasks.Outcome {
	t.Helper()
	prepared, err := dealTopicCases{}.Prepare(json.RawMessage(fixture), json.RawMessage(expected))
	if err != nil {
		t.Fatalf("preparing the case: %v", err)
	}
	trace, err := prepared.Run(context.Background(), stub)
	if err != nil {
		t.Fatalf("running the case: %v", err)
	}
	return prepared.Evaluate(trace)
}

func TestTheDealTopicCaseGradesEachDealByItsVerdict(t *testing.T) {
	for _, c := range []struct {
		name  string
		reply func([]string) string
		want  string
	}{
		{"the expected verdicts", verdictsReply(0.9, "about", "not_about"), aitasks.OutcomeAccepted},
		{"a well-formed wrong verdict", verdictsReply(0.9, "about", "about"), aitasks.OutcomeWrongAnswer},
		{"a verdict outside the closed set", verdictsReply(0.9, "about", "maybe"), aitasks.OutcomeInvalid},
		{"a confidence outside [0,1]", verdictsReply(1.5, "about", "not_about"), aitasks.OutcomeInvalid},
		{"a deal left unanswered", func(ids []string) string { return verdictsReply(0.9, "about")(ids[:1]) }, aitasks.OutcomeInvalid},
	} {
		got := evaluateDealTopic(t, twoDealFixture, `["about","not_about"]`, dealTopicStub{answer: c.reply})
		if got.Result != c.want {
			t.Errorf("%s reports %q (%s), want %q", c.name, got.Result, got.Detail, c.want)
		}
	}
}

func TestTheDealTopicCaseRefusesWhatNoReplyCanReach(t *testing.T) {
	for name, c := range map[string][2]string{
		"no deal":           {`{"subject":"s","body":"b","deals":[]}`, `[]`},
		"a verdict too few": {twoDealFixture, `["about"]`},
		"an unknown answer": {twoDealFixture, `["about","maybe"]`},
	} {
		if _, err := (dealTopicCases{}).Prepare(json.RawMessage(c[0]), json.RawMessage(c[1])); err == nil {
			t.Errorf("%s: Prepare accepted a scenario no reply can satisfy", name)
		}
	}
}

// The decision on an answer: one confident "about" files that deal only when
// every other deal is confidently "not_about". Every deal confidently
// "not_about" rules them all out, and anything else is doubt.
func TestADealTopicAnswerFilesOnlyOnConfidence(t *testing.T) {
	const first, second = "01a1bbf5-b1d3-70a8-88a8-40db094f51de", "01a1bbf5-b1db-7238-ba38-cb859d5ed13c"
	result := func(id, verdict string, confidence float64) dealTopicResult {
		return dealTopicResult{DealID: id, Verdict: verdict, Confidence: schema.Confidence(confidence)}
	}
	for _, c := range []struct {
		name    string
		results []dealTopicResult
		deal    string
		sure    bool
	}{
		{"one confident about, the other ruled out", []dealTopicResult{result(first, "about", 0.9), result(second, "not_about", 0.9)}, first, true},
		{"one confident about, the other ruled out unsurely", []dealTopicResult{result(first, "about", 0.9), result(second, "not_about", 0.6)}, "", false},
		{"one confident about, the other unsure", []dealTopicResult{result(first, "about", 0.9), result(second, "unsure", 0.9)}, "", false},
		{"about below the floor", []dealTopicResult{result(first, "about", 0.84)}, "", false},
		{"two confident abouts", []dealTopicResult{result(first, "about", 0.9), result(second, "about", 0.95)}, "", false},
		{"every deal ruled out", []dealTopicResult{result(first, "not_about", 0.9), result(second, "not_about", 0.86)}, "", true},
		{"one ruled out unsurely", []dealTopicResult{result(first, "not_about", 0.9), result(second, "not_about", 0.5)}, "", false},
	} {
		got, err := dealTopicOf(c.results)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		want := capture.DealTopic{Confident: c.sure}
		if c.deal != "" {
			deal, err := ids.Parse(c.deal)
			if err != nil {
				t.Fatal(err)
			}
			want.Deal = deal
		}
		if got != want {
			t.Errorf("%s: the answer reads %+v, want %+v", c.name, got, want)
		}
	}
}

// aboutEveryDeal is a topic model sure each deal it is asked about fits, which
// is how capture filed a suggested deal before the check existed.
var aboutEveryDeal = dealTopicStub{answer: func(ids []string) string {
	verdicts := make([]string, len(ids))
	for i := range verdicts {
		verdicts[i] = dealTopicAbout
	}
	return verdictsReply(0.95, verdicts...)(ids)
}}
