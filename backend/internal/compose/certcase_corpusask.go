// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The certification case for corpus_ask — a bounded document corpus asked in
// free text.
//
// It certifies the SHIPPED path: the request comes from CorpusAskRequest and
// the reply is read by GroundCorpusAnswer, because that checker is what stands
// between a reader and a sentence grounded in nothing. A case that rebuilt
// either would measure a copy, and a copy stays green through the change that
// breaks the original.
//
// What the expectation MEANS here: which passages a correct answer must rest
// on. Not the wording — the sentences are prose, and pinning them would fail a
// good answer for choosing different words. What production cannot guarantee,
// and this therefore measures, is whether the model answered from the passages
// it was given or from what it already believed.
//
// Two shapes of scenario matter, and both are represented in the corpus. One is
// a question the passages DO answer, where the measurement is which passages
// were cited. The other is a question they do not — where the only correct
// answer is no claims at all, and a model that writes a plausible paragraph
// anyway is the failure this whole endpoint exists to prevent.
//
// The fixture names its passages by LABEL. Prepare mints the ids, so an id in
// the reply is one the model was handed rather than one the corpus author could
// have written into the expected answer.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gradionhq/margince/backend/internal/compose/aitasks"
	"github.com/gradionhq/margince/backend/internal/modules/ai"
	"github.com/gradionhq/margince/backend/internal/modules/knowledge"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/textlang"
	"github.com/gradionhq/margince/backend/internal/shared/ports/model"
)

// corpusAskFixture is one question and the passages retrieval handed it.
//
// The passages arrive already retrieved because retrieval is deterministic and
// certified by its own suite. What is graded here is only what a model does
// with passages it has been given — including doing nothing with them.
type corpusAskFixture struct {
	Question string                    `json:"question"`
	Passages []corpusAskPassageFixture `json:"passages"`
}

type corpusAskPassageFixture struct {
	Label    string `json:"label"`
	Document string `json:"document"`
	Text     string `json:"text"`
}

type corpusAskCases struct{}

func (corpusAskCases) Site() aitasks.Site {
	return aitasks.Site{Task: ai.TaskCorpusAsk, Variant: "corpus_ask", Kind: ai.SiteKindOneShot}
}

// Prepare turns one question and its passages into a runnable case, minting an
// id per labelled passage.
//
// An expected answer of `[]` is meaningful and is NOT the same as an omitted
// one: it is the scenario asserting that a correct answer cites nothing. That
// distinction is why the field is decoded into a pointer.
//
//nolint:ireturn // PreparedCase IS the seam: one implementation per site behind the one interface the cert lane runs.
func (corpusAskCases) Prepare(fixture, expected json.RawMessage) (aitasks.PreparedCase, error) {
	var f corpusAskFixture
	if err := json.Unmarshal(fixture, &f); err != nil {
		return nil, fmt.Errorf("corpus_ask/corpus_ask: the fixture is not the shape this site takes: %w", err)
	}
	if strings.TrimSpace(f.Question) == "" {
		return nil, fmt.Errorf("corpus_ask/corpus_ask: the fixture asks no question")
	}
	if len(f.Passages) == 0 {
		return nil, fmt.Errorf(
			"corpus_ask/corpus_ask: the fixture supplies no passages, and production never asks the lane without them")
	}
	var want []string
	if err := json.Unmarshal(expected, &want); err != nil {
		return nil, fmt.Errorf(
			"corpus_ask/corpus_ask: the expected answer is not a list of passage labels the reply must cite: %w", err)
	}
	passages, label, err := corpusAskPassages(f)
	if err != nil {
		return nil, fmt.Errorf("corpus_ask/corpus_ask: %w", err)
	}
	for _, name := range want {
		if _, known := label[name]; !known {
			return nil, fmt.Errorf(
				"corpus_ask/corpus_ask: the expected answer names passage %q, which the fixture does not supply — an unreachable expectation grades every reply wrong", name)
		}
	}
	return &corpusAskCase{question: f.Question, passages: passages, label: label, expected: want}, nil
}

