// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What the meeting-brief case owes the certification lane: it refuses a
// scenario that could not measure anything BEFORE a paid run, it reads the
// reply with the service's own parser, and it separates the three things a
// reply can be.
//
// The separation carries the meaning of a run. ParseBriefSections DROPS what it
// cannot ground rather than refusing the reply, so a brief about the wrong
// conversation and a brief the parser rejected both arrive with sections
// missing — and they are opposite events. The first is a model that wrote about
// something else; the second is a model that produced something unusable.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/compose/meetingbrief"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/promptfence"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// briefStub answers with whatever the scenario under test wants back.
type briefStub struct {
	reply string
	seen  []model.Request
}

func (s *briefStub) Complete(_ context.Context, req model.Request) (model.Response, error) {
	s.seen = append(s.seen, req)
	return model.Response{Text: s.reply}, nil
}

// briefCaseFor builds the case over a meeting whose own activity id is the
// record a correct brief cites. The meeting is always citable (knownRecords
// seeds it), so the test needs no id smuggled out of the prompt.
func briefCaseFor(token string) (*meetingBriefCase, string) {
	meeting := ids.NewV7().String()
	now := time.Date(2026, time.August, 4, 12, 0, 0, 0, time.UTC)
	return &meetingBriefCase{
		in: meetingbrief.Input{
			ActivityID: meeting,
			Subject:    "Coffee with Rainer",
			Company:    "Asia Flight Services",
			StartsAt:   now.Add(24 * time.Hour),
			Now:        now,
		},
		mustCite:  meeting,
		mustName:  token,
		citeLabel: "wish_list",
	}, meeting
}

// sectionsReply renders one grounded sentence in the shape the site parses.
func sectionsReply(text, activityID string) string {
	return `{"sections":[{"kind":"talking_points","sentences":[{"text":"` + text +
		`","nature":"fact","evidence":[{"entity_type":"activity","entity_id":"` + activityID + `"}]}]}]}`
}

func TestTheMeetingBriefCaseAcceptsAGroundedSpecificBrief(t *testing.T) {
	t.Parallel()
	c, meeting := briefCaseFor("quote tracking")
	got := c.Evaluate(aitasks.Trace{Output: sectionsReply("They asked for quote tracking.", meeting)})
	if got.Result != aitasks.OutcomeAccepted {
		t.Errorf("outcome = %q (%s), want accepted", got.Result, got.Detail)
	}
}

// Grounded in the right meeting and still not preparation: a brief that never
// names what this account asked for would read the same about any of them.
func TestTheMeetingBriefCaseNamesAGenericBrief(t *testing.T) {
	t.Parallel()
	c, meeting := briefCaseFor("quote tracking")
	got := c.Evaluate(aitasks.Trace{Output: sectionsReply("They have priorities this quarter.", meeting)})
	if got.Result != aitasks.OutcomeWrongAnswer {
		t.Fatalf("outcome = %q (%s), want wrong_answer", got.Result, got.Detail)
	}
	if !strings.Contains(got.Detail, "quote tracking") {
		t.Errorf("detail = %q, want it to name the phrase the brief never wrote", got.Detail)
	}
}

// Every sentence cited a record of some other meeting, so the filter dropped
// them all. Production shows the deterministic floor for this, not prose.
func TestTheMeetingBriefCaseReportsABriefAboutAnotherMeetingAsAbstained(t *testing.T) {
	t.Parallel()
	c, _ := briefCaseFor("quote tracking")
	elsewhere := ids.NewV7().String()
	got := c.Evaluate(aitasks.Trace{Output: sectionsReply("They asked for quote tracking.", elsewhere)})
	if got.Result != aitasks.OutcomeAbstained {
		t.Errorf("outcome = %q (%s), want abstained", got.Result, got.Detail)
	}
}

