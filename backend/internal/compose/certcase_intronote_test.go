// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What the intro-note case owes the certification lane: it issues the request
// the endpoint issues, it reads the reply through the endpoint's own checker,
// and it separates the three things a reply can be.
//
// The separation carries the meaning of a run here. A reply the checker refuses
// is INVALID and the reader would have seen the template instead — scoring the
// template would certify prose no model wrote, and this site's template is good
// enough that a grader would happily pass it.
//
// That the seam these functions go through assembles the SAME facts the HTTP
// handler assembles is proven where the handler lives
// (network.TestAScenarioBecomesTheFactsTheHandlerAssembles). It is proven there
// rather than claimed here because Reads is not constructible from this
// package — and an unheld claim in a comment is worse than none, since the next
// reader greps, finds it, and stops looking.

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/compose/network"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// noteFixtureJSON is a scenario's fixture as the corpus states it.
const noteFixtureJSON = `{
	"colleague": "Sofia Meier",
	"contact": "Philipp Königs",
	"requester": "Jonas Weber",
	"through": "",
	"relationship": "moderate",
	"last_spoke": "2026-08-20",
	"why_it_matters": "We have done depot retrofits at two carriers their size."
}`

// sendableNote is a reply the production checker accepts: it greets the contact
// by first name and names the rep in full, which is what parseIntroNote demands.
const sendableNote = `{"subject":"Introducing Jonas Weber",
	"body":"Hi Philipp,\n\nI wanted to put you in touch with Jonas Weber.\n\nBest,\nSofia"}`

// noteCompleterStub answers with whatever the case wrote for it.
type noteCompleterStub struct {
	reply string
	err   error
	reqs  []model.Request
}

func (s *noteCompleterStub) Complete(_ context.Context, req model.Request) (model.Response, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return model.Response{}, s.err
	}
	return model.Response{Text: s.reply}, nil
}

func preparedNote(t *testing.T, forbidden string) aitasks.PreparedCase {
	t.Helper()
	prepared, err := introNoteCases{}.Prepare(json.RawMessage(noteFixtureJSON), json.RawMessage(forbidden))
	if err != nil {
		t.Fatalf("preparing the scenario: %v", err)
	}
	return prepared
}

// The site this case certifies is the one the registry binds it to. A case
// answering for a different site would score one site's model on another's
// prompt and file the result under the wrong name.
func TestTheIntroNoteCaseNamesTheSiteItCertifies(t *testing.T) {
	t.Parallel()
	site := introNoteCases{}.Site()
	if got := string(site.Task) + "/" + site.Variant; got != introNoteSite {
		t.Fatalf("the case certifies %q, want %q", got, introNoteSite)
	}
}

// Run issues the production request, and it must be the production one: a case
// that built its own prompt would measure a copy that stays green through the
// change which breaks the endpoint.
func TestTheIntroNoteCaseIssuesTheEndpointsOwnRequest(t *testing.T) {
	t.Parallel()
	var fixture network.IntroNoteFixture
	if err := json.Unmarshal([]byte(noteFixtureJSON), &fixture); err != nil {
		t.Fatalf("reading the fixture: %v", err)
	}
	stub := &noteCompleterStub{reply: sendableNote}

	trace, err := preparedNote(t, `[]`).Run(t.Context(), stub)
	if err != nil {
		t.Fatalf("running: %v", err)
	}
	if len(stub.reqs) != 1 {
		t.Fatalf("the case made %d model call(s), want the one this site sends", len(stub.reqs))
	}
	// What this asserts is that the case SENT the seam's request rather than one
	// of its own — not that the seam is right, which network's tests hold. A
	// comparison against network.IntroNoteRequestFor would be circular, since
	// that is the function Prepare calls.
	sent := stub.reqs[0]
	if len(sent.Messages) != 1 {
		t.Fatalf("the case sent %d message(s), want the one this site sends", len(sent.Messages))
	}
	for _, fact := range []string{fixture.Contact, fixture.Requester, fixture.Colleague, fixture.Value} {
		if !strings.Contains(sent.Messages[0].Content, fact) {
			t.Errorf("the fixture's %q never reached the model", fact)
		}
	}
	// The stripper and the schema are the endpoint's, and a case that dropped
	// either would certify a call with a different contract than production's.
	if sent.SecretStripper == nil {
		t.Error("the case sent the call without the endpoint's secret stripper")
	}
	if !strings.Contains(string(sent.ResponseSchema), "subject") {
		t.Error("the case did not demand this site's reply shape")
	}
	if trace.Output != sendableNote {
		t.Errorf("the trace carried %q, want the model's reply", trace.Output)
	}
	if len(trace.Requests) != 1 {
		t.Fatalf("the trace recorded %d request(s), want the one that was sent", len(trace.Requests))
	}
}

