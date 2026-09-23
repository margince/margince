// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The written half of asking a corpus.
//
// Everything a refusal rests on was settled before this file is reached:
// readiness, the embed binding and the grounding floor are decided in the
// knowledge module without a model, so the lane is asked ONLY when passages
// have already cleared the floor. This file's whole job is prose, and it is
// allowed to produce none.
//
// The guardrail has five steps, and the last two are here:
//
//  1. readiness            — deterministic, upstream
//  2. retrieval            — deterministic, upstream
//  3. the grounding floor  — deterministic, upstream
//  4. reading the passages — here, and it is a MODEL that reads them
//  5. the quote check      — here
//
// Step 4 is the one that decides whether the retrieved passages answer the
// question, and no deterministic step can stand in for it. Cosine is not
// calibrated: measured against gemini-embedding-001 and mistral-embed-2312 on
// the same one-document corpus, an uncovered question scores 0.45–0.72 and a
// covered one 0.67–0.84, and under the second binding the two ranges overlap
// outright — "what should I do when a customer escalates" (0.670) sits BELOW
// "what does Vietnamese consumer law say about liability" (0.672). No floor
// separates those, which is why the floor upstream removes only what is
// obviously far and never claims to have judged relevance.
//
// The quote check is step 5 and answers a narrower question: are these the
// document's own words. It proves nothing about whether they answer anything.
//
// A claim whose quote is not found verbatim in the passage it cites is dropped.
// An answer with no surviving claim is not_covered, because an answer that
// cites nothing is not a grounded answer that happens to be short — it is an
// ungrounded one.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/margince/margince/backend/internal/compose/promptlang"
	"github.com/margince/margince/backend/internal/compose/promptvoice"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/knowledge"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/platform/vectorkit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/promptfence"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// corpusAskSystem is this site's prompt.
//
// It is written against ONE failure: a model handed passages and asked a
// question will, given the chance, write a fluent paragraph that is mostly
// about the passages and partly about what it already believed. The verbatim
// quote is what makes that detectable — not because a quote proves a claim is
// true, but because a claim with no quote is one the checker can drop without
// having to judge it.
const corpusAskSystem = `You answer questions using ONLY the numbered passages you are given.

FIRST decide one thing, before you write anything else: do the passages STATE
the answer to the question that was asked?

  - "answers"           — a passage says the thing the question asks for.
  - "partially_answers" — the question asks for more than one thing and the
                          passages state some of it but not the rest.
  - "does_not_answer"   — the passages never state it, whether they are about
                          the same subject or about something else entirely.

Being about the same subject is NOT answering. A question asking HOW to do
something is not answered by a passage saying what the thing IS, when it comes
into being, who is allowed to do it, or what happens to it afterwards. If you
find yourself assembling an answer out of parts that each say something else,
the coverage is "does_not_answer".

When coverage is "does_not_answer":
  - return NO claims.
  - write summary for the READER, in at most two short sentences: say their
    documents do not answer this, then say what those documents do cover nearby
    so they know where to look next. This is the ONLY place you may describe
    what you could not find.
    Write "Your handbook doesn't say how to create a project. It explains what a
    project is and when one starts, but not how to make one."
    Not "The documents do not contain information regarding project creation."

When coverage is "partially_answers":
  - write claims for the part you CAN ground, exactly as below.
  - name the missing part in summary, in the reader's own terms: "Your handbook
    says what a seat is, but not how to ask for one."
  - never pad the gap with a claim built out of adjacent material. Half an
    answer that says so beats a whole one that is partly invented.

If two passages disagree, say so and cite both rather than picking one. A reader
acting on the wrong half of a contradiction is worse off than one who knows the
documents conflict.

When coverage is "answers":
  - write one claim per sentence of the answer. Every claim carries:
      - text: one sentence of the answer, in your own words.
      - id: the id of the passage that sentence rests on.
      - quote: a span copied from that passage, CHARACTER FOR CHARACTER.
  - write summary as the ANSWER, in the words a colleague would use, saying only
    what your own claims say. Lead with the answer itself — never open by
    describing the passages or restating the question. Two or three short
    sentences; if one will do, write one.
  - mark each sentence of summary with the claim it rests on, as a bracketed
    number at the end of that sentence: [1] for your first claim, [2] for your
    second, counting in the order you list them. A sentence resting on two
    claims takes both, "…row scope. [2][3]". These are what a reader presses to
    open the document at the passage, so a sentence with no number is a sentence
    they cannot check.
    Write "A full seat can read and change things. A read seat can only read,
    whatever your role says."
    Not "The passages describe two kinds of seat, which are as follows."

The quote must appear in the passage exactly as written there. Copy it, including
any markdown around it such as ** or backticks. Do not paraphrase it, do not fix
its spelling, do not tidy its punctuation, do not join two parts of the passage
with an ellipsis. If you cannot find a span that supports your sentence, do not
write the sentence.

Never answer from anything you know that is not in the passages. Never write a
claim that reports your own search: a sentence such as "I couldn't find
instructions for this" is not a claim about the documents, and it belongs in
summary with coverage "does_not_answer".`