func TestTheMeetingBriefCaseReportsAnUnreadableReplyAsInvalid(t *testing.T) {
	t.Parallel()
	c, _ := briefCaseFor("quote tracking")
	got := c.Evaluate(aitasks.Trace{Output: "I cannot help with that."})
	if got.Result != aitasks.OutcomeInvalid {
		t.Errorf("outcome = %q (%s), want invalid", got.Result, got.Detail)
	}
}

// Run must issue the request PRODUCTION issues, not a copy: the system prompt
// is the site's own, and the reply is handed back untouched.
func TestTheMeetingBriefCaseIssuesTheProductionRequest(t *testing.T) {
	t.Parallel()
	c, meeting := briefCaseFor("quote tracking")
	stub := &briefStub{reply: sectionsReply("They asked for quote tracking.", meeting)}
	trace, err := c.Run(context.Background(), stub)
	if err != nil {
		t.Fatalf("running the case: %v", err)
	}
	if len(stub.seen) != 1 {
		t.Fatalf("the case made %d calls, want exactly one", len(stub.seen))
	}
	// Compared through the fence's own canonicalisation: every call mints a
	// fresh boundary nonce, so two renderings of ONE prompt differ by exactly
	// that marker and by nothing else. Comparing the raw strings would fail on
	// every correct request.
	want := meetingbrief.BriefRequest(c.in, "en")
	sent := promptfence.Canonicalize(stub.seen[0].System, stub.seen[0].System)
	if sent != promptfence.Canonicalize(want.System, want.System) {
		t.Error("the case sent a system prompt the site does not send")
	}
	if trace.Output != stub.reply {
		t.Error("the case edited the reply before evaluating it")
	}
}

// The guards exist so a scenario that could measure nothing is refused at parse
// time rather than after a paid run.
func TestTheMeetingBriefCaseRefusesScenariosThatMeasureNothing(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, fixture, expected, want string
	}{
		{
			name:     "one conversation leaves no wrong one to cite",
			fixture:  `{"subject":"s","company":"c","attendee":"a","messages":[{"label":"only","subject":"x","body":"quote tracking"}]}`,
			expected: `{"cites_label":"only","names_token":"quote tracking"}`,
			want:     "fewer than two",
		},
		{
			name:     "no token admits a brief about anybody",
			fixture:  planFixture,
			expected: `{"cites_label":"wish_list","names_token":"  "}`,
			want:     "names no account-specific token",
		},
		{
			name:     "a label the fixture never carries is unreachable",
			fixture:  planFixture,
			expected: `{"cites_label":"nowhere","names_token":"quote tracking"}`,
			want:     "does not carry",
		},
		{
			name:     "a token no message produced could only be invented",
			fixture:  planFixture,
			expected: `{"cites_label":"wish_list","names_token":"zeppelin leasing"}`,
			want:     "appears in no message",
		},
		{
			name:     "a fixture of the wrong shape",
			fixture:  `["not an object"]`,
			expected: planExpectation,
			want:     "not the shape this site takes",
		},
		{
			name:     "an expectation of the wrong shape",
			fixture:  planFixture,
			expected: `"just a string"`,
			want:     "not this site's shape",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := meetingBriefCases{}.Prepare(json.RawMessage(tc.fixture), json.RawMessage(tc.expected))
			if err == nil {
				t.Fatalf("the case accepted a scenario that measures nothing (%s)", tc.name)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want it to name %q", err, tc.want)
			}
		})
	}
}

// A scenario the corpus actually carries must prepare, or the gate beside this
// one would be refusing every real run.
func TestTheMeetingBriefCasePreparesARealScenario(t *testing.T) {
	t.Parallel()
	prepared, err := meetingBriefCases{}.Prepare(json.RawMessage(planFixture), json.RawMessage(planExpectation))
	if err != nil {
		t.Fatalf("preparing a scenario the corpus carries: %v", err)
	}
	if prepared == nil {
		t.Fatal("Prepare returned no case and no error")
	}
	if got := (meetingBriefCases{}).Site().Variant; got != "meeting_brief" {
		t.Errorf("variant = %q, want meeting_brief", got)
	}
}
