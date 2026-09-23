// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The reply half of asking a corpus: the shape the model answers in, and the
// reading of it that decides what a contact is shown.
//
// That file builds the ONE request and assembles the answer; this one owns the
// reply's vocabulary, so the schema the model is given and the parser that reads
// it sit beside each other and cannot drift apart unnoticed.
//
// WHY COVERAGE IS A FIELD AND NOT AN ABSENCE.
//
// The first version of this site asked for no claims when the passages did not
// answer the question — an empty list, and nothing else. That reads well and
// fails in the field, because an absence is not a decision anybody made. A
// model handed eight passages that are ALL about the subject it was asked about
// has to answer by declining to write, at exactly the moment every other
// instinct is pushing it to be useful. Measured against the shipped handbook,
// it did not decline: asked how to create a project, it wrote "You create a
// project while you're still in the selling phase" over a passage saying a
// project is born while you are still selling — a fabricated instruction
// resting on a real quote.
//
// So coverage is now asked for FIRST and explicitly. "I don't know" has its own
// slot, the model has to fill one in either way, and a refusal is something it
// states rather than something it achieves by staying silent.
//
// WHY THE QUOTE CHECK COULD NEVER HAVE CAUGHT THIS.
//
// The check verifies that a claim's QUOTE is the document's own words. It never
// reads the SENTENCE beside it, and the sentence is where the invention lives.
// Both fabrications measured against the real handbook carried genuine quotes;
// one of them quoted a raw markdown table row out of a permissions grid, whose
// trailing "**no**" belonged to a column the sentence never mentioned. No
// tightening of the quote rule reaches that, which is why the fix is a decision
// and not a stricter check.

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/knowledge"
	"github.com/margince/margince/backend/internal/shared/schema"
)

// The reply's field names. They appear in the schema's property map, in the
// required-key list beside it, and in the struct tags below; a key that
// disagreed between any two of those would be a field the model is asked for
// and the parser never reads.
//
// Held by: TestTheReplySchemaAndTheParserAgreeOnEveryKey (corpusask_test.go),
// which reads the GENERATED schema and requires every key the parser decodes to
// appear in it — the answer's three and the claim's three alike, so a fourth
// spelling introduced anywhere fails there.
const (
	claimTextKey     = "text"
	claimIDKey       = "id"
	claimQuoteKey    = "quote"
	answerCoverKey   = "coverage"
	answerSummaryKey = "summary"
	answerClaimsKey  = "claims"
)

// What the model may say about coverage. Closed: a value not listed here is a
// verdict nothing downstream maps to an outcome.
//
// PARTLY is its own word because a compound question is the common shape and
// binary coverage forces a bad answer to it. Asked "what is a seat and how do I
// request one", a handbook that defines seats and never covers requesting one
// leaves a model choosing between refusing what it can answer and answering what
// it cannot. Both are wrong, and which one it picks is a coin toss the reader
// pays for. Given the third option it answers the half it has and says so.
//
// It is COVERED for the reader: grounded claims exist, so the answer stands on
// evidence and the summary names the gap. The parser therefore treats it exactly
// as `answers` — the value earns its place in the prompt, by giving the model
// somewhere honest to put a half-answer, not in a branch here.
const (
	coverageAnswers      = "answers"
	coveragePartly       = "partially_answers"
	coverageDoesNotCover = "does_not_answer"
)

// askedClaim is one claim as the model returns it.
type askedClaim struct {
	Text  string `json:"text"`
	ID    string `json:"id"`
	Quote string `json:"quote"`
}

type askedAnswer struct {
	// A POINTER so an absent key is distinguishable from an empty value. A
	// reply carrying none of this site's keys decodes to exactly the zero
	// value, and reading that as a decision would report a confident verdict
	// on a reply that never answered the question.
	Coverage *string       `json:"coverage"`
	Summary  string        `json:"summary"`
	Claims   *[]askedClaim `json:"claims"`
}