// corpusAskPassages builds the retrieved passages production would hand the
// lane, minting one id per labelled passage.
//
// Similarity is set above any default floor because these passages have already
// been retrieved: the floor is a deterministic gate upstream, and a fixture that
// pretended otherwise would grade the model on a decision it never makes.
func corpusAskPassages(f corpusAskFixture) ([]knowledge.Passage, map[string]string, error) {
	passages := make([]knowledge.Passage, 0, len(f.Passages))
	label := map[string]string{}
	for _, p := range f.Passages {
		if err := refuseUnnameable(p.Label, "passage", label); err != nil {
			return nil, nil, err
		}
		if strings.TrimSpace(p.Text) == "" {
			return nil, nil, fmt.Errorf("passage %q carries no text", p.Label)
		}
		chunkID := ids.NewV7()
		label[p.Label] = chunkID.String()
		passages = append(passages, knowledge.Passage{
			ChunkID:      chunkID,
			DocumentID:   ids.NewV7(),
			DocumentName: p.Document,
			Text:         p.Text,
			Similarity:   0.9,
		})
	}
	return passages, label, nil
}

// corpusAskCase certifies one written answer over one set of passages.
type corpusAskCase struct {
	question string
	passages []knowledge.Passage
	label    map[string]string
	expected []string
}

// Run issues the one request this site sends, through the production request
// builder — including its per-call citation enum, which is part of what the
// model is actually constrained by.
func (c *corpusAskCase) Run(ctx context.Context, completer aitasks.Completer) (aitasks.Trace, error) {
	// English, pinned, rather than the installation's base language: a
	// certification record grades a fixed corpus, and a score that moved with a
	// settings row would not be comparable between two installations or across
	// one that changed its mind. The rule is PRESENT in the graded request for
	// the same reason — production sends one, so a case that left it out would
	// grade a prompt the product does not send.
	req := CorpusAskRequest(c.question, c.passages, string(textlang.English))
	trace := aitasks.Trace{Requests: []model.Request{req}}
	resp, err := completer.Complete(ctx, req)
	if err != nil {
		return trace, fmt.Errorf("corpus_ask/corpus_ask: %w", err)
	}
	trace.Output = resp.Text
	return trace, nil
}

// Evaluate runs the production quote check and asks whether what survived
// cites the passages the scenario says the answer rests on.
func (c *corpusAskCase) Evaluate(trace aitasks.Trace) aitasks.Outcome {
	kept, err := GroundCorpusAnswer(trace.Output, c.passages)
	if err != nil {
		// Unparseable is invalid rather than wrong: production composes the
		// deterministic answer here, and grading it as a wrong answer would
		// blame the model's reasoning for its formatting.
		return aitasks.Outcome{Result: aitasks.OutcomeInvalid, Detail: err.Error()}
	}
	cited := map[string]bool{}
	for _, claim := range kept {
		cited[claim.ChunkId.String()] = true
	}
	// A scenario expecting NOTHING is the abstention case, and it is graded the
	// other way round: any surviving claim is a failure. This is the shape the
	// endpoint exists to get right — a model handed passages that do not answer
	// the question must return nothing rather than write a paragraph that
	// sounds like an answer.
	if len(c.expected) == 0 {
		if len(kept) > 0 {
			return aitasks.Outcome{
				Result: aitasks.OutcomeWrongAnswer,
				Detail: fmt.Sprintf("answered with %d claim(s) from passages that do not cover the question", len(kept)),
			}
		}
		return aitasks.Outcome{Result: aitasks.OutcomeAccepted}
	}
	var missing []string
	for _, name := range c.expected {
		if !cited[c.label[name]] {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return aitasks.Outcome{
			Result: aitasks.OutcomeWrongAnswer,
			Detail: "never cited: " + strings.Join(missing, ", "),
		}
	}
	return aitasks.Outcome{Result: aitasks.OutcomeAccepted}
}
