// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What the owed-verdict case owes the certification lane: it issues the request
// the engine issues, judges the reply with the batch-fidelity validator the
// engine judges it with, and keeps apart the three things a reply can be.
//
// A batch reply breaks in ways a single answer cannot — a verdict for a message
// nobody sent, one message answered twice, one left out — and every one of those
// is UNUSABLE rather than wrong. A case that collapsed them into "wrong" would
// report a broken batch contract as a quality problem, and somebody would go
// looking at the prompt.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// owedCompleterStub answers from the ids the request actually asks about, which
// is the only place they exist: Prepare mints them and hands them to nobody. A
// stub told the ids up front would prove less than a model reading them.
type owedCompleterStub struct {
	answer func(requestedIDs []string) string
	seen   []model.Request
}

func (s *owedCompleterStub) Complete(_ context.Context, req model.Request) (model.Response, error) {
	s.seen = append(s.seen, req)
	requested, err := requestedIDsIn(req)
	if err != nil {
		return model.Response{}, err
	}
	return model.Response{Text: s.answer(requested)}, nil
}

// owedReplyText is raw text rather than a marshalled struct, so a malformed
// reply is as expressible as a well-formed one.
func owedReplyText(results ...string) string {
	return `{"results":[` + strings.Join(results, ",") + `]}`
}

// owedVerdictFor is one judged message in the shape the prompt demands. The
// confidence sits above the engine's floor throughout: what this case measures
// is the reply's fidelity and its verdicts, and the floor is the engine's
// separate decision about an answer it already believes.
func owedVerdictFor(id, verdict string) string {
	return `{"id":"` + id + `","verdict":"` + verdict + `","confidence":0.9}`
}

// owedAnswer answers every requested id positionally with the verdicts given —
// the shape of a reply that honoured the batch contract, whatever it says.
func owedAnswer(verdicts ...string) func(requested []string) string {
	return func(requested []string) string {
		results := make([]string, 0, len(requested))
		for i, id := range requested {
			results = append(results, owedVerdictFor(id, verdicts[i]))
		}
		return owedReplyText(results...)
	}
}

// The two messages every case below judges: one putting a question to us, one
// reporting to a desk address with the reader merely copied. The expected
// verdicts differ, so a case cannot pass by answering the same token twice — and
// the second is the shape that opened this whole piece of work.
func owedFixtureJSON(t *testing.T) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(owedFixture{
		{
			Subject: "Re: the retrofit quote",
			Body:    "Can you confirm the price before Friday?",
			To:      []string{"lars@ourco.test"},
		},
		{
			Subject: "Monatsreporting Juli",
			Body:    "Attached is the monthly report. No action needed.",
			To:      []string{"reporting@customer.test"},
			Cc:      []string{"lars@ourco.test"},
		},
	})
	if err != nil {
		t.Fatalf("encoding the fixture: %v", err)
	}
	return raw
}

func owedExpectation(t *testing.T, verdicts ...string) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(verdicts)
	if err != nil {
		t.Fatalf("encoding the expectation: %v", err)
	}
	return raw
}

func runOwedCase(t *testing.T, expected []string, answer func(requested []string) string) (aitasks.Outcome, aitasks.Trace) {
	t.Helper()
	prepared, err := owedVerdictCases{}.Prepare(owedFixtureJSON(t), owedExpectation(t, expected...))
	if err != nil {
		t.Fatalf("preparing the case: %v", err)
	}
	trace, err := prepared.Run(context.Background(), &owedCompleterStub{answer: answer})
	if err != nil {
		t.Fatalf("running the case: %v", err)
	}
	return prepared.Evaluate(trace), trace
}