// CorpusAnswer is what the model said, after the quote check.
//
// Covered and Claims are separate because they fail apart: a model may declare
// coverage and then ground nothing, and the two facts want different words on
// screen.
type CorpusAnswer struct {
	// Covered is the model's own verdict: did these passages state the answer.
	Covered bool
	// Summary is the reader's sentence — the plain-language answer when covered,
	// and the plain-language refusal when not. It is NOT independently grounded
	// and never stands in for a claim: when covered it may say only what the
	// claims below it say, and when not covered it asserts nothing about the
	// corpus except that it does not cover the question.
	Summary string
	// Claims are the grounded sentences, each one's quote verified.
	Claims []crmcontracts.KnowledgeClaim
	// Renumbered is true when the quote check dropped a claim, so the positions
	// the writer's summary points at are no longer the positions that survived.
	//
	// It matters because the summary marks each sentence with the NUMBER of the
	// claim it rests on, and those numbers are the model's own count of what it
	// wrote. Drop the second of three and the third slides into its place: the
	// sentence still says [2], and [2] now opens a passage about something else
	// — a citation pointing at the wrong evidence, which is the failure this
	// whole surface exists to prevent.
	Renumbered bool
}

// corpusAskSchema is the reply shape, with this call's own passage ids as the
// citation enum.
//
// coverage is listed FIRST and required. Structured output is generated in
// order, so a model that must emit the verdict before the claims has decided
// coverage before it starts writing — rather than writing first and labelling
// what it wrote.
func corpusAskSchema(ids []string) json.RawMessage {
	return schema.Must(schema.Object(
		map[string]schema.Node{
			answerCoverKey: schema.Enum(coverageAnswers, coveragePartly, coverageDoesNotCover).
				Describe("Whether the passages STATE the answer to the question asked."),
			answerSummaryKey: schema.String().
				Describe("One or two plain sentences for the reader: the answer, or what the documents do not cover."),
			answerClaimsKey: schema.Array(schema.Object(
				map[string]schema.Node{
					claimTextKey:  schema.String().Describe("One sentence of the answer, in your own words."),
					claimIDKey:    schema.Enum(ids...).Describe("The passage this sentence rests on."),
					claimQuoteKey: schema.String().Describe("A span copied from that passage, character for character."),
				},
				claimTextKey, claimIDKey, claimQuoteKey,
			)),
		},
		answerCoverKey, answerSummaryKey, answerClaimsKey,
	))
}

// corpusReplyValid is the retry predicate, and it is the site's OWN read of the
// reply rather than a looser shape check: the model is shown the refusal the
// answer path would have raised, which is the only message that names the fault.
//
// It refuses exactly what GroundCorpusAnswer refuses, and no more. A reply that
// declares does_not_answer is NOT refused — that is the answer this site asks
// for when the passages do not cover the question, and re-asking there would
// push a model that judged correctly to judge again until it caved.
func corpusReplyValid(passages []knowledge.Passage) ai.Validator {
	return func(text string) error {
		_, err := GroundCorpusAnswer(text, passages)
		return err
	}
}

// GroundCorpusAnswer parses a reply and keeps only the claims whose quote is
// actually in the passage they cite.
//
// Exported because the certification lane reads a reply with the SAME checker
// production uses. A cert that re-implemented this would measure a copy, and
// the copy stays green through the change that breaks the original.
func GroundCorpusAnswer(replyText string, passages []knowledge.Passage) (CorpusAnswer, error) {
	var parsed askedAnswer
	if err := json.Unmarshal([]byte(ai.Unfence(replyText)), &parsed); err != nil {
		return CorpusAnswer{}, fmt.Errorf("the corpus ask reply is not the shape this site takes: %w", err)
	}
	if parsed.Coverage == nil {
		return CorpusAnswer{}, fmt.Errorf(
			"the corpus ask reply carries no %q key: a reply must say whether the passages answer the "+
				"question, as %q, %q or %q",
			answerCoverKey, coverageAnswers, coveragePartly, coverageDoesNotCover)
	}
	summary := strings.TrimSpace(parsed.Summary)
	switch *parsed.Coverage {
	case coverageDoesNotCover:
		// The claims are dropped unread. A model that declared the passages do
		// not answer the question and attached grounded sentences anyway has
		// contradicted itself, and the verdict is the half to trust: it is the
		// judgement this site asked for, and the sentences are the half every
		// measured failure came from.
		return CorpusAnswer{Covered: false, Summary: summary}, nil
	case coverageAnswers, coveragePartly:
	default:
		return CorpusAnswer{}, fmt.Errorf(
			"the corpus ask reply says coverage %q, which is none of %q, %q or %q",
			*parsed.Coverage, coverageAnswers, coveragePartly, coverageDoesNotCover)
	}
	if parsed.Claims == nil {
		return CorpusAnswer{}, errors.New(`the corpus ask reply carries no "claims" key: an answer that cites ` +
			`nothing is written as coverage "does_not_answer"`)
	}
	kept := groundClaims(*parsed.Claims, passages)
	return CorpusAnswer{
		Covered:    true,
		Summary:    summary,
		Claims:     kept,
		Renumbered: len(kept) != len(*parsed.Claims),
	}, nil
}

