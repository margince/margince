// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What a scenario owes this site before a paid run: an expectation a run of the
// agent could meet, tool answers a call could reach, and a fixture shaped like
// what the scheduler, retrieval and the tools hand a run. Each refusal names
// what is wrong, because finding it later costs a run.

import (
	"encoding/json"
	"strings"
	"testing"
)

// agentLoopRefusal is one scenario Prepare owes a refusal, and the words the
// refusal owes its author.
type agentLoopRefusal struct {
	name     string
	site     string
	fixture  json.RawMessage
	expected json.RawMessage
	want     string
}

func runAgentLoopRefusals(t *testing.T, cases []agentLoopRefusal) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			site := tc.site
			if site == "" {
				site = agentLoopAgent
			}
			_, err := agentLoopCases{agent: site}.Prepare(tc.fixture, tc.expected)
			if err == nil {
				t.Fatal("Prepare accepted a scenario this site cannot measure")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("the refusal reads %q, want it to name %q", err, tc.want)
			}
		})
	}
}

func TestAgentLoopCaseRefusesAnEndNoRunCouldReach(t *testing.T) {
	base := fixtureJSON(t, agentLoopBaseFixture(t))
	runAgentLoopRefusals(t, []agentLoopRefusal{
		{name: "an expectation that is not a step name", fixture: base, expected: json.RawMessage(`{"step":"final"}`), want: "not a step name"},
		{
			name: "the list read as the run's end", fixture: base, expected: agentLoopExpectationJSON(t, agentLoopReadTool),
			want: "ends either by writing with annotate_brief or as \"final\"",
		},
		{name: "a tool this agent is never offered", fixture: base, expected: agentLoopExpectationJSON(t, "send_email"), want: `expects "send_email"`},
	})
}

// agentLoopFixtureWith is the base fixture with its tool answers replaced.
func agentLoopFixtureWith(t *testing.T, edit func(*agentLoopFixture)) json.RawMessage {
	t.Helper()
	f := agentLoopBaseFixture(t)
	edit(&f)
	return fixtureJSON(t, f)
}

func TestAgentLoopCaseRefusesToolAnswersNoCallCouldReach(t *testing.T) {
	final := agentLoopExpectationJSON(t, agentLoopFinalStep)
	runAgentLoopRefusals(t, []agentLoopRefusal{
		{
			name: "an answer for a tool the agent is not offered", expected: final, want: `answers "send_email"`,
			fixture: agentLoopFixtureWith(t, func(f *agentLoopFixture) {
				f.Tools = append(f.Tools, agentLoopToolAnswer{Tool: "send_email", Result: json.RawMessage(`{}`)})
			}),
		},
		{
			name: "an answer pinning an argument its tool does not take", expected: final, want: `pins "owner"`,
			fixture: agentLoopFixtureWith(t, func(f *agentLoopFixture) {
				f.Tools[1].Args = map[string]json.RawMessage{"owner": json.RawMessage(`"me"`)}
			}),
		},
		{
			name: "no answer for the list the goal reads first", expected: final, want: "answers read_brief 0 times",
			fixture: agentLoopFixtureWith(t, func(f *agentLoopFixture) { f.Tools = f.Tools[1:] }),
		},
		{
			name: "two answers for the list", expected: final, want: "answers read_brief 2 times",
			fixture: agentLoopFixtureWith(t, func(f *agentLoopFixture) { f.Tools = append(f.Tools, f.Tools[0]) }),
		},
		{
			name: "a list answer the tool could not have served", expected: final, want: "not the brief the tool serves",
			fixture: agentLoopFixtureWith(t, func(f *agentLoopFixture) {
				f.Tools[0].Result = json.RawMessage(`{"items":[],"verdict":"all good"}`)
			}),
		},
	})
}

