// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package knowledge

// The deterministic half of asking a corpus: everything decided WITHOUT a
// model.
//
// Two of the outcomes are settled here in full, and they are the two that are
// NOT about the question:
//
//   not_ready              — about the CORPUS. It is still ingesting, holds
//                            nothing, or is being re-embedded.
//   retrieval_unavailable  — about the INSTALLATION. No embed lane is bound, so
//                            nothing was searched at all.
//
// Collapsing them is the failure this endpoint exists to avoid. Answering
// "your documents do not cover this" for a corpus that is merely half-ingested
// is an affirmative false claim about documents that DO cover it.
//
// not_covered is about the QUESTION, and this file cannot settle it in general.
// It reports not_covered where nothing reached the model to be read: an empty
// question, a question the lane could answer only with a zero vector, or a
// ranking the grounding floor emptied. Whether a corpus that DID hand over
// passages covers the question is decided downstream by the writer that reads
// them, because ranking cannot decide it: cosine is not calibrated, and
// under mistral-embed-2312 a covered question against a one-document corpus
// measured 0.670 while an unrelated one measured 0.672. The floor below removes
// what is obviously far; it does not tell those two apart, and no value of it
// could.

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/vectorkit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// retrieveLimit is how many passages an ask ranks up.
//
// Eight rather than a larger number because every one of them is sent to the
// model, and a model handed forty passages writes an answer that quotes the
// most fluent rather than the most relevant. The floor already removes the
// ungrounded ones; this bounds what a grounded answer is built from.
const retrieveLimit = 8

// Passage is one retrieved span, with what a citation needs to point at it.
type Passage struct {
	ChunkID      ids.UUID
	DocumentID   ids.UUID
	DocumentName string
	Text         string
	Similarity   float64
	// StartLine is the 1-based line of the document this span begins on, or 0
	// for a passage written before the column existed. A citation says nothing
	// about where rather than guessing.
	StartLine int
}

// Locate answers where in the document a span of this passage's text begins:
// the 1-based line and column a reader would open the file at.
//
// Both zero when the passage does not know its own start line, or when the span
// is not in its text. A location that points at the wrong line is worse than
// none — the whole value of a citation is that following it lands you on the
// sentence.
//
// The column is in CHARACTERS, not bytes: a person counts across a line by what
// they can see, and a byte offset would put them past the mark on any line with
// an accent in it.
func (p Passage) Locate(span string) (line, column int) {
	if p.StartLine == 0 {
		return 0, 0
	}
	at := strings.Index(p.Text, span)
	if at < 0 {
		return 0, 0
	}
	before := p.Text[:at]
	line = p.StartLine + strings.Count(before, "\n")
	if nl := strings.LastIndex(before, "\n"); nl >= 0 {
		// A later line of the passage: the column is measured from that
		// line's own start.
		return line, utf8.RuneCountInString(before[nl+1:]) + 1
	}
	// The passage's FIRST line, which may itself begin mid-line in the
	// document — a span cut at the width ceiling does. Nothing here knows how
	// far in that was, so the column is measured from the passage's start and
	// is a lower bound on the true one.
	return line, utf8.RuneCountInString(before) + 1
}

// Readiness is what the deterministic pass concluded, and the counts a screen
// shows beside it.
type Readiness struct {
	Outcome  crmcontracts.KnowledgeAnswerOutcome
	Coverage crmcontracts.KnowledgeCoverage
	Corpus   crmcontracts.KnowledgeAnswerCorpus
}