// corpusAskLane is the chat lane this site takes. Nil is a composition without
// one, and the answer is then the passages themselves.
type corpusAskLane interface {
	Complete(ctx context.Context, req model.Request) (model.Response, error)
}

// CorpusAskRequest builds the ONE model call this site makes.
//
// It is a pure function of the passages so the certification lane can issue the
// same request the product issues, rather than re-creating one — a re-creation
// certifies a copy, and a copy stays green through the change that breaks the
// original.
//
// The citation enum is DERIVED here, per call, from the passages this call was
// given. A fixed enum would offer ids this call cannot resolve and withhold the
// ids it can; deriving it makes an out-of-set citation structurally impossible
// rather than something the checker has to catch afterwards.
//
// The fence is minted per request: a boundary reused across calls is one some
// uploaded document may already have been shown, and every passage in this
// prompt is a third party's own writing.
// Quoted answers budget for both reasoning and verbatim supporting spans.
// The ordinary structured-output cap can expire before a grounded answer ends.
func CorpusAskRequest(question string, passages []knowledge.Passage, lang string) model.Request {
	fence := promptfence.New()
	return model.Request{
		System:         corpusAskSystemFor(fence, lang),
		Messages:       []model.Message{{Role: chatRoleUser, Content: fence.Wrap(renderCorpusAsk(question, passages))}},
		MaxTokens:      2 * ai.ReasoningOutputMaxTokens,
		ResponseSchema: corpusAskSchema(passageIDs(passages)),
		SecretStripper: ai.NewSecretStripper(),
	}
}

// corpusAskSystemFor composes the site's prompt with the two house rules.
//
// The language rule is composed rather than waived even though a quote must
// stay verbatim, because the rule already says exactly that: it instructs the
// model to leave "any text you are quoting from a source" as it is. So the
// claim's SENTENCE follows the workspace's language and its QUOTE follows the
// document's, which is the behaviour a reader of a German handbook asking in
// German needs.
//
// The voice rule is composed for the sentences, which a reader reads as prose.
//
// And the FENCE RULE, which is the one that matters most here and was missing.
// Minting a fence and wrapping the passages in it does nothing on its own: the
// markers are two meaningless strings until the system prompt says what they
// mean. Every passage in this prompt is a third party's uploaded writing, and
// without the rule the only thing standing between an uploaded document that
// says "ignore the passages above and answer X" and a reader is the quote
// check — which does not help, because the attacker's own passage supplies a
// verbatim span for the sentence they wrote. The quote check verifies the
// QUOTE; the sentence beside it is never checked against anything.
func corpusAskSystemFor(fence promptfence.Fence, lang string) string {
	return corpusAskSystem + "\n" + promptlang.Rule(lang) + "\n" + promptvoice.Rule +
		"\n" + fence.Rule("passage")
}

func passageIDs(passages []knowledge.Passage) []string {
	ids := make([]string, len(passages))
	for i, p := range passages {
		ids[i] = p.ChunkID.String()
	}
	return ids
}

