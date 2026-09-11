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
	"stats": {"word_count": 809}
}`

const demoExpectation = `{"names_token":"grinding pass"}`

func preparedDemoCase(t *testing.T) aitasks.PreparedCase {
	t.Helper()
	prepared, err := voiceDemoDraftCases{}.Prepare(json.RawMessage(demoFixture), json.RawMessage(demoExpectation))
	if err != nil {
		t.Fatalf("preparing the case: %v", err)
	}
	return prepared
}

func TestTheDemoDraftCaseAcceptsADraftInTheBuiltVoice(t *testing.T) {
	t.Parallel()
	prepared := preparedDemoCase(t)
	got := prepared.Evaluate(aitasks.Trace{
		Output: `{"subject":"Week three","body":"It adds a grinding pass. Two days, no more."}`,
	})
	if got.Result != aitasks.OutcomeAccepted {
		t.Errorf("outcome = %q (%s), want accepted", got.Result, got.Detail)
	}
}

// Readable, competent, and in nobody's voice — the outcome that makes a voice
// build worthless to the member reading the card.
func TestTheDemoDraftCaseNamesADraftInNobodysVoice(t *testing.T) {
	t.Parallel()
	prepared := preparedDemoCase(t)
	got := prepared.Evaluate(aitasks.Trace{
		Output: `{"subject":"Update","body":"I wanted to reach out with a quick status update."}`,
	})
	if got.Result != aitasks.OutcomeWrongAnswer {
		t.Fatalf("outcome = %q (%s), want wrong_answer", got.Result, got.Detail)
	}
	if !strings.Contains(got.Detail, "grinding pass") {
		t.Errorf("detail = %q, want it to name the phrase the draft never wrote", got.Detail)
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
	stub := &demoStub{reply: `{"subject":"Week three","body":"It adds a grinding pass."}`}
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
			name:     "a corpus under the build floor was never buildable",
			fixture:  `{"personality":"p","voice_profile_md":"# V","exemplars":[{"text":"grinding pass"}],"stats":{"word_count":10}}`,
			expected: demoExpectation,
			want:     "own-authored words",
		},
		{
			name:     "no phrase admits a draft in anybody's voice",
			fixture:  demoFixture,
			expected: `{"names_token":"   "}`,
			want:     "names no phrase of this voice",
		},
		{
			name:     "a phrase this voice never produced could only be invented",
			fixture:  demoFixture,
			expected: `{"names_token":"synergistic alignment"}`,
			want:     "appears in neither the profile nor its examples",
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
			want:     "not this site's shape",
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