// groundClaims keeps the claims whose quote survives the check, stamping each
// with where in its document the quote begins and ends.
func groundClaims(asked []askedClaim, passages []knowledge.Passage) []crmcontracts.KnowledgeClaim {
	byID := make(map[string]knowledge.Passage, len(passages))
	for _, p := range passages {
		byID[p.ChunkID.String()] = p
	}
	var kept []crmcontracts.KnowledgeClaim
	for _, c := range asked {
		p, ok := byID[c.ID]
		if !ok {
			// The schema's enum should make this unreachable; it is checked
			// anyway because "should be unreachable" is not a guarantee about
			// a provider's output, and a citation to a passage this call never
			// saw is the exact failure the enum exists to prevent.
			continue
		}
		if !quotedFromDocument(p.Text, c.Quote) {
			continue
		}
		text := strings.TrimSpace(c.Text)
		claim := crmcontracts.KnowledgeClaim{
			ChunkId:      openapi_types.UUID(p.ChunkID),
			DocumentId:   openapi_types.UUID(p.DocumentID),
			DocumentName: p.DocumentName,
			Quote:        strings.TrimSpace(c.Quote),
		}
		if text != "" {
			claim.Text = &text
		}
		locateClaim(&claim, p, c.Quote)
		kept = append(kept, claim)
	}
	return kept
}

// locateClaim stamps where in the document the quote begins and ends, when the
// passage can say.
//
// The quote is located by its RAW text rather than the trimmed or
// whitespace-collapsed form, because the offset has to be an offset into the
// document's real bytes — collapsing runs of spaces would shift every column
// after the first one that collapsed. A quote the passage cannot locate leaves
// ALL four fields absent: a line number pointing at the wrong line is worse than
// none, and the whole value of a citation is that following it lands you on the
// sentence.
func locateClaim(claim *crmcontracts.KnowledgeClaim, p knowledge.Passage, quote string) {
	span := p.Locate(quote)
	if span.Line == 0 {
		// A model may return a quote that only matches once whitespace is
		// collapsed — a re-wrapped line. The claim survived the check on that
		// basis, so try the collapsed text too rather than dropping a location
		// the reader could have used. The end comes back from the SAME call as
		// the start, so it measures whichever form actually matched: an end
		// taken from the other one would run the highlight past the quote by
		// every space the collapse removed.
		span = p.Locate(collapseSpace(quote))
	}
	if span.Line == 0 {
		return
	}
	claim.Line = &span.Line
	claim.Column = &span.Column
	claim.EndLine = &span.EndLine
	claim.EndColumn = &span.EndColumn
}

// passageClaims renders the retrieved passages as the deterministic answer:
// each passage IS a claim, quoting itself, with no sentence written over it.
//
// The Text field is deliberately absent rather than empty. The contract says a
// claim's text is absent when generated_by is deterministic — then the quote
// stands on its own and no prose was written — and an empty string would render
// as a blank sentence rather than as no sentence.
func passageClaims(passages []knowledge.Passage) []crmcontracts.KnowledgeClaim {
	claims := make([]crmcontracts.KnowledgeClaim, len(passages))
	for i, p := range passages {
		claims[i] = crmcontracts.KnowledgeClaim{
			ChunkId:      openapi_types.UUID(p.ChunkID),
			DocumentId:   openapi_types.UUID(p.DocumentID),
			DocumentName: p.DocumentName,
			Quote:        strings.TrimSpace(p.Text),
		}
		// The whole passage IS the quote here, so it begins where the passage
		// begins — no search needed, and none possible: locating a string
		// inside itself always answers offset zero.
		if p.StartLine > 0 {
			line, column := p.StartLine, 1
			claims[i].Line = &line
			claims[i].Column = &column
		}
	}
	return claims
}
