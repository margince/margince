// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What the classify case owes the certification lane: it drives the production
// pass over an in-memory backlog. The request, the validator, the floor and the
// solo re-ask are then the engine's own. It also separates three things a run can
// be. A batch reply breaks in ways a single answer cannot: a label for a message
// nobody sent, one answered twice, one left out. Each is unusable, not wrong.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/promptfence"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// classifyCompleterStub answers from the ids the request actually asks about,
// which is the only place those ids exist: Prepare mints them and hands them to
// nobody. A stub told the ids up front would prove less than a model reading
// them.
type classifyCompleterStub struct {
	answer func(requestedIDs []string) string
	seen   []model.Request
}

func (s *classifyCompleterStub) Complete(_ context.Context, req model.Request) (model.Response, error) {
	s.seen = append(s.seen, req)
	requested, err := requestedIDsIn(req)
	if err != nil {
		return model.Response{}, err
	}
	return model.Response{Text: s.answer(requested)}, nil
}

// requestedIDsIn reads the ids off the identified spans, in prompt order,
// anchored on the boundary this request's own system prompt declares.
func requestedIDsIn(req model.Request) ([]string, error) {
	marker, declared := promptfence.MarkerIn(req.System)
	if !declared {
		return nil, errors.New("the classify request declares no data boundary")
	}
	if len(req.Messages) != 1 {
		return nil, fmt.Errorf("the classify request has %d messages, want the single user turn", len(req.Messages))
	}
	var out []string
	rest := req.Messages[0].Content
	for {
		_, span, found := strings.Cut(rest, "<"+marker+` source_id="`)
		if !found {
			break
		}
		id, after, closed := strings.Cut(span, `"`)
		if !closed {
			return nil, errors.New("an identified span never closes its id attribute")
		}
		out = append(out, id)
		rest = after
	}
	if len(out) == 0 {
		return nil, errors.New("the classify request carries no identified span")
	}
	return out, nil
}

// classifyReplyText is the raw text a model returns, built as text rather than
// marshalled so a malformed reply is as expressible as a well-formed one.
func classifyReplyText(results ...string) string {
	return `{"results":[` + strings.Join(results, ",") + `]}`
}

// classifyVerdictFor is one labeled message in the shape the classify prompt
// demands. The confidence sits above the engine's floor throughout: what this
// case measures is the reply's fidelity and its labels, and the floor is the
// engine's separate decision about what to do with an answer it already
// believes.
func classifyVerdictFor(id, label string) string {
	return fmt.Sprintf(`{"id":%q,"label":%q,"confidence":0.9}`, id, label)
}

// classifyAnswer answers every requested id, positionally, with the labels
// given — the shape of a reply that honoured the batch contract, whatever it
// says.
func classifyAnswer(labels ...string) func(requested []string) string {
	return func(requested []string) string {
		results := make([]string, 0, len(requested))
		for i, id := range requested {
			results = append(results, classifyVerdictFor(id, labels[i]))
		}
		return classifyReplyText(results...)
	}
}

// The two messages every case below labels: one asking for work, one arranging
// a time, so the expected labels differ and a case cannot pass by answering the
// same token twice.
func classifyFixtureJSON(t *testing.T) json.RawMessage {
	t.Helper()
	// Both are INBOUND — a customer asking and a customer proposing a time —
	// which is what makes the prompt's reply half reachable at all. A fixture of
	// outbound mail would certify a prompt that never asks the reply question,
	// and would stay green through a change that broke it.
	raw, err := json.Marshal(captureClassifyFixture{
		{Subject: "quote please", Body: "We need forty seats by March.", Inbound: true},
		{Subject: "lunch thursday", Body: "Shall we say noon at the usual place?", Inbound: true},
	})
	if err != nil {
		t.Fatalf("encoding the fixture: %v", err)
	}
	return raw
}

// classifyExpectation is what the corpus asserts, encoded as the corpus will
// carry it — beside the fixture, never inside it.
func classifyExpectation(t *testing.T, labels ...string) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(labels)
	if err != nil {
		t.Fatalf("encoding the expectation: %v", err)
	}
	return raw
}

func runClassifyCase(t *testing.T, expected []string, answer func(requested []string) string) (aitasks.Outcome, aitasks.Trace) {
	t.Helper()
	prepared, err := captureClassifyCases{}.Prepare(classifyFixtureJSON(t), classifyExpectation(t, expected...))
	if err != nil {
		t.Fatalf("preparing the case: %v", err)
	}
	trace, err := prepared.Run(context.Background(), &classifyCompleterStub{answer: answer})
	if err != nil {
		t.Fatalf("running the case: %v", err)
	}
	return prepared.Evaluate(trace), trace
}