// A lane failure is the case's failure, not a verdict about the model: nothing
// came back to score.
func TestAFailedLaneIsReportedRatherThanScored(t *testing.T) {
	t.Parallel()
	stub := &noteCompleterStub{err: errors.New("every bound tier failed")}
	_, err := preparedNote(t, `[]`).Run(t.Context(), stub)
	if err == nil {
		t.Fatal("a lane that never answered was reported as a run")
	}
}

// A reply the endpoint refuses is INVALID, never a low score. The reader would
// have been handed the template, and this site's template is good enough that a
// grader would pass it — so scoring it would certify prose no model wrote.
func TestAReplyTheEndpointRefusesIsInvalid(t *testing.T) {
	t.Parallel()
	for name, reply := range map[string]string{
		"not the envelope":    "Here is the note you asked for.",
		"no subject":          `{"subject":"","body":"Hi Philipp,\n\nMeet Jonas Weber."}`,
		"never names the rep": `{"subject":"An introduction","body":"Hi Philipp,\n\nMeet a colleague of mine."}`,
		"never greets the contact": `{"subject":"Introducing Jonas Weber",` +
			`"body":"Hello there,\n\nI wanted to introduce Jonas Weber."}`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := preparedNote(t, `[]`).Evaluate(aitasks.Trace{Output: reply})
			if got.Result != aitasks.OutcomeInvalid {
				t.Fatalf("a reply the endpoint refuses scored %q, want %q", got.Result, aitasks.OutcomeInvalid)
			}
			if got.Detail == "" {
				t.Error("an invalid outcome said nothing about why the reply was refused")
			}
		})
	}
}

// A sendable note that claims nothing the record does not hold is accepted.
func TestASendableNoteThatClaimsNothingIsAccepted(t *testing.T) {
	t.Parallel()
	got := preparedNote(t, `["close friend","asked me to"]`).Evaluate(aitasks.Trace{Output: sendableNote})
	if got.Result != aitasks.OutcomeAccepted {
		t.Fatalf("a clean note scored %q (%s), want %q", got.Result, got.Detail, aitasks.OutcomeAccepted)
	}
}

// A forbidden phrase is a WRONG ANSWER, not an invalid one: the note is
// well-formed and sendable, and says something the record does not support.
func TestANoteClaimingWhatTheRecordDoesNotHoldIsAWrongAnswer(t *testing.T) {
	t.Parallel()
	overclaimed := `{"subject":"Introducing Jonas Weber",
		"body":"Hi Philipp,\n\nJonas Weber is a close friend of mine.\n\nBest,\nSofia"}`
	got := preparedNote(t, `["close friend"]`).Evaluate(aitasks.Trace{Output: overclaimed})
	if got.Result != aitasks.OutcomeWrongAnswer {
		t.Fatalf("an overclaiming note scored %q, want %q", got.Result, aitasks.OutcomeWrongAnswer)
	}
	if !strings.Contains(got.Detail, "close friend") {
		t.Errorf("the detail did not name the phrase that was claimed: %q", got.Detail)
	}
}

// The SUBJECT is searched too, and this is where the case differs from its
// colleague-facing sibling: this site asks for a subject line naming the rep,
// so the subject is prose a customer reads. An overclaim placed there would
// otherwise go out unexamined.
func TestAnOverclaimInTheSubjectIsCaughtToo(t *testing.T) {
	t.Parallel()
	inSubject := `{"subject":"Introducing my close friend Jonas Weber",
		"body":"Hi Philipp,\n\nI wanted to put you in touch with Jonas Weber.\n\nBest,\nSofia"}`
	got := preparedNote(t, `["close friend"]`).Evaluate(aitasks.Trace{Output: inSubject})
	if got.Result != aitasks.OutcomeWrongAnswer {
		t.Fatalf("an overclaim in the subject scored %q, want %q", got.Result, aitasks.OutcomeWrongAnswer)
	}
}