// Retrieve settles the ask as far as it can be settled without a model.
//
// It returns Outcome=answered together with the passages ONLY when the corpus
// was searched in full and something cleared its grounding floor. Every other
// outcome comes back with no passages, because a passage below the floor must
// not reach the model: a claim citing one would pass the quote check and
// re-open exactly the hole the floor exists to close.
func (s *Store) Retrieve(ctx context.Context, corpusID ids.UUID, question string, e vectorkit.Embedder) (Readiness, []Passage, error) {
	if err := auth.Require(ctx, "knowledge_corpus", principal.ActionRead); err != nil {
		return Readiness{}, nil, err
	}
	identity, dims := e.EmbedIdentity()
	state, err := s.readinessOf(ctx, corpusID, identity)
	if err != nil {
		return Readiness{}, nil, err
	}
	// Answered BEFORE readiness is reported, because "no lane is bound" is a
	// statement about the installation that outranks anything about this
	// corpus: a workspace told its corpus is not ready would go and look at
	// documents that are perfectly fine.
	if identity == "" {
		state.Outcome = crmcontracts.KnowledgeAnswerOutcomeRetrievalUnavailable
		return state, nil, nil
	}
	if state.Outcome != crmcontracts.KnowledgeAnswerOutcomeAnswered {
		return state, nil, nil
	}

	vec, err := s.questionVector(ctx, question, e, dims)
	if err != nil {
		return state, nil, err
	}
	if vec == nil {
		// Nothing to rank. Every cosine against the zero vector is NaN and
		// `ORDER BY sim DESC` sorts NaN FIRST, so ranking would put arbitrary
		// passages at the top of an answer.
		//
		// Reported as not_covered, and the choice is worth stating because the
		// alternative was considered. not_covered is a statement about the
		// QUESTION, which is exactly right here: a question with no words in it
		// is a question the corpus cannot answer, and the reader who typed
		// spaces gets the topic statement quoted back, which tells them what to
		// ask instead. The one case this reads slightly wrong for — a real
		// question the lane could only answer with a zero vector — is a lane
		// malfunction, and it is already the outcome that spends nothing and
		// claims nothing.
		state.Outcome = crmcontracts.KnowledgeAnswerOutcomeNotCovered
		return state, nil, nil
	}

	// Readiness is settled AGAIN here, and this second read is the
	// authoritative one. The first decides whether to spend a model call
	// embedding the question at all; it cannot be the answer, because a
	// transaction may not be held across that network call.
	//
	// What can change in between is exactly what makes an outcome dishonest. A
	// document uploaded after the first read leaves the corpus mid-ingest, and
	// retrieval over the old embedded subset would find nothing and report
	// not_covered — blaming the question for a corpus that is simply not
	// finished. A document DELETED in between leaves passages ranked out of
	// rows that no longer exist, and the answer would cite a file the corpus
	// does not hold.
	//
	// Both disappear by reading readiness, the passages and the floor in ONE
	// transaction, which is what this is.
	var grounded []Passage
	err = s.tx(ctx, func(tx pgx.Tx) (err error) {
		state, grounded, err = groundedIn(ctx, tx, corpusID, vec, identity)
		return err
	})
	if err != nil {
		return state, nil, err
	}
	if state.Outcome != crmcontracts.KnowledgeAnswerOutcomeAnswered {
		return state, nil, nil
	}
	if len(grounded) == 0 {
		state.Outcome = crmcontracts.KnowledgeAnswerOutcomeNotCovered
		return state, nil, nil
	}
	return state, grounded, nil
}

// groundedIn settles readiness, ranks, and keeps what clears the floor — all
// inside ONE transaction, which is the point: see Retrieve.
func groundedIn(
	ctx context.Context, tx pgx.Tx, corpusID ids.UUID, vec []float32, identity string,
) (Readiness, []Passage, error) {
	state, err := readinessIn(ctx, tx, corpusID, identity)
	if err != nil {
		return state, nil, err
	}
	if state.Outcome != crmcontracts.KnowledgeAnswerOutcomeAnswered {
		return state, nil, nil
	}
	passages, err := rankIn(ctx, tx, corpusID, vec, identity)
	if err != nil {
		return state, nil, err
	}
	floor, err := groundingFloorIn(ctx, tx, corpusID)
	if err != nil {
		return state, nil, err
	}
	var grounded []Passage
	for _, p := range passages {
		if p.Similarity >= floor {
			grounded = append(grounded, p)
		}
	}
	return state, grounded, nil
}

// readinessOf answers whether this corpus can be asked at all, and gathers the
// counts and the corpus identity every outcome carries.
//
// The rule, in this order:
//
//	ready ⇔ no document queued or running
//	      ∧ at least one document done
//	      ∧ every live passage embedded AT THE LIVE IDENTITY
//
// The emphasis is the whole point of taking identity as a parameter. The
// corpus's own coverage counts any passage carrying any identity, which is the
// right number for a screen — but a corpus whose vectors all sit under a
// SUPERSEDED binding would then read as fully embedded, be called ready, and
// retrieve nothing, because the ask's identity filter excludes every one of
// them. The honest answer there is not_ready, and it is only reachable by
// asking the same question the retrieval will ask.
//
// A FAILED document leaves the denominator entirely. One bad file must not make
// the whole corpus unanswerable forever — its own row already says it failed
// and why, which is where that fact belongs.
func (s *Store) readinessOf(ctx context.Context, corpusID ids.UUID, identity string) (Readiness, error) {
	var state Readiness
	err := s.tx(ctx, func(tx pgx.Tx) (err error) {
		state, err = readinessIn(ctx, tx, corpusID, identity)
		return err
	})
	return state, err
}