func TestOwedCaseSeparatesTheThreeThingsAReplyCanBe(t *testing.T) {
	expected := []string{activities.OwedVerdictAsksUs, activities.OwedVerdictInformsUs}

	cases := []struct {
		name       string
		answer     func(requested []string) string
		wantResult string
		wantDetail string
	}{
		{
			name:       "the expected verdicts, well formed",
			answer:     owedAnswer(expected...),
			wantResult: aitasks.OutcomeAccepted,
		},
		{
			// A verdict for a message this call never carried is the shape a
			// talked-into model takes, and the record has to be able to say so.
			name: "a verdict for a message nobody asked about",
			answer: func([]string) string {
				return owedReplyText(owedVerdictFor(ids.NewV7().String(), activities.OwedVerdictAsksUs))
			},
			wantResult: aitasks.OutcomeInvalid,
			wantDetail: "was not requested",
		},
		{
			name: "one message answered twice and the other not at all",
			answer: func(requested []string) string {
				return owedReplyText(
					owedVerdictFor(requested[0], activities.OwedVerdictAsksUs),
					owedVerdictFor(requested[0], activities.OwedVerdictInformsUs))
			},
			wantResult: aitasks.OutcomeInvalid,
			wantDetail: "appears twice",
		},
		{
			// A silently dropped message leaves its row read out of the backlog
			// and never judged.
			name: "a message left out of the results",
			answer: func(requested []string) string {
				return owedReplyText(owedVerdictFor(requested[0], activities.OwedVerdictAsksUs))
			},
			wantResult: aitasks.OutcomeInvalid,
			wantDetail: "missing from the results",
		},
		{
			name:       "a verdict outside the closed vocabulary",
			answer:     owedAnswer("maybe", activities.OwedVerdictInformsUs),
			wantResult: aitasks.OutcomeInvalid,
			wantDetail: "is not asks_us|informs_us",
		},
		{
			name:       "a reply that is not the required JSON",
			answer:     func([]string) string { return "I decline to answer." },
			wantResult: aitasks.OutcomeInvalid,
			wantDetail: "unparseable",
		},
		{
			// Well formed and wrong measures the MODEL, not the reply — the
			// opposite fix from every case above it.
			name:       "a well-formed answer the scenario disagrees with",
			answer:     owedAnswer(activities.OwedVerdictAsksUs, activities.OwedVerdictAsksUs),
			wantResult: aitasks.OutcomeWrongAnswer,
			wantDetail: `message 2 is judged "asks_us" where the scenario expects "informs_us"`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			outcome, _ := runOwedCase(t, expected, tc.answer)
			if outcome.Result != tc.wantResult {
				t.Fatalf("Result = %q (%s), want %q", outcome.Result, outcome.Detail, tc.wantResult)
			}
			if !strings.Contains(outcome.Detail, tc.wantDetail) {
				t.Errorf("Detail = %q, want it to name %q", outcome.Detail, tc.wantDetail)
			}
		})
	}
}

// The envelope reaches the prompt, and that is the whole reason this site can
// answer at all.
//
// A report to a desk address with the reader merely copied reads exactly like a
// direct request from its subject and body alone — it is how the message that
// opened this work reached the top of a rep's day. A prompt without the
// recipient line certifies a strictly weaker question than the one production
// asks.
func TestTheOwedRequestCarriesTheRecipientLine(t *testing.T) {
	_, trace := runOwedCase(t,
		[]string{activities.OwedVerdictAsksUs, activities.OwedVerdictInformsUs},
		owedAnswer(activities.OwedVerdictAsksUs, activities.OwedVerdictInformsUs))

	prompt := trace.Requests[0].Messages[0].Content
	for _, want := range []string{"reporting@customer.test", "Cc: lars@ourco.test"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("the prompt does not carry %q:\n%s", want, prompt)
		}
	}
}