func TestClassifyCaseSeparatesTheThreeThingsAReplyCanBe(t *testing.T) {
	expected := []string{"commitment", "meeting"}

	cases := []struct {
		name       string
		answer     func(requested []string) string
		wantResult string
		wantDetail string
	}{
		{
			name:       "the expected labels, well formed",
			answer:     classifyAnswer(expected...),
			wantResult: aitasks.OutcomeAccepted,
		},
		{
			// The production validator's own refusal, reported in its own words: a
			// label for a message this call never carried is the shape a talked-into
			// model takes, and the record has to be able to say so.
			name: "a label for a message nobody asked about",
			answer: func(requested []string) string {
				return classifyReplyText(classifyVerdictFor(ids.NewV7().String(), "other"))
			},
			wantResult: aitasks.OutcomeInvalid,
			wantDetail: "was not requested",
		},
		{
			name: "one message answered twice and the other not at all",
			answer: func(requested []string) string {
				return classifyReplyText(
					classifyVerdictFor(requested[0], "commitment"), classifyVerdictFor(requested[0], "other"))
			},
			wantResult: aitasks.OutcomeInvalid,
			wantDetail: "appears twice",
		},
		{
			// A silently dropped message would leave its row read out of the backlog
			// and never labeled.
			name: "a message left out of the results",
			answer: func(requested []string) string {
				return classifyReplyText(classifyVerdictFor(requested[0], "commitment"))
			},
			wantResult: aitasks.OutcomeInvalid,
			wantDetail: "missing from the results",
		},
		{
			// The pass drops an answer it cannot use and asks about the message alone. A
			// second unusable answer leaves it unsure. That is what the product serves, and
			// what the case counts against the model.
			name:       "a label outside the closed vocabulary, asked again and still outside it",
			answer:     classifyAnswer("spam", "meeting"),
			wantResult: aitasks.OutcomeWrongAnswer,
			wantDetail: `message 1 was left unsure, with no label, where the scenario expects "commitment"`,
		},
		{
			name:       "a reply that is not the required JSON",
			answer:     func([]string) string { return "I decline to answer." },
			wantResult: aitasks.OutcomeInvalid,
			wantDetail: "unparseable",
		},
		{
			// Well formed and wrong is a measurement of the model, not a defect in
			// the reply — the opposite fix from every case above it.
			name:       "a well-formed answer the scenario disagrees with",
			answer:     classifyAnswer("commitment", "other"),
			wantResult: aitasks.OutcomeWrongAnswer,
			wantDetail: `message 2 is labeled "other" where the scenario expects "meeting"`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			outcome, _ := runClassifyCase(t, expected, tc.answer)
			if outcome.Result != tc.wantResult {
				t.Fatalf("Result = %q (%s), want %q", outcome.Result, outcome.Detail, tc.wantResult)
			}
			if !strings.Contains(outcome.Detail, tc.wantDetail) {
				t.Errorf("Detail = %q, want it to name %q", outcome.Detail, tc.wantDetail)
			}
		})
	}
}

// Every disagreement reaches the Detail, not just the first: a run that labeled
// one message right and two wrong is not the near miss the first line alone
// would read as.
func TestClassifyCaseNamesEveryMessageItDisagreesWith(t *testing.T) {
	outcome, _ := runClassifyCase(t, []string{"commitment", "meeting"}, classifyAnswer("other", "other"))

	if outcome.Result != aitasks.OutcomeWrongAnswer {
		t.Fatalf("Result = %q (%s), want a wrong answer", outcome.Result, outcome.Detail)
	}
	for _, want := range []string{"message 1", "message 2"} {
		if !strings.Contains(outcome.Detail, want) {
			t.Errorf("Detail = %q, want it to name %s", outcome.Detail, want)
		}
	}
}