// The match is case-insensitive, because a model that capitalises a claim has
// still made it.
func TestAForbiddenPhraseIsCaughtWhateverItsCase(t *testing.T) {
	t.Parallel()
	shouted := `{"subject":"Introducing Jonas Weber",
		"body":"Hi Philipp,\n\nJonas Weber is a CLOSE FRIEND of mine.\n\nBest,\nSofia"}`
	got := preparedNote(t, `["close friend"]`).Evaluate(aitasks.Trace{Output: shouted})
	if got.Result != aitasks.OutcomeWrongAnswer {
		t.Fatalf("a capitalised claim scored %q, want %q", got.Result, aitasks.OutcomeWrongAnswer)
	}
}

// A fixture missing any of the three people the note cannot be written without
// describes a call the product never makes, and is refused before a model is
// paid for.
func TestAFixtureMissingAPersonTheNoteNeedsIsRefused(t *testing.T) {
	t.Parallel()
	for name, fixture := range map[string]string{
		"no recipient": `{"colleague":"Sofia Meier","requester":"Jonas Weber"}`,
		"no colleague": `{"contact":"Philipp Königs","requester":"Jonas Weber"}`,
		"no rep":       `{"colleague":"Sofia Meier","contact":"Philipp Königs"}`,
		"blank rep":    `{"colleague":"Sofia Meier","contact":"Philipp Königs","requester":"   "}`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := introNoteCases{}.Prepare(json.RawMessage(fixture), json.RawMessage(`[]`))
			if err == nil {
				t.Fatal("a fixture describing a note the endpoint never produces was prepared anyway")
			}
		})
	}
}

// A scenario this site cannot read is refused rather than run: paying a model to
// answer a prompt built from nothing measures nothing.
func TestAnUnreadableScenarioIsRefusedRatherThanRun(t *testing.T) {
	t.Parallel()
	_, shapeErr := introNoteCases{}.Prepare(json.RawMessage(`"not an object"`), json.RawMessage(`[]`))
	if shapeErr == nil {
		t.Fatal("a fixture that is not this site's shape was prepared anyway")
	}
	_, expectErr := introNoteCases{}.Prepare(
		json.RawMessage(noteFixtureJSON), json.RawMessage(`{"not":"a list"}`))
	if expectErr == nil {
		t.Fatal("an expectation that is not a list of phrases was prepared anyway")
	}
}

// A scenario written in the other contract's relationship vocabulary is refused
// before a model is paid for it.
//
// The route's buckets are none/weak/moderate/strong; cold/developing/strong is
// the company page's enum. A scenario in the wrong one puts a word in the prompt
// this endpoint never sends, so a green run would grade a prompt nobody runs —
// and nothing downstream could tell.
func TestAScenarioInTheOtherContractsVocabularyIsRefused(t *testing.T) {
	t.Parallel()
	wrong := `{"colleague":"Sofia Meier","contact":"Philipp Königs","requester":"Jonas Weber",
		"relationship":"developing","last_spoke":"2026-08-20"}`
	_, err := introNoteCases{}.Prepare(json.RawMessage(wrong), json.RawMessage(`[]`))
	if err == nil {
		t.Fatal("a scenario using the company page's relationship words was prepared anyway")
	}
	if !strings.Contains(err.Error(), "developing") {
		t.Errorf("the refusal did not name the word at fault: %v", err)
	}
}

// A blank forbidden phrase is refused, because it is contained by every reply:
// one in the list would report a wrong answer on every run whatever the model
// wrote, and the site would read as never able to write a sendable note.
func TestABlankForbiddenPhraseIsRefused(t *testing.T) {
	t.Parallel()
	for _, expected := range []string{`[""]`, `["   "]`, `["close friend","\t"]`} {
		_, err := introNoteCases{}.Prepare(json.RawMessage(noteFixtureJSON), json.RawMessage(expected))
		if err == nil {
			t.Errorf("%s was prepared, and would have failed every run", expected)
		}
	}
	// A list with no phrases at all stays legal: it means the scenario is
	// checking only that a forwardable note came back.
	_, err := introNoteCases{}.Prepare(json.RawMessage(noteFixtureJSON), json.RawMessage(`[]`))
	if err != nil {
		t.Errorf("an empty expectation was refused: %v", err)
	}
}
