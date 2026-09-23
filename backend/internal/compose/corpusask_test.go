// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The quote check, which is the whole guardrail on the written half of a corpus
// ask.
//
// Steps 1-3 are deterministic and live upstream; by the time a reply reaches
// here, passages have already cleared the grounding floor. What is left is the
// one question this file answers: did the sentence actually come out of the
// passage it cites?

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/knowledge"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

const askedPassageText = "Captured messages are kept for 400 days from the day they arrive."

func askPassages() []knowledge.Passage {
	return []knowledge.Passage{{
		ChunkID:      ids.NewV7(),
		DocumentID:   ids.NewV7(),
		DocumentName: "operating-handbook.md",
		Text:         askedPassageText,
		Similarity:   0.9,
	}}
}

// corpusReply renders a model answer over the given passage ids: a reply that
// DECLARES the passages answer the question. A call with no claims is therefore
// not an abstention — it is a model that said it could answer and then grounded
// nothing, which is the shape every measured fabrication took. corpusRefusal is
// the abstention.
func corpusReply(claims ...askedClaim) string {
	if claims == nil {
		claims = []askedClaim{}
	}
	covered := coverageAnswers
	out, err := json.Marshal(askedAnswer{Coverage: &covered, Claims: &claims})
	if err != nil {
		panic(err)
	}
	return string(out)
}

// corpusRefusal renders the reply this site asks for when the passages do not
// cover the question: the verdict, and the sentence that says what they cover
// instead.
func corpusRefusal() string {
	declined := coverageDoesNotCover
	out, err := json.Marshal(askedAnswer{
		Coverage: &declined,
		Summary:  "These documents do not cover that.",
		Claims:   &[]askedClaim{},
	})
	if err != nil {
		panic(err)
	}
	return string(out)
}

// fixedLane answers whatever it was built with.
type fixedLane struct {
	text string
	err  error
	reqs []model.Request
}

func (l *fixedLane) Complete(_ context.Context, req model.Request) (model.Response, error) {
	l.reqs = append(l.reqs, req)
	if l.err != nil {
		return model.Response{}, l.err
	}
	return model.Response{Text: l.text}, nil
}

func corpusQuietLog() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func answeredState() knowledge.Readiness {
	return knowledge.Readiness{
		Outcome: crmcontracts.KnowledgeAnswerOutcomeKnowledgeAnswerOutcomeAnswered,
		Corpus:  crmcontracts.KnowledgeAnswerCorpus{Name: "How-to", TopicStatement: "How this product is operated."},
	}
}

// A claim whose quote is really in its passage survives.
func TestAClaimQuotingItsPassageIsKept(t *testing.T) {
	passages := askPassages()
	kept, err := GroundCorpusAnswer(corpusReply(askedClaim{
		Text:  "Messages are kept for 400 days.",
		ID:    passages[0].ChunkID.String(),
		Quote: "kept for 400 days",
	}), passages)
	if err != nil {
		t.Fatalf("ground: %v", err)
	}
	if len(kept.Claims) != 1 {
		t.Fatalf("kept %d claims, want 1", len(kept.Claims))
	}
	if kept.Claims[0].Text == nil || *kept.Claims[0].Text != "Messages are kept for 400 days." {
		t.Fatalf("the claim's sentence is %v", kept.Claims[0].Text)
	}
	// The citation has to point at something the reader can open.
	if kept.Claims[0].DocumentName != "operating-handbook.md" {
		t.Fatalf("the claim cites %q", kept.Claims[0].DocumentName)
	}
}

// The citation carries a RANGE, because the modal that follows it highlights the
// quote rather than dropping a cursor on its first character.
func TestAClaimCarriesWhereItsQuoteStartsAndEnds(t *testing.T) {
	passages := askPassages()
	passages[0].StartLine = 12
	kept, err := GroundCorpusAnswer(corpusReply(askedClaim{
		Text:  "Messages are kept for 400 days.",
		ID:    passages[0].ChunkID.String(),
		Quote: "kept for 400 days",
	}), passages)
	if err != nil {
		t.Fatalf("ground: %v", err)
	}
	if len(kept.Claims) != 1 {
		t.Fatalf("kept %d claims, want 1", len(kept.Claims))
	}
	// "Captured messages are " is 22 characters, and the quote is 17 long.
	assertClaimSpan(t, kept.Claims[0], 12, 23, 12, 40)
}

