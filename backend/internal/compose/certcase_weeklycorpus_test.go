// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose_test

// The weekly scenarios graded against canned replies: each must accept the
// forms a right answer takes and refuse the wrong answer it exists to catch.
// What is under test is the scenario's discrimination, not a model.

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/aicert"
	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/modules/ai"
)

// preparedCorpusCase loads one committed scenario and prepares it through the
// case its site binds, exactly as a certification run does.
func preparedCorpusCase(t *testing.T, path string) aitasks.PreparedCase {
	t.Helper()
	census, err := compose.NewTaskCensus()
	if err != nil {
		t.Fatalf("building the task census: %v", err)
	}
	sc, err := aicert.LoadScenarioFile(path, census)
	if err != nil {
		t.Fatalf("loading %s: %v", path, err)
	}
	factory, ok := census.CaseFor(ai.Task(sc.Task), sc.Site)
	if !ok {
		t.Fatalf("%s names %s/%s, which binds no case", path, sc.Task, sc.Site)
	}
	prepared, err := factory.Prepare(json.RawMessage(sc.Fixture), json.RawMessage(sc.Expect.Answer))
	if err != nil {
		t.Fatalf("preparing %s: %v", path, err)
	}
	return prepared
}

func narrativeReply(t *testing.T, sentence string) string {
	t.Helper()
	encoded, err := json.Marshal(map[string]string{"narrative": sentence})
	if err != nil {
		t.Fatalf("encoding the canned reply: %v", err)
	}
	return string(encoded)
}

func TestTheWonDealScenarioAcceptsEveryFormOfTheWinAndNoWrongWeek(t *testing.T) {
	prepared := preparedCorpusCase(t, "aicert/corpus/weekly_review/a_week_with_one_thing_worth_saying.yaml")
	for _, tc := range []struct {
		sentence   string
		want       string
		wantDetail string
	}{
		{"Weber Rahmenvertrag was won, and Stahlbau Krämer moved to Angebot.", aitasks.OutcomeAccepted, ""},
		{"Winning Weber Rahmenvertrag led the week; one of six tasks is still open.", aitasks.OutcomeAccepted, ""},
		{"Closing Weber Rahmenvertrag led the week, and Stahlbau Krämer moved to Angebot.", aitasks.OutcomeAccepted, ""},
		// The name alone is not the headline: a sentence that never says the
		// deal was won has not told the reader what changed.
		{"Weber Rahmenvertrag and Stahlbau Krämer both moved this week.", aitasks.OutcomeWrongAnswer, "won or win"},
		{"One deal was won and Stahlbau Krämer moved to Angebot.", aitasks.OutcomeWrongAnswer, "Weber"},
		{"Weber Rahmenvertrag was lost, though you won back time on five tasks.", aitasks.OutcomeWrongAnswer, "lost"},
	} {
		got := prepared.Evaluate(aitasks.Trace{Output: narrativeReply(t, tc.sentence)})
		if got.Result != tc.want || !strings.Contains(got.Detail, tc.wantDetail) {
			t.Errorf("%q: got %s (%s), want %s naming %q", tc.sentence, got.Result, got.Detail, tc.want, tc.wantDetail)
		}
	}
}

func TestTheLossPatternScenarioWantsACitedLearningAndRefusesSilence(t *testing.T) {
	prepared := preparedCorpusCase(t, "aicert/corpus/weekly_learnings/a_repeated_pattern_is_cited.yaml")
	const aurora, borealis = "01a05500-0000-7000-8000-0000000000e1", "01a05500-0000-7000-8000-0000000000e2"
	learning := func(ids ...string) string {
		citations := make([]string, 0, len(ids))
		for _, id := range ids {
			citations = append(citations, `{"type":"deal","id":"`+id+`"}`)
		}
		return `{"learnings":[{"kind":"pattern","text":"You lost all three deals that closed out this week.","citations":[` +
			strings.Join(citations, ",") + `]}]}`
	}
	for _, tc := range []struct {
		name, reply, want string
	}{
		{"cites the rows", learning(aurora, borealis), aitasks.OutcomeAccepted},
		{"a lane that refuses every week", `{"learnings":[]}`, aitasks.OutcomeAbstained},
		{"rests on a sibling, not the named row", learning(borealis), aitasks.OutcomeWrongAnswer},
	} {
		if got := prepared.Evaluate(aitasks.Trace{Output: tc.reply}); got.Result != tc.want {
			t.Errorf("%s: got %s (%s), want %s", tc.name, got.Result, got.Detail, tc.want)
		}
	}
}