// A window the scheduler and retrieval would never have produced for this site
// is a prompt the product never sends, so whatever a model says to it measures
// nothing.
func TestAgentLoopCaseRefusesAWindowTheLoopIsNeverHanded(t *testing.T) {
	withRef := func(ref string) json.RawMessage {
		return agentLoopFixtureWith(t, func(f *agentLoopFixture) { f.TriggerRef = ref })
	}
	final := agentLoopExpectationJSON(t, agentLoopFinalStep)
	runAgentLoopRefusals(t, []agentLoopRefusal{
		{name: "a job with no trigger occurrence", fixture: withRef(""), expected: final, want: "names no trigger"},
		{
			name: "a trigger shape production stopped minting", fixture: withRef("morning_brief:2026-07-27"), expected: final,
			want: "has 2 segment(s); the scheduler mints 3",
		},
		{name: "a seat digest of the wrong width", fixture: withRef("morning_brief:2026-07-27:d48b383f"), expected: final, want: "segment 3 is"},
		{
			name: "another agent's occurrence", fixture: withRef("overnight_at_risk_sweep:2026-07-27:d48b383f3e8acec5d620c82b8c9b4202"),
			expected: final, want: `names "overnight_at_risk_sweep"`,
		},
		{
			name: "a site no scheduled agent carries", site: "ad_hoc_question",
			fixture: withRef("ad_hoc_question:2026-07-27:d48b383f3e8acec5d620c82b8c9b4202"), expected: final,
			want: "no scheduled agent carries this site's name",
		},
		{
			name: "a fixture supplying its own goal", fixture: agentLoopRawFixture(t, "goal", `"Find the Acme account."`),
			expected: final, want: `unknown field "goal"`,
		},
		{
			name: "a fixture naming a tool surface instead of answers", fixture: agentLoopRawFixture(t, "tools", `"catalog"`),
			expected: final, want: "not the shape this site takes",
		},
		{
			name: "a seed item choosing its own trust tier", expected: final, want: `unknown field "trust_tier"`,
			fixture: agentLoopRawFixture(t, "grounding", `[{"source_id":"user:0198f3a1-7c42-7e0b-9d51-2a6f4b8c1e0a","trust_tier":"T1","content":"x"}]`),
		},
	})
}

// agentLoopRawFixture is the base fixture with one top-level key set to value,
// so a test can hand this site a shape the Go type cannot hold.
func agentLoopRawFixture(t *testing.T, key, value string) json.RawMessage {
	t.Helper()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(fixtureJSON(t, agentLoopBaseFixture(t)), &fields); err != nil {
		t.Fatalf("decoding the fixture: %v", err)
	}
	fields[key] = json.RawMessage(value)
	return fixtureJSON(t, fields)
}

// A fixture answer pins only what tells two calls apart, as a subset, with
// numbers compared by what they mean: float64 would make two integers past
// 2^53 one answer, and comparing literals would make 5 and 5.0 two.
func TestAToolAnswerMatchesTheCallsItsPinnedArgumentsName(t *testing.T) {
	pinned := map[string]json.RawMessage{"record_id": json.RawMessage(`"d1"`), "max_items": json.RawMessage(`5`)}
	for _, tc := range []struct {
		name  string
		call  string
		match bool
	}{
		{"the pinned arguments and more", `{"record_type":"deal","record_id":"d1","max_items":5}`, true},
		{"the same number written two ways", `{"record_id":"d1","max_items":5.0}`, true},
		{"another record", `{"record_id":"d2","max_items":5}`, false},
		{"a pinned argument left out", `{"max_items":5}`, false},
		{"integers past float64's reach", `{"record_id":"d1","max_items":9007199254740993}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := len(agentLoopArgDisagreements(pinned, json.RawMessage(tc.call))) == 0; got != tc.match {
				t.Errorf("a call %s matched = %v, want %v", tc.call, got, tc.match)
			}
		})
	}
	if far := map[string]json.RawMessage{"n": json.RawMessage(`9007199254740993`)}; len(agentLoopArgDisagreements(far, json.RawMessage(`{"n":9007199254740992}`))) == 0 {
		t.Error("two integers that round to one float64 matched")
	}
}
