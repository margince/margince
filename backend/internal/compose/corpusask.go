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
// The guardrail has four steps, and the last two are here:
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
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	openapi_types "github.com/oapi-codegen/runtime/types"

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
	"github.com/margince/margince/backend/internal/shared/schema"
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

Write one claim per sentence of the answer. Every claim carries:
  - text: one sentence of the answer, in your own words.
  - id: the id of the passage that sentence rests on.
  - quote: a span copied from that passage, CHARACTER FOR CHARACTER.

The quote must appear in the passage exactly as written there. Do not
paraphrase it, do not fix its spelling, do not join two parts of the passage
with an ellipsis. If you cannot find a span that supports your sentence, do not
write the sentence.

If the passages do not answer the question, return no claims at all. An empty
answer is correct and expected. Never answer from anything you know that is not
in the passages, and never say the passages are insufficient — just return
nothing.`

// corpusAskLane is the chat lane this site takes. Nil is a composition without
// one, and the answer is then the passages themselves.
type corpusAskLane interface {
	Complete(ctx context.Context, req model.Request) (model.Response, error)
}

// The reply's field names. They appear in the schema's property map, in the
// required-key list beside it, and in the struct tags below; a key that
// disagreed between any two of those would be a field the model is asked for
// and the parser never reads.
//
// Held by: TestTheReplySchemaAndTheParserAgreeOnEveryKey (corpusask_test.go) —
// it reads the generated schema and requires every key the parser decodes to
// appear in it, so a fourth spelling introduced anywhere fails there.
const (
	claimTextKey  = "text"
	claimIDKey    = "id"
	claimQuoteKey = "quote"
)

// askedClaim is one claim as the model returns it.
type askedClaim struct {
	Text  string `json:"text"`
	ID    string `json:"id"`
	Quote string `json:"quote"`
}

type askedAnswer struct {
	// A POINTER so an absent key is distinguishable from an empty list. They
	// mean opposite things here: `{"claims":[]}` is the answer this site asks
	// for when the passages do not cover the question, while `{}` — or any
	// reply carrying none of this site's keys, which decodes to exactly the
	// same zero value — is a reply in a shape this site does not take. Reading
	// the second as the first would report "not covered", a confident statement
	// about the corpus, on a reply that never answered the question.
	Claims *[]askedClaim `json:"claims"`
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
func CorpusAskRequest(question string, passages []knowledge.Passage, lang string) model.Request {
	fence := promptfence.New()
	return model.Request{
		System:         corpusAskSystemFor(fence, lang),
		Messages:       []model.Message{{Role: chatRoleUser, Content: fence.Wrap(renderCorpusAsk(question, passages))}},
		MaxTokens:      ai.ReasoningOutputMaxTokens,
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
// The voice rule is composed for the sentences, which a person reads as prose.
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

// corpusAskSchema is the reply shape, with this call's own passage ids as the
// citation enum.
func corpusAskSchema(ids []string) json.RawMessage {
	return schema.Must(schema.Object(
		map[string]schema.Node{
			"claims": schema.Array(schema.Object(
				map[string]schema.Node{
					claimTextKey:  schema.String().Describe("One sentence of the answer, in your own words."),
					claimIDKey:    schema.Enum(ids...).Describe("The passage this sentence rests on."),
					claimQuoteKey: schema.String().Describe("A span copied from that passage, character for character."),
				},
				claimTextKey, claimIDKey, claimQuoteKey,
			)),
		},
		"claims",
	))
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
		GeneratedBy: crmcontracts.Deterministic,
	}
	if state.Outcome != crmcontracts.KnowledgeAnswerOutcomeAnswered {
		return answer
	}
	claims := passageClaims(passages)
	answer.Claims = &claims
	if lane == nil {
		answer.Outcome = crmcontracts.KnowledgeAnswerOutcomeUnreviewed
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
		answer.Outcome = crmcontracts.KnowledgeAnswerOutcomeUnreviewed
		return answer
	}
	if len(written) == 0 {
		// The model read the passages and found nothing in them that answers
		// the question — or wrote only claims the quote check dropped. Either
		// way this is not_covered, and it is the honest answer: an answer that
		// cites nothing is an ungrounded one, not a short grounded one.
		answer.Outcome = crmcontracts.KnowledgeAnswerOutcomeNotCovered
		answer.Claims = nil
		return answer
	}
	answer.Claims = &written
	answer.GeneratedBy = crmcontracts.Model
	return answer
}

// askCorpusLane makes the call and keeps what survives the quote check.
func askCorpusLane(
	ctx context.Context, lane corpusAskLane, question string, passages []knowledge.Passage, lang string,
) ([]crmcontracts.KnowledgeClaim, error) {
	req := CorpusAskRequest(question, passages, lang)
	var resp model.Response
	var err error
	if structured, ok := lane.(validatedBrain); ok {
		resp, err = structured.CompleteValidated(ctx, req, corpusReplyValid(passages))
	} else {
		resp, err = lane.Complete(ctx, req)
	}
	if err != nil {
		return nil, fmt.Errorf("the corpus ask lane: %w", err)
	}
	return GroundCorpusAnswer(resp.Text, passages)
}

// corpusReplyValid is the retry predicate, and it is the site's OWN read of the
// reply rather than a looser shape check: the model is shown the refusal the
// answer path would have raised, which is the only message that names the fault.
//
// It refuses exactly what GroundCorpusAnswer refuses, and no more. A reply whose
// claims all fail the quote check is NOT refused: the prompt asks for no claims
// when the passages do not cover the question, so a re-ask there would push a
// model that answered correctly to answer again.
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
func GroundCorpusAnswer(replyText string, passages []knowledge.Passage) ([]crmcontracts.KnowledgeClaim, error) {
	var parsed askedAnswer
	if err := json.Unmarshal([]byte(ai.Unfence(replyText)), &parsed); err != nil {
		return nil, fmt.Errorf("the corpus ask reply is not the shape this site takes: %w", err)
	}
	if parsed.Claims == nil {
		return nil, errors.New(`the corpus ask reply carries no "claims" key: an answer that cites nothing ` +
			`is written as {"claims": []}`)
	}
	byID := make(map[string]knowledge.Passage, len(passages))
	for _, p := range passages {
		byID[p.ChunkID.String()] = p
	}
	var kept []crmcontracts.KnowledgeClaim
	for _, c := range *parsed.Claims {
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
	return kept, nil
}

// The quote check is quotedFromDocument, shared with the field-extract lane
// rather than spelled again here. It is the same question — are these the
// document's own words — and two spellings of one invariant drift until they
// disagree about a reply one of them would have refused.

// locateClaim stamps where in the document the quote begins, when the passage
// can say.
//
// The quote is located by its RAW text rather than the trimmed or
// whitespace-collapsed form, because the offset has to be an offset into the
// document's real bytes — collapsing runs of spaces would shift every column
// after the first one that collapsed. A quote the passage cannot locate leaves
// both fields absent: a line number pointing at the wrong line is worse than
// none, and the whole value of a citation is that following it lands you on the
// sentence.
func locateClaim(claim *crmcontracts.KnowledgeClaim, p knowledge.Passage, quote string) {
	line, column := p.Locate(quote)
	if line == 0 {
		// A model may return a quote that only matches once whitespace is
		// collapsed — a re-wrapped line. The claim survived the check on that
		// basis, so try the collapsed text too rather than dropping a location
		// the reader could have used.
		line, column = p.Locate(collapseSpace(quote))
	}
	if line == 0 {
		return
	}
	claim.Line = &line
	claim.Column = &column
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