// A calendar invitation says so in the prompt, because it changes the answer:
// an invite that asks nothing a calendar reply cannot settle is informs_us.
func TestTheOwedRequestSaysWhenAMessageCarriedAnInvitation(t *testing.T) {
	fixture, err := json.Marshal(owedFixture{
		{Subject: "coffee with Lars @1500", Body: "See you then.", HasCalendarPart: true},
	})
	if err != nil {
		t.Fatalf("encoding the fixture: %v", err)
	}
	prepared, err := owedVerdictCases{}.Prepare(fixture, owedExpectation(t, activities.OwedVerdictInformsUs))
	if err != nil {
		t.Fatalf("preparing the case: %v", err)
	}
	stub := &owedCompleterStub{answer: owedAnswer(activities.OwedVerdictInformsUs)}
	if _, err := prepared.Run(context.Background(), stub); err != nil {
		t.Fatalf("running the case: %v", err)
	}
	if !strings.Contains(stub.seen[0].Messages[0].Content, "calendar invitation") {
		t.Error("the prompt does not say the message carried an invitation")
	}
}

// Every disagreement reaches the Detail, not just the first.
func TestOwedCaseNamesEveryMessageItDisagreesWith(t *testing.T) {
	outcome, _ := runOwedCase(t,
		[]string{activities.OwedVerdictAsksUs, activities.OwedVerdictInformsUs},
		owedAnswer(activities.OwedVerdictInformsUs, activities.OwedVerdictAsksUs))

	if outcome.Result != aitasks.OutcomeWrongAnswer {
		t.Fatalf("Result = %q (%s), want a wrong answer", outcome.Result, outcome.Detail)
	}
	for _, want := range []string{"message 1", "message 2"} {
		if !strings.Contains(outcome.Detail, want) {
			t.Errorf("Detail = %q, want it to name %s", outcome.Detail, want)
		}
	}
}

// The fixture supplies no id, and Prepare mints a fresh one per message per
// preparation. Were the ids to come from the corpus, their author could write
// them into the expected reply, and a model merely echoing ids it was handed
// would be indistinguishable from one that judged the right messages.
func TestOwedCaseMintsAnIDPerMessageRatherThanReadingIt(t *testing.T) {
	fixture := owedFixtureJSON(t)
	expected := []string{activities.OwedVerdictAsksUs, activities.OwedVerdictInformsUs}

	ask := func() []string {
		t.Helper()
		prepared, err := owedVerdictCases{}.Prepare(fixture, owedExpectation(t, expected...))
		if err != nil {
			t.Fatalf("preparing the case: %v", err)
		}
		stub := &owedCompleterStub{answer: owedAnswer(expected...)}
		if _, err := prepared.Run(context.Background(), stub); err != nil {
			t.Fatalf("running the case: %v", err)
		}
		requested, err := requestedIDsIn(stub.seen[0])
		if err != nil {
			t.Fatalf("reading the requested ids: %v", err)
		}
		return requested
	}

	first, second := ask(), ask()
	if len(first) != 2 {
		t.Fatalf("the request asks about %d messages, want one id per fixture entry", len(first))
	}
	seen := map[string]bool{}
	for _, id := range append(append([]string{}, first...), second...) {
		if _, err := ids.Parse(id); err != nil {
			t.Errorf("the requested id %q is not one this system mints: %v", id, err)
		}
		if seen[id] {
			t.Errorf("the id %q was asked about twice", id)
		}
		seen[id] = true
	}
}

// The trace is what the canary gate and the record read. A case that ran the
// production request but recorded nothing would certify a request nobody can
// inspect.
func TestOwedCaseTraceCarriesTheRequestItIssued(t *testing.T) {
	expected := []string{activities.OwedVerdictAsksUs, activities.OwedVerdictInformsUs}
	outcome, trace := runOwedCase(t, expected, owedAnswer(expected...))

	if outcome.Result != aitasks.OutcomeAccepted {
		t.Fatalf("Result = %q (%s), want accepted", outcome.Result, outcome.Detail)
	}
	if len(trace.Requests) != 1 {
		t.Fatalf("the trace carries %d requests, want the one this site issues", len(trace.Requests))
	}
	if !strings.Contains(trace.Requests[0].System, "asks its recipient side for something") {
		t.Errorf("the traced request is not the owed-verdict prompt: %q", trace.Requests[0].System)
	}
	if !strings.Contains(trace.Requests[0].Messages[0].Content, "Can you confirm the price before Friday?") {
		t.Errorf("a fixture body never reached the request:\n%s", trace.Requests[0].Messages[0].Content)
	}
	if trace.Output == "" {
		t.Error("the trace records no model output for the validator to read")
	}
}