// A fixture is what PRODUCTION is given; an expectation is what the CORPUS
// asserts. Keeping them apart is what lets a gate rewrite every free-text field
// of a fixture — the canary sweep does exactly that — without rewriting an
// assertion, and it is what keeps the minted ids out of the corpus entirely.
func TestClassifyFixtureCarriesOnlyWhatProductionIsGiven(t *testing.T) {
	var messages []map[string]json.RawMessage
	if err := json.Unmarshal(classifyFixtureJSON(t), &messages); err != nil {
		t.Fatalf("decoding the fixture: %v", err)
	}
	if len(messages) == 0 {
		t.Fatal("the fixture carries no message")
	}
	// What the backlog row actually hands the engine. `inbound` joined it when
	// the same call started judging whether a reply was positive: the prompt
	// asks that question only of mail somebody sent us, so a fixture unable to
	// say which direction a message went could not reach half the prompt.
	given := map[string]bool{"subject": true, "body": true, "inbound": true}
	for i, fields := range messages {
		for name := range fields {
			// A chat's channel is handed over too, and only for chat.
			if !given[name] && name != "channel" {
				t.Errorf("message %d carries %q, which the backlog row does not hand the engine", i+1, name)
			}
		}
		for name := range given {
			if _, present := fields[name]; !present {
				t.Errorf("message %d drops %q, which production always supplies", i+1, name)
			}
		}
	}
}

// The model is asked about each message by its ref in fixture order, and no
// activity id reaches the request.
func TestClassifyCaseAsksByRefNeverByActivityID(t *testing.T) {
	prepared, err := captureClassifyCases{}.Prepare(classifyFixtureJSON(t), classifyExpectation(t, "commitment", "meeting"))
	if err != nil {
		t.Fatalf("preparing the case: %v", err)
	}
	stub := &classifyCompleterStub{answer: classifyAnswer("commitment", "meeting")}
	if _, err := prepared.Run(context.Background(), stub); err != nil {
		t.Fatalf("running the case: %v", err)
	}
	requested, err := requestedIDsIn(stub.seen[0])
	if err != nil {
		t.Fatalf("reading the requested ids: %v", err)
	}
	if !slices.Equal(requested, []string{"m1", "m2"}) {
		t.Errorf("the request asks about %v, want [m1 m2]", requested)
	}
	for _, m := range prepared.(*captureClassifyCase).batch {
		if strings.Contains(stub.seen[0].Messages[0].Content, m.ID.String()) {
			t.Errorf("activity id %s reached the model", m.ID)
		}
	}
}

// The trace is what the canary gate and the record read. A case that ran the
// production request but recorded nothing would certify a request nobody can
// inspect.
func TestClassifyCaseTraceCarriesTheRequestItIssued(t *testing.T) {
	outcome, trace := runClassifyCase(t, []string{"commitment", "meeting"}, classifyAnswer("commitment", "meeting"))

	if outcome.Result != aitasks.OutcomeAccepted {
		t.Fatalf("Result = %q (%s), want accepted", outcome.Result, outcome.Detail)
	}
	if len(trace.Requests) != 1 {
		t.Fatalf("the trace carries %d requests, want the one this site issues", len(trace.Requests))
	}
	if !strings.Contains(trace.Requests[0].System, "You label captured messages") {
		t.Errorf("the traced request is not the classify prompt: %q", trace.Requests[0].System)
	}
	if !strings.Contains(trace.Requests[0].Messages[0].Content, "We need forty seats by March.") {
		t.Errorf("a fixture body never reached the request:\n%s", trace.Requests[0].Messages[0].Content)
	}
	if trace.Output == "" {
		t.Error("the trace records no model output for the validator to read")
	}
}