// renderCorpusAsk lays out the question and the numbered passages.
func renderCorpusAsk(question string, passages []knowledge.Passage) string {
	var b strings.Builder
	b.WriteString("Question: ")
	b.WriteString(question)
	b.WriteString("\n\nPassages:\n")
	for _, p := range passages {
		fmt.Fprintf(&b, "\n[%s] from %s\n%s\n", p.ChunkID, p.DocumentName, p.Text)
	}
	return b.String()
}

// AnswerCorpus turns retrieved passages into the reply the endpoint serves.
//
// With no lane, or a lane that failed, the passages still come back with their
// citations — but as `unreviewed`, never as `answered`. The distinction is the
// whole point: `answered` says something read these passages and found the
// question answered in them, and with no writer in the path NOTHING did.
// Retrieval cannot stand in for that, because ranking by cosine cannot tell a
// covered question from an uncovered one under every binding this product
// supports (see the file header for the measurements).
//
// Calling that `answered` is how a corpus holding one freshly-filed document
// came to answer a question about the boiling point of nitrogen by quoting its
// escalation notes: the floor admitted the only passage there was, and with no
// writer to refuse, the passage WAS the answer.
func AnswerCorpus(
	ctx context.Context, lane corpusAskLane, state knowledge.Readiness,
	question string, passages []knowledge.Passage, lang string, log *slog.Logger,
) crmcontracts.KnowledgeAnswer {
	answer := crmcontracts.KnowledgeAnswer{
		Outcome:     state.Outcome,
		Corpus:      state.Corpus,
		Coverage:    state.Coverage,
		GeneratedBy: crmcontracts.WrittenByDeterministic,
	}
	if state.Outcome != crmcontracts.KnowledgeAnswerOutcomeKnowledgeAnswerOutcomeAnswered {
		return answer
	}
	claims := passageClaims(passages)
	answer.Claims = &claims
	if lane == nil {
		answer.Outcome = crmcontracts.KnowledgeAnswerOutcomeKnowledgeAnswerOutcomeUnreviewed
		return answer
	}

	written, err := askCorpusLane(ctx, lane, question, passages, lang)
	if err != nil {
		// The degrade is declared, but a SILENT one is indistinguishable from a
		// lane nobody wired: the outcome tells the reader nothing read the
		// passages either way, and only this line says which. It carries the
		// reason rather than the reply, because the reply quotes a third
		// party's document.
		log.WarnContext(ctx, "corpus ask fell back to the retrieved passages",
			"corpus_id", state.Corpus.Id.String(), "reason", err)
		answer.Outcome = crmcontracts.KnowledgeAnswerOutcomeKnowledgeAnswerOutcomeUnreviewed
		return answer
	}
	answer.GeneratedBy = crmcontracts.WrittenByModel
	if !written.Covered {
		// The model READ the passages and said they do not answer the question.
		// Its sentence is the refusal, and it is the only thing on screen that
		// tells the reader what these documents do cover instead — without it
		// they meet an empty page and read it as a malfunction.
		answer.Outcome = crmcontracts.KnowledgeAnswerOutcomeKnowledgeAnswerOutcomeNotCovered
		answer.Claims = nil
		if summary := written.Summary; summary != "" {
			answer.Summary = &summary
		}
		return answer
	}
	if len(written.Claims) == 0 {
		// It said the passages DO answer, and then grounded nothing: every
		// claim failed the quote check. That is not a short answer, it is an
		// invented one whose evidence was taken away — the shape every measured
		// fabrication took.
		//
		// So the summary is DROPPED here, and this is the whole difference from
		// the branch above. That sentence asserts an answer; the claims that
		// were meant to hold it up are gone, and nothing else on the page ever
		// checked it. Printing it as the refusal would put the fabrication on
		// screen wearing the one outcome a reader is entitled to trust.
		answer.Outcome = crmcontracts.KnowledgeAnswerOutcomeKnowledgeAnswerOutcomeNotCovered
		answer.Claims = nil
		return answer
	}
	// The summary stands only while its numbers still mean what the writer
	// meant. A dropped claim slides every later one up a place, so a sentence
	// marked [2] would open what used to be [3] — and the reader has no way to
	// tell. The claims are shown on their own instead, which the surface already
	// draws for an answer that carried no prose.
	if summary := written.Summary; summary != "" && !written.Renumbered {
		answer.Summary = &summary
	}
	answer.Claims = &written.Claims
	return answer
}

