// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What the demonstration-draft case owes the certification lane: it refuses a
// scenario that could measure nothing BEFORE a paid run, it reads the reply
// with the production reader, and it tells apart the two ways the profile card
// fails a member.
//
// Those two are not the same event. A reply the reader refuses leaves the card
// BLANK, which is the state the whole step exists to avoid. A reply that reads
// well in nobody's particular voice fills the card with a stranger's writing,
// which is worse: the member looks at it, does not recognise themselves, and
// concludes the build does not work.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// demoStub answers with whatever the scenario under test wants back.
type demoStub struct {
	reply string
	seen  []model.Request
}

func (s *demoStub) Complete(_ context.Context, req model.Request) (model.Response, error) {
	s.seen = append(s.seen, req)
	return model.Response{Text: s.reply}, nil
}

// demoFixture is one built voice over the build floor, carrying the phrase the
// expectation names so the scenario is preparable.
const demoFixture = `{
	"personality": "Blunt. Decides in the first sentence and then says why.",
	"voice_profile_md": "# Voice DNA\n\nWrites like an operator confirming a date.",
	"exemplars": [{"register":"customer_email","kind":"email",
		"text":"We can hold the tolerance, but it adds a grinding pass and about two days."}],
	"stats": {"sample_count": 4, "word_count": 809, "sentence_count": 79, "mean_sentence_words": 10.24, "median_sentence_words": 10, "sentence_word_stddev": 5.44, "em_dash_per_100_words": 0, "question_per_100_words": 0.12, "exclaim_per_100_words": 0, "ellipsis_per_100_words": 0, "line_breaks_per_100_words": 11.12, "register_words": {"customer_email": 438, "internal_chat": 371}, "top_words": ["that", "this", "thing", "what", "will", "before", "have", "need", "tell", "with", "because", "customer"]}
}`

const demoExpectation = `0.6`

func preparedDemoCase(t *testing.T) aitasks.PreparedCase {
	t.Helper()
	prepared, err := voiceDemoDraftCases{}.Prepare(json.RawMessage(demoFixture), json.RawMessage(demoExpectation))
	if err != nil {
		t.Fatalf("preparing the case: %v", err)
	}
	return prepared
}

func TestTheDemoDraftCaseAcceptsADraftThatSitsCloseToTheCorpus(t *testing.T) {
	t.Parallel()
	prepared := preparedDemoCase(t)
	// The corpus averages ten words a sentence. What is measured here is that
	// RHYTHM, not the words themselves — a content token would reward
	// parroting, which is why this site takes eval_draft's floor instead.
	got := prepared.Evaluate(aitasks.Trace{
		Output: `{"subject":"Week three","body":"It runs late by two days and I would rather say so now. ` +
			`The coating sample comes back on Thursday and I will confirm then. Nothing else has moved."}`,
	})
	if got.Result != aitasks.OutcomeAccepted {
		t.Errorf("outcome = %q (%s), want accepted", got.Result, got.Detail)
	}
}

// Readable, competent, and in nobody's rhythm — one long hedging sentence where
// the corpus runs short and plain. This is the outcome that makes a voice build
// worthless to the member reading the card.
func TestTheDemoDraftCaseNamesADraftThatSitsFarFromTheCorpus(t *testing.T) {
	t.Parallel()
	prepared := preparedDemoCase(t)
	got := prepared.Evaluate(aitasks.Trace{
		Output: `{"subject":"Update","body":"I wanted to reach out and provide you with a comprehensive status ` +
			`update regarding the current state of the project and the various workstreams that are presently ` +
			`in flight across the wider team at this point in time."}`,
	})
	if got.Result != aitasks.OutcomeWrongAnswer {
		t.Fatalf("outcome = %q (%s), want wrong_answer", got.Result, got.Detail)
	}
	if !strings.Contains(got.Detail, "expects at least") {
		t.Errorf("detail = %q, want it to name the floor the draft missed", got.Detail)
	}
}

// What production shows for this is a blank card.
func TestTheDemoDraftCaseReportsAnUnreadableReplyAsInvalid(t *testing.T) {
	t.Parallel()
	prepared := preparedDemoCase(t)
	got := prepared.Evaluate(aitasks.Trace{Output: "Sure! Here is a draft:"})
	if got.Result != aitasks.OutcomeInvalid {
		t.Errorf("outcome = %q (%s), want invalid", got.Result, got.Detail)
	}
}