// An expectation the validator can never satisfy would measure nothing for as
// long as it stayed in the corpus. Prepare is where that gets named, while it is
// still a wiring error rather than a paid run of zeros.
func TestClassifyCaseRefusesAnUnreachableExpectation(t *testing.T) {
	cases := []struct {
		name       string
		labels     []string
		wantReason string
	}{
		{
			name:       "a label outside the closed vocabulary",
			labels:     []string{"commitment", "spam"},
			wantReason: "commitment|meeting|other",
		},
		{
			name:       "fewer labels than the batch carries messages",
			labels:     []string{"commitment"},
			wantReason: "one label per message",
		},
		{
			name:       "more labels than the batch carries messages",
			labels:     []string{"commitment", "meeting", "other"},
			wantReason: "one label per message",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := captureClassifyCases{}.Prepare(classifyFixtureJSON(t), classifyExpectation(t, tc.labels...))
			if err == nil {
				t.Fatalf("a scenario expecting %v prepared", tc.labels)
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
func TestClassifyCaseRefusesABatchProductionCouldNeverRead(t *testing.T) {
	oversized := make(captureClassifyFixture, classifyBatchSize+1)
	labels := make([]string, len(oversized))
	for i := range oversized {
		oversized[i] = captureClassifyMessage{Subject: "quote please", Body: "We need forty seats."}
		labels[i] = "commitment"
	}

	cases := []struct {
		name       string
		fixture    captureClassifyFixture
		labels     []string
		wantReason string
	}{
		{
			name:       "no message at all",
			fixture:    captureClassifyFixture{},
			labels:     []string{},
			wantReason: "supplies no message",
		},
		{
			name:       "more messages than one call ever carries",
			fixture:    oversized,
			labels:     labels,
			wantReason: "one call labels at most",
		},
		{
			name: "a body longer than the backlog read returns",
			fixture: captureClassifyFixture{
				{Subject: "quote please", Body: strings.Repeat("a", classifyBodyLimit+1)},
			},
			labels:     []string{"commitment"},
			wantReason: "truncates every body",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fixture, err := json.Marshal(tc.fixture)
			if err != nil {
				t.Fatalf("encoding the fixture: %v", err)
			}
			_, err = captureClassifyCases{}.Prepare(fixture, classifyExpectation(t, tc.labels...))
			if err == nil {
				t.Fatal("a batch production could never read prepared")
			}
			if !strings.Contains(err.Error(), tc.wantReason) {
				t.Errorf("the refusal does not say what the backlog read would have handed it: %v", err)
			}
		})
	}
}

// A scenario shaped like something else asserts nothing about the reply, and a
// case that ran it anyway would report a number nobody wrote a claim for.
func TestClassifyCaseRefusesAnExpectationItCannotRead(t *testing.T) {
	for _, expected := range []json.RawMessage{nil, json.RawMessage(`{"labels":["other"]}`), json.RawMessage(`7`)} {
		_, err := captureClassifyCases{}.Prepare(classifyFixtureJSON(t), expected)
		if err == nil {
			t.Fatalf("a scenario expecting %s prepared", expected)
		}
		if !strings.Contains(err.Error(), "list of labels") {
			t.Errorf("the refusal does not say what an expectation must be: %v", err)
		}
	}
}

// A fixture shaped like something else is not a batch, and running it would
// certify a prompt built from whatever survived the decode.
func TestClassifyCaseRefusesAFixtureItCannotRead(t *testing.T) {
	_, err := captureClassifyCases{}.Prepare(json.RawMessage(`{"subject":"quote please"}`), classifyExpectation(t, "other"))
	if err == nil {
		t.Fatal("a fixture that is not a list of messages prepared")
	}
	if !strings.Contains(err.Error(), "shape this site takes") {
		t.Errorf("the refusal does not say what a fixture must be: %v", err)
	}
}

// The case must be reachable through the same registry the census validates, or
// the site is registered and served by nothing.
func TestTaskCensusBindsTheClassifyCase(t *testing.T) {
	registry, err := NewTaskCensus()
	if err != nil {
		t.Fatalf("the census does not validate: %v", err)
	}
	site := captureClassifyCases{}.Site()
	bound, ok := registry.CaseFor(site.Task, site.Variant)
	if !ok {
		t.Fatalf("no certification case is bound to %s/%s", site.Task, site.Variant)
	}
	if bound.Site() != site {
		t.Errorf("the bound case serves %+v, want %+v", bound.Site(), site)
	}
}

// A reply expectation grades the verdict and may leave the label open. The
// case for a short reply over a quoted refusal asks only that it is not a no.
func TestClassifyCaseGradesTheReplyWhenTheScenarioNamesOne(t *testing.T) {
	fixture, err := json.Marshal(captureClassifyFixture{{Subject: "Re: phase two", Body: "We will wait for your answer.", Inbound: true}})
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, expected, reply string
		want                  string
	}{
		{"a ruled-out verdict", `[{"not_reply":"negative"}]`, "negative", aitasks.OutcomeWrongAnswer},
		{"any other verdict with the label open", `[{"not_reply":"negative"}]`, "neutral", aitasks.OutcomeAccepted},
		{"the verdict wanted", `[{"label":"other","reply":"neutral"}]`, "neutral", aitasks.OutcomeAccepted},
		{"another verdict than the one wanted", `[{"label":"other","reply":"negative"}]`, "neutral", aitasks.OutcomeWrongAnswer},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prepared, err := captureClassifyCases{}.Prepare(fixture, json.RawMessage(tc.expected))
			if err != nil {
				t.Fatalf("preparing the case: %v", err)
			}
			answer := func(requested []string) string {
				return classifyReplyText(fmt.Sprintf(`{"id":%q,"label":"other","confidence":0.9,"reply":%q}`, requested[0], tc.reply))
			}
			trace, err := prepared.Run(context.Background(), &classifyCompleterStub{answer: answer})
			if err != nil {
				t.Fatalf("running the case: %v", err)
			}
			if got := prepared.Evaluate(trace); got.Result != tc.want {
				t.Fatalf("outcome = %+v, want %v", got, tc.want)
			}
		})
	}
}

