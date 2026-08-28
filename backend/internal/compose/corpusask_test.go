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
	"reflect"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
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

// corpusReply renders a model answer over the given passage ids.
func corpusReply(claims ...askedClaim) string {
	out, err := json.Marshal(askedAnswer{Claims: claims})
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
		Outcome: crmcontracts.KnowledgeAnswerOutcomeAnswered,
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
	if len(kept) != 1 {
		t.Fatalf("kept %d claims, want 1", len(kept))
	}
	if kept[0].Text == nil || *kept[0].Text != "Messages are kept for 400 days." {
		t.Fatalf("the claim's sentence is %v", kept[0].Text)
	}
	// The citation has to point at something the reader can open.
	if kept[0].DocumentName != "operating-handbook.md" {
		t.Fatalf("the claim cites %q", kept[0].DocumentName)
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
	if len(kept) != 0 {
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
	if len(kept) != 1 {
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
	if len(kept) != 0 {
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
	if len(kept) != 0 {
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
	if len(kept) != 0 {
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

	if answer.Outcome != crmcontracts.KnowledgeAnswerOutcomeNotCovered {
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

	if answer.Outcome != crmcontracts.KnowledgeAnswerOutcomeUnreviewed {
		t.Fatalf("outcome = %q, want unreviewed", answer.Outcome)
	}
	if answer.GeneratedBy != crmcontracts.Deterministic {
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

	if answer.Outcome != crmcontracts.KnowledgeAnswerOutcomeUnreviewed {
		t.Fatalf("outcome = %q, want unreviewed", answer.Outcome)
	}
	if answer.GeneratedBy != crmcontracts.Deterministic {
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
		crmcontracts.KnowledgeAnswerOutcomeNotReady,
		crmcontracts.KnowledgeAnswerOutcomeNotCovered,
		crmcontracts.KnowledgeAnswerOutcomeRetrievalUnavailable,
		crmcontracts.KnowledgeAnswerOutcomeUnreviewed,
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

	if answer.Outcome != crmcontracts.KnowledgeAnswerOutcomeAnswered {
		t.Fatalf("outcome = %q, want answered", answer.Outcome)
	}
	if answer.GeneratedBy != crmcontracts.Model {
		t.Fatalf("generated_by = %q, want model", answer.GeneratedBy)
	}
}