// Run must issue the request the BUILD issues, and exactly once.
func TestTheDemoDraftCaseIssuesTheProductionRequest(t *testing.T) {
	t.Parallel()
	prepared := preparedDemoCase(t)
	stub := &demoStub{reply: `{"subject":"Week three","body":"It runs late. Two days, no more."}`}
	trace, err := prepared.Run(context.Background(), stub)
	if err != nil {
		t.Fatalf("running the case: %v", err)
	}
	if len(stub.seen) != 1 {
		t.Fatalf("the case made %d calls, want exactly one", len(stub.seen))
	}
	// The demonstration carries no held-out sample: only the profile is inside
	// the boundary, which is what separates this prompt from eval_draft's.
	if !strings.Contains(stub.seen[0].System, "profile") {
		t.Error("the system prompt does not name the boundary this site declares")
	}
	if trace.Output != stub.reply {
		t.Error("the case edited the reply before evaluating it")
	}
}

func TestTheDemoDraftCaseRefusesScenariosThatMeasureNothing(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, fixture, expected, want string
	}{
		{
			name:     "no built profile leaves no voice to demonstrate",
			fixture:  `{"personality":"p","voice_profile_md":"  ","exemplars":[{"text":"grinding pass"}],"stats":{"word_count":809}}`,
			expected: demoExpectation,
			want:     "no built profile",
		},
		{
			name:     "no example leaves no voice to copy",
			fixture:  `{"personality":"p","voice_profile_md":"# V","exemplars":[],"stats":{"word_count":809}}`,
			expected: demoExpectation,
			want:     "no verbatim example",
		},
		{
			name:     "a blank example shows the model nothing of this voice",
			fixture:  `{"personality":"p","voice_profile_md":"# V","exemplars":[{"text":"   "}],"stats":{"word_count":809}}`,
			expected: demoExpectation,
			want:     "every verbatim example the fixture supplies is blank",
		},
		{
			name:     "a corpus under the build floor was never buildable",
			fixture:  `{"personality":"p","voice_profile_md":"# V","exemplars":[{"text":"grinding pass"}],"stats":{"word_count":10}}`,
			expected: demoExpectation,
			want:     "own-authored words",
		},
		{
			name:     "a floor every draft clears asserts nothing",
			fixture:  demoFixture,
			expected: `0`,
			want:     "which every draft clears",
		},
		{
			name:     "a floor above 1 can never be met",
			fixture:  demoFixture,
			expected: `1.5`,
			want:     "at most 1",
		},
		{
			name:     "a fixture of the wrong shape",
			fixture:  `["not an object"]`,
			expected: demoExpectation,
			want:     "not the shape this site takes",
		},
		{
			name:     "an expectation of the wrong shape",
			fixture:  demoFixture,
			expected: `"just a string"`,
			want:     "not a stylometric floor",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := voiceDemoDraftCases{}.Prepare(json.RawMessage(tc.fixture), json.RawMessage(tc.expected))
			if err == nil {
				t.Fatalf("the case accepted a scenario that measures nothing (%s)", tc.name)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want it to name %q", err, tc.want)
			}
		})
	}
}

// The build floor the guard enforces is the builder's own, not a number copied
// beside it: a copy would stop agreeing the first time the floor moved.
func TestTheDemoDraftGuardUsesTheBuildersOwnFloor(t *testing.T) {
	t.Parallel()
	under := strings.Replace(demoFixture, `"word_count": 809`, `"word_count": 1`, 1)
	_, err := voiceDemoDraftCases{}.Prepare(json.RawMessage(under), json.RawMessage(demoExpectation))
	if err == nil {
		t.Fatal("a corpus under the build floor was accepted")
	}
	if !strings.Contains(err.Error(), "at least 800") {
		t.Errorf("error = %q, want it to quote ai.StarterVoiceWords (%d)", err, ai.StarterVoiceWords)
	}
	if got := (voiceDemoDraftCases{}).Site().Variant; got != "demo_draft" {
		t.Errorf("variant = %q, want demo_draft", got)
	}
}
