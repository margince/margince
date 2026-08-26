// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package knowledge

// The deterministic half of asking a corpus: everything decided WITHOUT a
// model.
//
// Three of the four outcomes are settled here, and the fourth is only made
// possible here. That split is the design: a refusal must never depend on a
// model call, or the product cannot say why it refused.
//
// The three refusals say different things and are never interchangeable:
//
//   not_ready              — about the CORPUS. It is still ingesting, holds
//                            nothing, or is being re-embedded.
//   retrieval_unavailable  — about the INSTALLATION. No embed lane is bound, so
//                            nothing was searched at all.
//   not_covered            — about the QUESTION. The corpus was searched, in
//                            full, and holds nothing close enough to ground an
//                            answer.
//
// Collapsing them is the failure this endpoint exists to avoid. Answering
// "your documents do not cover this" for a corpus that is merely half-ingested
// is an affirmative false claim about documents that DO cover it.

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/gradionhq/margince/backend/internal/contracts"
	"github.com/gradionhq/margince/backend/internal/platform/auth"
	"github.com/gradionhq/margince/backend/internal/platform/database/storekit"
	"github.com/gradionhq/margince/backend/internal/platform/vectorkit"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/principal"
	"github.com/gradionhq/margince/backend/internal/shared/ports/model"
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
		// An empty question, or one the lane could only answer with a zero
		// vector. Every cosine against zero is NaN and `ORDER BY sim DESC`
		// sorts NaN first, so there is nothing to rank and nothing honest to
		// say except that the corpus does not cover it.
		state.Outcome = crmcontracts.KnowledgeAnswerOutcomeNotCovered
		return state, nil, nil
	}

	passages, err := s.rank(ctx, corpusID, vec, identity)
	if err != nil {
		return state, nil, err
	}
	floor, err := s.groundingFloor(ctx, corpusID)
	if err != nil {
		return state, nil, err
	}
	grounded := passages[:0]
	for _, p := range passages {
		if p.Similarity >= floor {
			grounded = append(grounded, p)
		}
	}
	if len(grounded) == 0 {
		state.Outcome = crmcontracts.KnowledgeAnswerOutcomeNotCovered
		return state, nil, nil
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
	err := s.tx(ctx, func(tx pgx.Tx) error {
		row, err := readCorpus(ctx, tx, corpusID, storekit.LiveOnly)
		if err != nil {
			return err
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
			return fmt.Errorf("count the corpus's documents: %w", err)
		}
		var retrievable int
		if err := tx.QueryRow(ctx,
			`SELECT count(*) FROM knowledge_chunk
			  WHERE corpus_id = $1 AND archived_at IS NULL AND embed_identity = $2`,
			corpusID, identity).Scan(&retrievable); err != nil {
			return fmt.Errorf("count the corpus's retrievable passages: %w", err)
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
		return nil
	})
	return state, err
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

// rank returns the corpus's closest passages to the question vector.
func (s *Store) rank(ctx context.Context, corpusID ids.UUID, vec []float32, identity string) ([]Passage, error) {
	var out []Passage
	err := s.tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			// The `c.embed_identity = $3` predicate is load-bearing twice over,
			// and must never be removed. For correctness: a vector from an
			// older binding lives in a space this query's vector does not
			// share, so ranking against it is meaningless. For crash
			// avoidance: the column is unbounded width, so comparing two widths
			// raises an error outright — the filter has to exclude those rows
			// BEFORE the projection computes the distance, which is why it is a
			// WHERE clause and not a HAVING.
			`SELECT c.id, c.document_id, d.filename, c.text, 1 - (c.embedding <=> $1::vector) AS sim
			   FROM knowledge_chunk c
			   JOIN knowledge_document d ON d.id = c.document_id
			  WHERE c.corpus_id = $2
			    AND c.embed_identity = $3
			    AND c.archived_at IS NULL
			  ORDER BY c.embedding <=> $1::vector
			  LIMIT $4`,
			vectorkit.Literal(vec), corpusID, identity, retrieveLimit)
		if err != nil {
			return fmt.Errorf("rank the corpus's passages: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var p Passage
			if err := rows.Scan(&p.ChunkID, &p.DocumentID, &p.DocumentName, &p.Text, &p.Similarity); err != nil {
				return err
			}
			out = append(out, p)
		}
		return rows.Err()
	})
	return out, err
}

// groundingFloor reads the similarity a passage must reach to be citable.
func (s *Store) groundingFloor(ctx context.Context, corpusID ids.UUID) (float64, error) {
	var floor float64
	err := s.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT min_similarity FROM knowledge_corpus WHERE id = $1 AND archived_at IS NULL`,
			corpusID).Scan(&floor)
	})
	if err != nil {
		return 0, notFoundOr(err, "read the corpus's grounding floor")
	}
	return floor, nil
}