// A model may return a quote that only matches once whitespace is collapsed — a
// re-wrapped line. The claim survives on that basis, and the END has to be
// measured on the SAME form: taken from the raw quote instead, the highlight
// would run past the sentence by every space the collapse removed.
func TestARewrappedQuoteIsMeasuredOnTheFormThatMatched(t *testing.T) {
	passages := askPassages()
	passages[0].StartLine = 12
	kept, err := GroundCorpusAnswer(corpusReply(askedClaim{
		Text:  "Messages are kept for 400 days.",
		ID:    passages[0].ChunkID.String(),
		Quote: "kept for\n    400 days",
	}), passages)
	if err != nil {
		t.Fatalf("ground: %v", err)
	}
	if len(kept.Claims) != 1 {
		t.Fatalf("kept %d claims, want 1", len(kept.Claims))
	}
	// The same 17 characters as above: the newline and its indent are not part
	// of what the document holds.
	assertClaimSpan(t, kept.Claims[0], 12, 23, 12, 40)
}

// A passage that cannot place its own quote stamps NO part of the range. Half a
// range is a highlight over the wrong words, which is worse than none.
func TestAClaimThePassageCannotPlaceCarriesNoRange(t *testing.T) {
	passages := askPassages()
	kept, err := GroundCorpusAnswer(corpusReply(askedClaim{
		Text:  "Messages are kept for 400 days.",
		ID:    passages[0].ChunkID.String(),
		Quote: "kept for 400 days",
	}), passages)
	if err != nil {
		t.Fatalf("ground: %v", err)
	}
	if len(kept.Claims) != 1 {
		t.Fatalf("kept %d claims, want 1", len(kept.Claims))
	}
	got := kept.Claims[0]
	if got.Line != nil || got.Column != nil || got.EndLine != nil || got.EndColumn != nil {
		t.Fatalf("a passage with no start line stamped a range: %+v", got)
	}
}

// assertClaimSpan reads the four location fields as one range, because that is
// what they are to the reader following the citation.
func assertClaimSpan(t *testing.T, claim crmcontracts.KnowledgeClaim, line, column, endLine, endColumn int) {
	t.Helper()
	if claim.Line == nil || claim.Column == nil || claim.EndLine == nil || claim.EndColumn == nil {
		t.Fatalf("the claim carries no range: %+v", claim)
	}
	if *claim.Line != line || *claim.Column != column || *claim.EndLine != endLine || *claim.EndColumn != endColumn {
		t.Fatalf("the quote spans %d:%d..%d:%d; want %d:%d..%d:%d",
			*claim.Line, *claim.Column, *claim.EndLine, *claim.EndColumn, line, column, endLine, endColumn)
	}
}

// A paraphrased quote is dropped. This is the guardrail: a model that rewrites
// the passage into its own words has removed the only evidence a checker can
// use, whether or not the sentence happens to be true.
func TestAParaphrasedQuoteIsDropped(t *testing.T) {
	passages := askPassages()
	kept, err := GroundCorpusAnswer(corpusReply(askedClaim{
		Text:  "Messages are kept for 400 days.",
		ID:    passages[0].ChunkID.String(),
		Quote: "retained for four hundred days",
	}), passages)
	if err != nil {
		t.Fatalf("ground: %v", err)
	}
	if len(kept.Claims) != 0 {
		t.Fatalf("a paraphrased quote survived: %+v", kept)
	}
}

// Re-wrapping is not rewriting. A quote broken across lines still says what the
// document says, and failing it would drop correct claims for their whitespace.
func TestAQuoteReWrappedAcrossLinesIsStillVerbatim(t *testing.T) {
	passages := askPassages()
	kept, err := GroundCorpusAnswer(corpusReply(askedClaim{
		Text:  "Messages are kept for 400 days.",
		ID:    passages[0].ChunkID.String(),
		Quote: "kept for\n   400 days",
	}), passages)
	if err != nil {
		t.Fatalf("ground: %v", err)
	}
	if len(kept.Claims) != 1 {
		t.Fatalf("a re-wrapped quote was dropped")
	}
}

// Changed capitalisation IS a rewrite. The point of a verbatim quote is that a
// reader can find it in the file, and folding case would admit quotes the
// document does not contain.
func TestAQuoteWithChangedCapitalisationIsDropped(t *testing.T) {
	passages := askPassages()
	kept, err := GroundCorpusAnswer(corpusReply(askedClaim{
		Text:  "Messages are kept for 400 days.",
		ID:    passages[0].ChunkID.String(),
		Quote: "Kept For 400 Days",
	}), passages)
	if err != nil {
		t.Fatalf("ground: %v", err)
	}
	if len(kept.Claims) != 0 {
		t.Fatalf("a re-capitalised quote survived: %+v", kept)
	}
}