// readinessIn is readinessOf inside a transaction the caller holds, so the
// retrieval below can share one snapshot with it.
func readinessIn(ctx context.Context, tx pgx.Tx, corpusID ids.UUID, identity string) (Readiness, error) {
	var state Readiness
	row, err := readCorpus(ctx, tx, corpusID, storekit.LiveOnly)
	if err != nil {
		return state, err
	}
	corpus := row.wire()
	state.Coverage = corpus.Coverage
	state.Corpus = crmcontracts.KnowledgeAnswerCorpus{
		Id:             corpus.Id,
		Name:           corpus.Name,
		TopicStatement: corpus.TopicStatement,
	}
	var inFlight, done int
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FILTER (WHERE ingest_status IN ('queued', 'running')),
		        count(*) FILTER (WHERE ingest_status = 'done')
		   FROM knowledge_document
		  WHERE corpus_id = $1 AND archived_at IS NULL`, corpusID).Scan(&inFlight, &done); err != nil {
		return state, fmt.Errorf("count the corpus's documents: %w", err)
	}
	var retrievable int
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM knowledge_chunk
		  WHERE corpus_id = $1 AND archived_at IS NULL AND embed_identity = $2`,
		corpusID, identity).Scan(&retrievable); err != nil {
		return state, fmt.Errorf("count the corpus's retrievable passages: %w", err)
	}
	state.Outcome = crmcontracts.KnowledgeAnswerOutcomeAnswered
	switch {
	case corpus.Reindexing != nil && *corpus.Reindexing,
		inFlight > 0,
		done == 0,
		corpus.Coverage.ChunksTotal == 0,
		// Counted at the LIVE identity, not at any identity. Vectors under
		// a superseded binding retrieve nothing, and saying not_covered
		// would blame the question for prose the corpus is holding.
		retrievable < corpus.Coverage.ChunksTotal:
		state.Outcome = crmcontracts.KnowledgeAnswerOutcomeNotReady
	}
	return state, nil
}

// questionVector embeds the question, or reports that there is nothing to rank.
// A nil vector with a nil error means exactly that.
func (s *Store) questionVector(ctx context.Context, question string, e vectorkit.Embedder, dims int) ([]float32, error) {
	question = strings.TrimSpace(question)
	if question == "" {
		return nil, nil
	}
	res, err := e.Embed(ctx, model.EmbedRequest{Inputs: []string{question}, Dimensions: dims})
	if err != nil {
		return nil, fmt.Errorf("embed the question: %w", err)
	}
	if len(res.Vectors) != 1 || res.Dims != dims {
		return nil, fmt.Errorf("the embed lane returned %d vectors of width %d, need 1×%d",
			len(res.Vectors), res.Dims, dims)
	}
	if vectorkit.IsZero(res.Vectors[0]) {
		return nil, nil
	}
	return res.Vectors[0], nil
}

// rankIn returns the corpus's closest passages to the question vector, inside
// the transaction the caller holds.
func rankIn(ctx context.Context, tx pgx.Tx, corpusID ids.UUID, vec []float32, identity string) ([]Passage, error) {
	// The `c.embed_identity = $3` predicate is load-bearing twice over, and
	// must never be removed. For correctness: a vector from an older binding
	// lives in a space this query's vector does not share, so ranking against
	// it is meaningless. For crash avoidance: the column is unbounded width, so
	// comparing two widths raises an error outright — the filter has to exclude
	// those rows BEFORE the projection computes the distance, which is why it
	// is a WHERE clause and not a HAVING.
	rows, err := tx.Query(ctx,
		`SELECT c.id, c.document_id, d.filename, c.text, coalesce(c.start_line, 0),
		        1 - (c.embedding <=> $1::vector) AS sim
		   FROM knowledge_chunk c
		   JOIN knowledge_document d ON d.id = c.document_id
		  WHERE c.corpus_id = $2
		    AND c.embed_identity = $3
		    AND c.archived_at IS NULL
		  ORDER BY c.embedding <=> $1::vector
		  LIMIT $4`,
		vectorkit.Literal(vec), corpusID, identity, retrieveLimit)
	if err != nil {
		return nil, fmt.Errorf("rank the corpus's passages: %w", err)
	}
	defer rows.Close()
	var out []Passage
	for rows.Next() {
		var p Passage
		if err := rows.Scan(&p.ChunkID, &p.DocumentID, &p.DocumentName, &p.Text, &p.StartLine, &p.Similarity); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// groundingFloorIn reads the similarity a passage must reach to be citable,
// inside the transaction the caller holds.
func groundingFloorIn(ctx context.Context, tx pgx.Tx, corpusID ids.UUID) (float64, error) {
	var floor float64
	if err := tx.QueryRow(ctx,
		`SELECT min_similarity FROM knowledge_corpus WHERE id = $1 AND archived_at IS NULL`,
		corpusID).Scan(&floor); err != nil {
		return 0, notFoundOr(err, "read the corpus's grounding floor")
	}
	return floor, nil
}