// An expectation the validator can never satisfy would measure nothing for as
// long as it sat in the corpus. Prepare names it while it is still a wiring
// error rather than a paid run of zeros.
func TestOwedCaseRefusesAnUnreachableExpectation(t *testing.T) {
	cases := []struct {
		name       string
		verdicts   []string
		wantReason string
	}{
		{
			name:       "a verdict outside the closed vocabulary",
			verdicts:   []string{activities.OwedVerdictAsksUs, "maybe"},
			wantReason: "asks_us|informs_us",
		},
		{
			name:       "fewer verdicts than the batch carries messages",
			verdicts:   []string{activities.OwedVerdictAsksUs},
			wantReason: "one verdict per message",
		},
		{
			name:       "more verdicts than the batch carries messages",
			verdicts:   []string{activities.OwedVerdictAsksUs, activities.OwedVerdictInformsUs, activities.OwedVerdictAsksUs},
			wantReason: "one verdict per message",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := owedVerdictCases{}.Prepare(owedFixtureJSON(t), owedExpectation(t, tc.verdicts...))
			if err == nil {
				t.Fatalf("a scenario expecting %v prepared", tc.verdicts)
			}
			if !strings.Contains(err.Error(), tc.wantReason) {
				t.Errorf("the refusal does not say why it is unreachable: %v", err)
			}
		})
	}
}

// A batch this site could never have been handed certifies a prompt the product
// cannot send: the backlog read caps a call at ten messages and truncates every
// body, so a fixture beyond either bound describes work no cycle does.
func TestOwedCaseRefusesABatchProductionCouldNeverRead(t *testing.T) {
	oversized := make(owedFixture, owedBatchSize+1)
	verdicts := make([]string, len(oversized))
	for i := range oversized {
		oversized[i] = owedFixtureMessage{Subject: "a question", Body: "Can you confirm?"}
		verdicts[i] = activities.OwedVerdictAsksUs
	}

	cases := []struct {
		name       string
		fixture    owedFixture
		verdicts   []string
		wantReason string
	}{
		{
			name:       "no message at all",
			fixture:    owedFixture{},
			verdicts:   []string{},
			wantReason: "supplies no message",
		},
		{
			name:       "more messages than one call ever carries",
			fixture:    oversized,
			verdicts:   verdicts,
			wantReason: "one call judges at most",
		},
		{
			name: "a body longer than the backlog read returns",
			fixture: owedFixture{
				{Subject: "a question", Body: strings.Repeat("a", owedBodyLimit+1)},
			},
			verdicts:   []string{activities.OwedVerdictAsksUs},
			wantReason: "truncates every body",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fixture, err := json.Marshal(tc.fixture)
			if err != nil {
				t.Fatalf("encoding the fixture: %v", err)
			}
			_, err = owedVerdictCases{}.Prepare(fixture, owedExpectation(t, tc.verdicts...))
			if err == nil {
				t.Fatal("a batch production could never read prepared")
			}
			if !strings.Contains(err.Error(), tc.wantReason) {
				t.Errorf("the refusal does not say what the backlog read would have handed it: %v", err)
			}
		})
	}
}

// The case must be reachable through the same registry the census validates, or
// the site is registered and served by nothing.
func TestTaskCensusBindsTheOwedVerdictCase(t *testing.T) {
	registry, err := NewTaskCensus()
	if err != nil {
		t.Fatalf("the census does not validate: %v", err)
	}
	site := owedVerdictCases{}.Site()
	bound, ok := registry.CaseFor(site.Task, site.Variant)
	if !ok {
		t.Fatalf("no certification case is bound to %s/%s", site.Task, site.Variant)
	}
	if bound.Site() != site {
		t.Errorf("the bound case serves %+v, want %+v", bound.Site(), site)
	}
}