// An empty quote never matches. strings.Contains is true for the empty string
// against anything, and "nothing" is exactly what a model reaches for when it
// has no span to point at — so without this guard the guardrail switches itself
// off in the one case it exists for.
func TestAnEmptyQuoteIsDropped(t *testing.T) {
	passages := askPassages()
	kept, err := GroundCorpusAnswer(corpusReply(askedClaim{
		Text:  "Messages are kept for 400 days.",
		ID:    passages[0].ChunkID.String(),
		Quote: "   ",
	}), passages)
	if err != nil {
		t.Fatalf("ground: %v", err)
	}
	if len(kept.Claims) != 0 {
		t.Fatalf("an empty quote survived: %+v", kept)
	}
}

// A citation to a passage this call never saw is dropped. The schema's enum
// should make it unreachable; "should be unreachable" is not a guarantee about
// a provider's output.
func TestACitationOutsideTheRetrievedSetIsDropped(t *testing.T) {
	passages := askPassages()
	kept, err := GroundCorpusAnswer(corpusReply(askedClaim{
		Text:  "Messages are kept for 400 days.",
		ID:    ids.NewV7().String(),
		Quote: "kept for 400 days",
	}), passages)
	if err != nil {
		t.Fatalf("ground: %v", err)
	}
	if len(kept.Claims) != 0 {
		t.Fatalf("a claim citing an unseen passage survived: %+v", kept)
	}
}

// An answer with no surviving claim is not_covered, not an empty answered. An
// answer that cites nothing is an ungrounded one, not a short grounded one.
func TestAnAnswerWhoseClaimsAllFailIsNotCovered(t *testing.T) {
	passages := askPassages()
	lane := &fixedLane{text: corpusReply(askedClaim{
		Text:  "The Professional plan is 49 EUR per seat.",
		ID:    passages[0].ChunkID.String(),
		Quote: "49 EUR per seat",
	})}
	answer := AnswerCorpus(t.Context(), lane, answeredState(), "what does it cost", passages,
		string(textlang.English), corpusQuietLog())

	if answer.Outcome != crmcontracts.KnowledgeAnswerOutcomeKnowledgeAnswerOutcomeNotCovered {
		t.Fatalf("outcome = %q, want not_covered", answer.Outcome)
	}
	if answer.Claims != nil {
		t.Fatalf("a not_covered answer carries claims: %+v", answer.Claims)
	}
}

// With no lane, the answer is the passages themselves — quoted, cited, and
// honestly labelled. The grounded part of a grounded answer was never the prose.
// With no lane the passages still come back — but as unreviewed, because
// nothing read them. Calling it answered asserts that these passages answer the
// question, and retrieval cannot establish that: an uncovered question and a
// covered one score inside one range under a real embedding binding.
func TestWithNoLaneThePassagesComeBackUnreviewed(t *testing.T) {
	passages := askPassages()
	answer := AnswerCorpus(t.Context(), nil, answeredState(), "how long are messages kept", passages,
		string(textlang.English), corpusQuietLog())

	if answer.Outcome != crmcontracts.KnowledgeAnswerOutcomeKnowledgeAnswerOutcomeUnreviewed {
		t.Fatalf("outcome = %q, want unreviewed", answer.Outcome)
	}
	if answer.GeneratedBy != crmcontracts.WrittenByDeterministic {
		t.Fatalf("generated_by = %q, want deterministic", answer.GeneratedBy)
	}
	if answer.Claims == nil || len(*answer.Claims) != 1 {
		t.Fatalf("the deterministic answer carries %v claims", answer.Claims)
	}
	claim := (*answer.Claims)[0]
	if claim.Quote != askedPassageText {
		t.Fatalf("the claim quotes %q, want the passage itself", claim.Quote)
	}
	// Absent rather than empty: the contract says a deterministic claim carries
	// no text, and an empty string renders as a blank sentence rather than as
	// no sentence.
	if claim.Text != nil {
		t.Fatalf("a deterministic claim carries a written sentence: %q", *claim.Text)
	}
}