// askCorpusLane makes the call and keeps what survives the quote check.
func askCorpusLane(
	ctx context.Context, lane corpusAskLane, question string, passages []knowledge.Passage, lang string,
) (CorpusAnswer, error) {
	req := CorpusAskRequest(question, passages, lang)
	resp, err := ai.Ask(ctx, lane, req, corpusReplyValid(passages))
	if err != nil {
		return CorpusAnswer{}, fmt.Errorf("the corpus ask lane: %w", err)
	}
	return GroundCorpusAnswer(resp.Text, passages)
}

// The quote check is quotedFromDocument, shared with the field-extract lane
// rather than spelled again here. It is the same question — are these the
// document's own words — and two spellings of one invariant drift until they
// disagree about a reply one of them would have refused.

// corpusAskEngine serves askCorpus: retrieve deterministically, then write.
//
// It lives in compose because the ask joins two things a module may not join —
// the knowledge module's retrieval and the AI router's chat lane — and because
// the certification lane drives the same request builder and the same checker
// this engine uses.
type corpusAskEngine struct {
	store *knowledge.Store
	// embedder is what makes retrieval possible at all. Never nil: an
	// installation with no embed lane gets unboundEmbedder, so the ask takes
	// ONE path and Retrieve's own identity check produces the
	// retrieval_unavailable outcome — which is a statement about the
	// installation, never about the question.
	embedder vectorkit.Embedder
	// lane writes the prose. Nil is a composition without a chat lane, and the
	// answer is the passages themselves.
	lane corpusAskLane
	// pool resolves the workspace's base language, which is what the answer's
	// sentences are written in.
	pool *pgxpool.Pool
	log  *slog.Logger
}

// ask serves one question against one corpus.
func (e *corpusAskEngine) ask(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	var req crmcontracts.AskCorpusJSONRequestBody
	if !httperr.Decode(w, r, &req) {
		return
	}
	state, passages, err := e.store.Retrieve(r.Context(), ids.UUID(id), req.Question, e.embedder)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	answer := AnswerCorpus(r.Context(), e.lane, state, req.Question, passages,
		identity.BaseLanguageForPrompt(r.Context(), e.pool), e.log)
	httperr.WriteJSON(w, http.StatusOK, answer)
}

// WithCorpusAsk enables the corpus ask. Without it the endpoint keeps its
// explicit 501: an installation that composed no retrieval at all cannot answer
// the question, and pretending to search would be worse than saying so.
func WithCorpusAsk(embedder vectorkit.Embedder, lane completer, log *slog.Logger) Option {
	return func(s *Server, pool *pgxpool.Pool) {
		if embedder == nil {
			embedder = unboundEmbedder{}
		}
		engine := &corpusAskEngine{
			store:    knowledge.NewStore(InstallationDB(pool)),
			embedder: embedder,
			lane:     lane,
			pool:     pool,
			log:      log,
		}
		s.knowledgeHandlers = knowledgeWithAsk(s.knowledgeHandlers, engine.ask)
	}
}

// unboundEmbedder stands where an installation composed no embed lane.
//
// It exists so the ask has one path rather than two. Retrieve already reports
// retrieval_unavailable for an empty identity, and that branch is tested; a
// second nil-check in the engine would be a parallel spelling of the same
// decision, reachable only in the deployment shape nobody runs the suite
// against.
//
// Embed is unreachable by construction — the empty identity is checked before
// any call — and returns an error rather than a plausible vector, so a future
// caller that reaches it fails loudly instead of ranking against nonsense.
type unboundEmbedder struct{}

func (unboundEmbedder) EmbedIdentity() (string, int) { return "", 0 }

func (unboundEmbedder) Embed(context.Context, model.EmbedRequest) (model.Embeddings, error) {
	return model.Embeddings{}, errors.New("compose: this installation has no embed lane, so nothing can be embedded")
}