// The backlog read sends the sender's current message only, so a fixture
// carrying a quote certifies a prompt the product never sends.
func TestClassifyCaseRefusesAFixtureCarryingQuotedMail(t *testing.T) {
	fixture, err := json.Marshal(captureClassifyFixture{{
		Subject: "Re: phase two", Inbound: true,
		Body: "We will wait for your answer.\n\nOn Mon, Anna <anna@example.test> wrote:\n> We will not go ahead.",
	}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = captureClassifyCases{}.Prepare(fixture, classifyExpectation(t, "other"))
	if err == nil || !strings.Contains(err.Error(), "current message") {
		t.Fatalf("a fixture with quoted mail prepared: %v", err)
	}
}

// classifyVerdictAt is one labeled message at the confidence given.
func classifyVerdictAt(id, label string, confidence float64) string {
	return fmt.Sprintf(`{"id":%q,"label":%q,"confidence":%v}`, id, label, confidence)
}

// A message answered below the floor in the batch is asked about alone, and the
// case judges the label that second answer gives. The floor is the engine's, so a
// case that skipped the re-ask would score the hedged first answer.
func TestClassifyCaseJudgesTheLabelTheSoloReAskGives(t *testing.T) {
	answer := func(requested []string) string {
		if len(requested) == 1 {
			return classifyReplyText(classifyVerdictAt(requested[0], "meeting", 0.9))
		}
		return classifyReplyText(classifyVerdictAt(requested[0], "commitment", 0.9), classifyVerdictAt(requested[1], "other", 0.4))
	}
	outcome, trace := runClassifyCase(t, []string{"commitment", "meeting"}, answer)

	if outcome.Result != aitasks.OutcomeAccepted {
		t.Fatalf("Result = %q (%s), want accepted: the re-ask labeled it meeting", outcome.Result, outcome.Detail)
	}
	if len(trace.Requests) != 2 {
		t.Fatalf("the trace carries %d requests, want the batch and one solo re-ask", len(trace.Requests))
	}
	if requested, _ := requestedIDsIn(trace.Requests[0]); len(requested) != 2 {
		t.Errorf("the first request asks about %v, want the whole batch", requested)
	}
	if requested, _ := requestedIDsIn(trace.Requests[1]); len(requested) != 1 {
		t.Errorf("the second request asks about %v, want the one message left under the floor", requested)
	}
}

// A message still under the floor when asked alone is stamped and not labeled.
// The run is wrong by the label it did not give, however right its first guess.
func TestClassifyCaseCountsAMessageLeftUnderTheFloorAgainstTheModel(t *testing.T) {
	answer := func(requested []string) string {
		results := make([]string, 0, len(requested))
		for i, id := range requested {
			results = append(results, classifyVerdictAt(id, []string{"commitment", "meeting"}[i%2], 0.4))
		}
		return classifyReplyText(results...)
	}
	outcome, _ := runClassifyCase(t, []string{"commitment", "meeting"}, answer)

	if outcome.Result != aitasks.OutcomeWrongAnswer || !strings.Contains(outcome.Detail, "message 1 was left unsure") {
		t.Fatalf("Result = %q (%s), want a wrong answer naming the message left unsure", outcome.Result, outcome.Detail)
	}
}

// A call that never completed is the lane's problem and no measurement of the
// model. It fails the run and does not score the batch as unlabeled.
func TestClassifyCaseFailsTheRunWhenACallFails(t *testing.T) {
	// A withheld answer and a deferral are swallowed by the pass itself, which
	// stamps or pauses and returns no error. The run must still fail.
	for name, broken := range map[string]error{
		"a plain error":     errors.New("provider unreachable"),
		"a provider outage": ai.ErrProviderDown,
		"a deferral":        ai.ErrBudgetDeferred,
		"a withheld answer": model.ErrOutputWithheld,
	} {
		t.Run(name, func(t *testing.T) {
			prepared, err := captureClassifyCases{}.Prepare(classifyFixtureJSON(t), classifyExpectation(t, "commitment", "meeting"))
			if err != nil {
				t.Fatalf("preparing the case: %v", err)
			}
			if _, err = prepared.Run(context.Background(), failingCompleter{err: broken}); !errors.Is(err, broken) {
				t.Fatalf("Run error = %v, want the completer's own", err)
			}
		})
	}
}

type failingCompleter struct{ err error }

func (f failingCompleter) Complete(context.Context, model.Request) (model.Response, error) {
	return model.Response{}, f.err
}