// A lane that fails degrades to the same passages rather than surfacing an
// error. The degrade is declared as unreviewed for the same reason a missing
// lane is: a lane that timed out read nothing.
func TestAFailedLaneDegradesToUnreviewedPassages(t *testing.T) {
	passages := askPassages()
	lane := &fixedLane{err: errors.New("the lane timed out")}
	answer := AnswerCorpus(t.Context(), lane, answeredState(), "how long are messages kept", passages,
		string(textlang.English), corpusQuietLog())

	if answer.Outcome != crmcontracts.KnowledgeAnswerOutcomeKnowledgeAnswerOutcomeUnreviewed {
		t.Fatalf("outcome = %q, want unreviewed", answer.Outcome)
	}
	if answer.GeneratedBy != crmcontracts.WrittenByDeterministic {
		t.Fatalf("generated_by = %q, want deterministic", answer.GeneratedBy)
	}
	if answer.Claims == nil || len(*answer.Claims) != 1 {
		t.Fatalf("a degraded answer carries %v claims", answer.Claims)
	}
}

// A refusal is never handed to the lane. Everything a refusal rests on was
// settled deterministically, and asking anyway would spend a model call to
// arrive at the answer already in hand.
func TestARefusalNeverReachesTheLane(t *testing.T) {
	lane := &fixedLane{text: corpusReply()}
	for _, outcome := range []crmcontracts.KnowledgeAnswerOutcome{
		crmcontracts.KnowledgeAnswerOutcomeKnowledgeAnswerOutcomeNotReady,
		crmcontracts.KnowledgeAnswerOutcomeKnowledgeAnswerOutcomeNotCovered,
		crmcontracts.KnowledgeAnswerOutcomeKnowledgeAnswerOutcomeRetrievalUnavailable,
		crmcontracts.KnowledgeAnswerOutcomeKnowledgeAnswerOutcomeUnreviewed,
	} {
		state := answeredState()
		state.Outcome = outcome
		answer := AnswerCorpus(t.Context(), lane, state, "anything", nil, string(textlang.English), corpusQuietLog())
		if answer.Outcome != outcome {
			t.Fatalf("%s became %s", outcome, answer.Outcome)
		}
		if answer.Claims != nil {
			t.Fatalf("%s carries claims", outcome)
		}
	}
	if len(lane.reqs) != 0 {
		t.Fatalf("a refusal made %d model call(s)", len(lane.reqs))
	}
}

// The citation enum is this call's own passage ids. A fixed enum would offer
// ids the call cannot resolve and withhold the ones it can, which is what makes
// an out-of-set citation structurally impossible rather than merely caught.
func TestTheRequestOffersOnlyThisCallsPassageIDs(t *testing.T) {
	passages := askPassages()
	req := CorpusAskRequest("how long are messages kept", passages, string(textlang.English))

	schemaText := string(req.ResponseSchema)
	if !strings.Contains(schemaText, passages[0].ChunkID.String()) {
		t.Fatal("the schema does not offer the passage this call retrieved")
	}
	// And the passage's own text reaches the model, or there is nothing to
	// quote from.
	if !strings.Contains(req.Messages[0].Content, askedPassageText) {
		t.Fatal("the prompt does not carry the passage text")
	}
	if !strings.Contains(req.Messages[0].Content, "how long are messages kept") {
		t.Fatal("the prompt does not carry the question")
	}
}

// The schema and the parser name the same keys.
//
// Three places spell each one — the schema's property map, its required list,
// and askedClaim's struct tags — and a key that disagreed between any two would
// be a field the model is asked for and the parser silently never reads. The
// symptom would be an answer that always has empty text, or always drops every
// claim, with nothing failing.
func TestTheReplySchemaAndTheParserAgreeOnEveryKey(t *testing.T) {
	req := CorpusAskRequest("anything", askPassages(), string(textlang.English))

	var doc struct {
		Properties struct {
			Claims struct {
				Items struct {
					Properties map[string]json.RawMessage `json:"properties"`
					Required   []string                   `json:"required"`
				} `json:"items"`
			} `json:"claims"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(req.ResponseSchema, &doc); err != nil {
		t.Fatalf("the reply schema is not the shape this test reads: %v", err)
	}
	claim := doc.Properties.Claims.Items

	// Every key the PARSER decodes must be offered by the schema, under exactly
	// the name the parser reads.
	for _, key := range []string{claimTextKey, claimIDKey, claimQuoteKey} {
		if _, offered := claim.Properties[key]; !offered {
			t.Fatalf("the parser reads %q and the schema does not offer it", key)
		}
	}
	// And the schema must offer nothing else: a property the parser ignores is
	// a field the model spends output on for no reader.
	if len(claim.Properties) != 3 {
		t.Fatalf("the schema offers %d properties, want the three the parser reads: %v",
			len(claim.Properties), claim.Properties)
	}
	// The struct tags are the third spelling, read from the type itself rather
	// than restated here.
	tags := map[string]bool{}
	for _, f := range reflect.VisibleFields(reflect.TypeOf(askedClaim{})) {
		tags[f.Tag.Get("json")] = true
	}
	for key := range claim.Properties {
		if !tags[key] {
			t.Fatalf("the schema offers %q and askedClaim has no field tagged for it", key)
		}
	}
	if len(claim.Required) != 3 {
		t.Fatalf("the schema requires %d keys, want all three", len(claim.Required))
	}
}

// The one outcome a reader could mistake for an answer is the one that must
// never be reached with a working lane: a lane that read the passages and wrote
// a surviving claim HAS reviewed them.
func TestAWrittenAnswerIsNeverUnreviewed(t *testing.T) {
	passages := askPassages()
	lane := &fixedLane{text: corpusReply(askedClaim{
		Text:  "Messages are kept for 400 days.",
		ID:    passages[0].ChunkID.String(),
		Quote: "kept for 400 days",
	})}
	answer := AnswerCorpus(t.Context(), lane, answeredState(), "how long are messages kept",
		passages, string(textlang.English), corpusQuietLog())

	if answer.Outcome != crmcontracts.KnowledgeAnswerOutcomeKnowledgeAnswerOutcomeAnswered {
		t.Fatalf("outcome = %q, want answered", answer.Outcome)
	}
	if answer.GeneratedBy != crmcontracts.WrittenByModel {
		t.Fatalf("generated_by = %q, want model", answer.GeneratedBy)
	}
}

// reAskingLane is the structured lane: it hands the validator its first answer
// and, when that is refused, answers again — which is what the real pipeline
// does with the refusal message in hand.
type reAskingLane struct {
	fixedLane
	validated int
	refused   error
	second    string
}

func (l *reAskingLane) CompleteValidated(
	_ context.Context, req model.Request, validate ai.Validator,
) (model.Response, error) {
	l.reqs = append(l.reqs, req)
	l.validated++
	l.refused = validate(l.text)
	if l.refused == nil {
		return model.Response{Text: l.text}, nil
	}
	return model.Response{Text: l.second}, nil
}

// A reply this site cannot read buys a re-ask rather than a silent degrade to
// the passages, and the refusal the model is shown is the answer path's own.
func TestAnUnreadableReplyIsReAskedThroughTheSitesOwnRefusal(t *testing.T) {
	passages := askPassages()
	lane := &reAskingLane{
		fixedLane: fixedLane{text: "here you go: {claims: none}"},
		second: corpusReply(askedClaim{
			Text:  "Messages are kept for 400 days.",
			ID:    passages[0].ChunkID.String(),
			Quote: "kept for 400 days",
		}),
	}
	answer := AnswerCorpus(t.Context(), lane, answeredState(), "how long are messages kept",
		passages, string(textlang.English), corpusQuietLog())

	if lane.validated != 1 {
		t.Fatalf("the validated lane was used %d times, want once", lane.validated)
	}
	if lane.refused == nil {
		t.Fatal("the validator accepted a reply GroundCorpusAnswer refuses, so nothing was re-asked")
	}
	if answer.Outcome != crmcontracts.KnowledgeAnswerOutcomeKnowledgeAnswerOutcomeAnswered {
		t.Fatalf("outcome = %q, want answered from the second reply", answer.Outcome)
	}
	if answer.GeneratedBy != crmcontracts.WrittenByModel {
		t.Fatalf("generated_by = %q, want model", answer.GeneratedBy)
	}
	if answer.Claims == nil || len(*answer.Claims) != 1 {
		t.Fatalf("the re-asked answer carries %v claims, want the one it wrote", answer.Claims)
	}
}

// The empty answer is the one this site asks for when the passages do not cover
// the question, so it must not be refused: re-asking there would spend a call
// pushing a model that obeyed the prompt to disobey it.
func TestAnAnswerWithNoClaimsIsNotReAsked(t *testing.T) {
	passages := askPassages()
	lane := &reAskingLane{fixedLane: fixedLane{text: corpusReply()}, second: "never sent"}
	answer := AnswerCorpus(t.Context(), lane, answeredState(), "what does it cost",
		passages, string(textlang.English), corpusQuietLog())

	if lane.validated != 1 {
		t.Fatalf("the validated lane was used %d times, want once", lane.validated)
	}
	if lane.refused != nil {
		t.Fatalf("an empty answer was refused: %v", lane.refused)
	}
	if answer.Outcome != crmcontracts.KnowledgeAnswerOutcomeKnowledgeAnswerOutcomeNotCovered {
		t.Fatalf("outcome = %q, want not_covered", answer.Outcome)
	}
}

// A reply carrying none of this site's keys decodes to the same zero value an
// empty answer would, and the two mean opposite things: one says the passages
// do not cover the question, the other never answered it. Reading the second as
// the first would report not_covered — a confident statement about the corpus —
// on a reply nobody could read.
func TestAReplyWithoutAClaimsKeyIsRefusedRatherThanReadAsEmpty(t *testing.T) {
	passages := askPassages()
	for _, reply := range []string{
		`{}`,
		`{"claims":null}`,
		`{"answer":"I could not find that in the documents."}`,
	} {
		if _, err := GroundCorpusAnswer(reply, passages); err == nil {
			t.Errorf("%s was read as an empty answer rather than refused", reply)
		}
	}
}

// And the empty answer itself still is not refused, which is the whole reason
// the two shapes had to be told apart rather than both rejected.
func TestADeclaredRefusalIsNotRefused(t *testing.T) {
	answer, err := GroundCorpusAnswer(
		`{"coverage":"does_not_answer","summary":"These documents do not say how to do that.","claims":[]}`,
		askPassages())
	if err != nil {
		t.Fatalf("a declared refusal was refused: %v", err)
	}
	if answer.Covered {
		t.Fatal("does_not_answer was read as covered")
	}
	if len(answer.Claims) != 0 {
		t.Fatalf("a refusal kept %d claim(s)", len(answer.Claims))
	}
	if answer.Summary != "These documents do not say how to do that." {
		t.Fatalf("the refusal's summary is %q, and it is the only thing the reader is shown", answer.Summary)
	}
}

// A model that declares the passages do not answer the question and attaches
// grounded sentences anyway has contradicted itself. The verdict is the half to
// trust: it is the judgement this site asks for, and the sentences are the half
// every measured fabrication came from.
func TestClaimsAreDroppedWhenTheModelDeclaredItCannotAnswer(t *testing.T) {
	passages := askPassages()
	answer, err := GroundCorpusAnswer(
		`{"coverage":"does_not_answer","summary":"Not covered.","claims":[{"text":"Messages are kept for 400 days.","id":"`+
			passages[0].ChunkID.String()+`","quote":"kept for 400 days"}]}`, passages)
	if err != nil {
		t.Fatalf("ground: %v", err)
	}
	if len(answer.Claims) != 0 {
		t.Fatalf("a refusal carried %d claim(s) through, so a reader is shown evidence for an answer that was declined", len(answer.Claims))
	}
}

// A reply with no coverage key is refused rather than read as a refusal: an
// absent verdict is a reply that never made the decision, and reading it as
// "does not answer" would report a confident statement about the corpus.
func TestAReplyWithNoCoverageKeyIsRefused(t *testing.T) {
	if _, err := GroundCorpusAnswer(`{"claims":[]}`, askPassages()); err == nil {
		t.Fatal("a reply carrying no coverage key was accepted")
	}
}

// The refusal reaches the model: a reply the site cannot read buys the same
// re-ask a malformed one does, rather than settling for not_covered.
func TestAReplyWithoutAClaimsKeyIsReAsked(t *testing.T) {
	passages := askPassages()
	lane := &reAskingLane{
		fixedLane: fixedLane{text: `{}`},
		second: corpusReply(askedClaim{
			Text:  "Messages are kept for 400 days.",
			ID:    passages[0].ChunkID.String(),
			Quote: "kept for 400 days",
		}),
	}
	answer := AnswerCorpus(t.Context(), lane, answeredState(), "how long are messages kept",
		passages, string(textlang.English), corpusQuietLog())

	if lane.refused == nil {
		t.Fatal("a reply with no claims key was accepted, so nothing was re-asked")
	}
	if answer.Outcome != crmcontracts.KnowledgeAnswerOutcomeKnowledgeAnswerOutcomeAnswered {
		t.Fatalf("outcome = %q, want answered from the second reply", answer.Outcome)
	}
}

// corpusAnswered renders a reply that DECLARES the passages answer the question
// and carries a summary asserting it — the shape whose claims are worth taking
// away, because the sentence survives them.
func corpusAnswered(summary string, claims ...askedClaim) string {
	covered := coverageAnswers
	out, err := json.Marshal(askedAnswer{Coverage: &covered, Summary: summary, Claims: &claims})
	if err != nil {
		panic(err)
	}
	return string(out)
}

// The two not_covered answers are not the same answer, and the difference is
// the summary.
//
// A DECLARED refusal keeps its sentence: the model read the passages, said they
// do not answer the question, and that sentence is the only thing telling the
// reader what the documents cover instead.
//
// An answer whose claims all failed the quote check loses it: the sentence
// asserts an answer, every piece of evidence meant to hold it up is gone, and
// nothing else on the page ever checked it. Printing it would put a fabrication
// on screen under the one outcome a reader is entitled to trust.
//
// Both arms live in one test on purpose — conflating them is the defect, so a
// test that cannot tell them apart would not have caught it.
func TestOnlyADeclaredRefusalKeepsItsSentence(t *testing.T) {
	passages := askPassages()
	const refusalSentence = "These documents do not cover that."
	for _, tc := range []struct {
		name        string
		reply       string
		wantSummary string
	}{
		{
			name:        "the model declared it cannot answer",
			reply:       corpusRefusal(),
			wantSummary: refusalSentence,
		},
		{
			name: "the model declared an answer and grounded none of it",
			reply: corpusAnswered("Messages are kept for 30 days.", askedClaim{
				Text:  "Messages are kept for 30 days.",
				ID:    passages[0].ChunkID.String(),
				Quote: "kept for 30 days",
			}),
			wantSummary: "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lane := &fixedLane{text: tc.reply}
			answer := AnswerCorpus(t.Context(), lane, answeredState(), "how long are messages kept",
				passages, string(textlang.English), corpusQuietLog())

			if answer.Outcome != crmcontracts.KnowledgeAnswerOutcomeKnowledgeAnswerOutcomeNotCovered {
				t.Fatalf("outcome = %q, want not_covered", answer.Outcome)
			}
			if answer.Claims != nil {
				t.Errorf("a not_covered answer carries claims: %+v", answer.Claims)
			}
			// The model DID read the passages either way, and the reader is
			// told so: a refusal it wrote is not the degrade of a missing lane.
			if answer.GeneratedBy != crmcontracts.WrittenByModel {
				t.Errorf("generated_by = %q, want model", answer.GeneratedBy)
			}
			switch {
			case tc.wantSummary == "" && answer.Summary != nil:
				t.Errorf("the summary survived its evidence: %q", *answer.Summary)
			case tc.wantSummary != "" && answer.Summary == nil:
				t.Error("the refusal lost the one sentence that says what these documents do cover")
			case tc.wantSummary != "" && *answer.Summary != tc.wantSummary:
				t.Errorf("summary = %q, want %q", *answer.Summary, tc.wantSummary)
			}
		})
	}
}

// A grounded answer keeps its summary, so the drop above is the ungrounded case
// and not the summary never arriving at all.
// A dropped claim slides every later one up a place, so the numbers in the
// writer's summary stop meaning what it meant: a sentence marked [2] opens what
// used to be [3], and the reader has no way to tell. The prose goes; the claims
// stay, which is the shape an answer with no summary already has.
func TestASummaryGoesWhenTheQuoteCheckRenumbersTheClaims(t *testing.T) {
	passages := askPassages()
	covered := coverageAnswers
	reply, err := json.Marshal(askedAnswer{
		Coverage: &covered,
		Summary:  "Kept for 400 days. [1] Purged nightly. [2]",
		Claims: &[]askedClaim{
			{Text: "A sentence nothing holds up.", ID: passages[0].ChunkID.String(), Quote: "no passage says this"},
			{Text: "Messages are kept for 400 days.", ID: passages[0].ChunkID.String(), Quote: "kept for 400 days"},
		},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	answer := AnswerCorpus(t.Context(), &fixedLane{text: string(reply)}, answeredState(),
		"how long are messages kept", passages, "en", slog.New(slog.DiscardHandler))

	if answer.Outcome != crmcontracts.KnowledgeAnswerOutcomeKnowledgeAnswerOutcomeAnswered {
		t.Fatalf("outcome %v, want answered — one claim did survive", answer.Outcome)
	}
	if answer.Summary != nil {
		t.Fatalf("the summary survived a renumbering: %q — its [2] now opens the claim that was [1]", *answer.Summary)
	}
	if answer.Claims == nil || len(*answer.Claims) != 1 {
		t.Fatalf("the surviving claim was not kept")
	}
}

func TestAGroundedAnswerKeepsItsSummary(t *testing.T) {
	passages := askPassages()
	lane := &fixedLane{text: corpusAnswered("Messages are kept for 400 days. [1]", askedClaim{
		Text:  "Messages are kept for 400 days.",
		ID:    passages[0].ChunkID.String(),
		Quote: "kept for 400 days",
	})}
	answer := AnswerCorpus(t.Context(), lane, answeredState(), "how long are messages kept",
		passages, string(textlang.English), corpusQuietLog())

	if answer.Outcome != crmcontracts.KnowledgeAnswerOutcomeKnowledgeAnswerOutcomeAnswered {
		t.Fatalf("outcome = %q, want answered", answer.Outcome)
	}
	if answer.Summary == nil || *answer.Summary != "Messages are kept for 400 days. [1]" {
		t.Fatalf("summary = %v, want the sentence the model wrote", answer.Summary)
	}
}

// A failed lane says WHY in the log, and the reason is all it says: the reply it
// would otherwise carry quotes a third party's uploaded document.
func TestAFailedLaneLogsTheReasonAndNotTheReply(t *testing.T) {
	var logged strings.Builder
	log := slog.New(slog.NewTextHandler(&logged, &slog.HandlerOptions{Level: slog.LevelWarn}))
	passages := askPassages()
	answer := AnswerCorpus(t.Context(), &fixedLane{err: errors.New("the lane timed out")}, answeredState(),
		"how long are messages kept", passages, string(textlang.English), log)

	if answer.Outcome != crmcontracts.KnowledgeAnswerOutcomeKnowledgeAnswerOutcomeUnreviewed {
		t.Fatalf("outcome = %q, want unreviewed", answer.Outcome)
	}
	line := logged.String()
	// Without this line a declared degrade is indistinguishable from a lane
	// nobody wired: the outcome reads the same either way.
	if !strings.Contains(line, "the lane timed out") {
		t.Errorf("the log does not carry the reason the lane fell back:\n%s", line)
	}
	if strings.Contains(line, askedPassageText) {
		t.Errorf("the log carries the uploaded document's own words:\n%s", line)
	}
}

// The embedder an installation with no embed lane gets names no binding, which
// is what makes Retrieve answer retrieval_unavailable — a statement about the
// installation, never about the question.
func TestAnInstallationWithNoEmbedLaneNamesNoBinding(t *testing.T) {
	identity, dims := unboundEmbedder{}.EmbedIdentity()
	if identity != "" || dims != 0 {
		t.Fatalf("EmbedIdentity() = %q, %d; want the empty identity retrieval refuses on", identity, dims)
	}
	// And it never invents a vector: a plausible one would be ranked against
	// the corpus and answer from nonsense instead of failing.
	vectors, err := unboundEmbedder{}.Embed(t.Context(), model.EmbedRequest{Inputs: []string{"anything"}})
	if err == nil {
		t.Fatalf("the unbound embedder answered %v instead of refusing", vectors)
	}
	if len(vectors.Vectors) != 0 {
		t.Errorf("the refusal came with %d vector(s)", len(vectors.Vectors))
	}
}

// Until WithCorpusAsk runs, the endpoint says so rather than searching: an
// installation that composed no retrieval cannot answer, and pretending to
// would be worse. After it runs, the same endpoint is the engine's.
func TestTheAskEndpointIsAnsweredOnlyOnceWithCorpusAskHasRun(t *testing.T) {
	id := crmcontracts.Id(ids.NewV7())
	ask := func(s *Server) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/v1/corpora/"+id.String()+"/ask", strings.NewReader("{"))
		r.Header.Set("Content-Type", "application/json")
		s.AskCorpus(w, r, id)
		return w
	}

	var bare Server
	if code := ask(&bare).Code; code != http.StatusNotImplemented {
		t.Fatalf("an unwired ask answered %d, want 501", code)
	}

	var wired Server
	// A nil embedder is the installation with no embed lane, and the option
	// must substitute the unbound one rather than leave a nil to be called.
	WithCorpusAsk(nil, nil, corpusQuietLog())(&wired, nil)
	// A body the decoder refuses, so the wiring is proved without a database:
	// anything the store would answer needs one, and the decoder's refusal can
	// only come from the engine's own handler.
	if code := ask(&wired).Code; code != http.StatusUnprocessableEntity {
		t.Fatalf("the wired ask answered %d for an unreadable body, want the engine's own 422", code)
	}
}
