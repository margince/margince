// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package knowledge

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/gradionhq/margince/backend/internal/contracts"
	"github.com/gradionhq/margince/backend/internal/platform/auth"
	"github.com/gradionhq/margince/backend/internal/platform/database"
	"github.com/gradionhq/margince/backend/internal/platform/database/storekit"
	"github.com/gradionhq/margince/backend/internal/shared/apperrors"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/principal"
)

// DefaultMinSimilarity is the grounding floor a corpus starts life with: the
// cosine a passage must reach before it may be cited at all.
//
// It is a starting point rather than a tuned value, and the corpus carries
// tuned_under_identity precisely so a floor that WAS tuned can be told apart
// from one that was merely defaulted. 0.35 is deliberately permissive — the
// quote check downstream is what keeps an ungrounded sentence out of an
// answer, and a floor set high enough to do that job on its own would refuse
// questions the corpus does answer.
const DefaultMinSimilarity = 0.35

// Store owns the knowledge tables. Handlers→Store, the CRUD spine: the store
// holds the transactional write shape and nothing above it writes SQL.
type Store struct {
	// db binds the installation's workspace, so a caller does not have to have
	// put it in ctx first.
	db *database.DB
}

// NewStore wires the store over the workspace-bound app pool.
func NewStore(db *database.DB) *Store { return &Store{db: db} }

func (s *Store) tx(ctx context.Context, fn func(pgx.Tx) error) error { return s.db.Tx(ctx, fn) }

// NewCorpus defines a corpus. MinSimilarity nil takes DefaultMinSimilarity —
// the transport never defaults it, so "the caller said 0.35" and "the caller
// said nothing" stay distinguishable up to the store that decides.
type NewCorpus struct {
	Name           string
	Description    *string
	TopicStatement string
	MinSimilarity  *float64
	DefaultAsk     bool
}

// CreateCorpus defines a corpus and returns it with its (empty) coverage.
func (s *Store) CreateCorpus(ctx context.Context, in NewCorpus) (crmcontracts.KnowledgeCorpus, error) {
	if err := auth.Require(ctx, "knowledge_corpus", principal.ActionCreate); err != nil {
		return crmcontracts.KnowledgeCorpus{}, err
	}
	floor := DefaultMinSimilarity
	if in.MinSimilarity != nil {
		floor = *in.MinSimilarity
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return crmcontracts.KnowledgeCorpus{}, err
	}
	var out crmcontracts.KnowledgeCorpus
	err = s.tx(ctx, func(tx pgx.Tx) error {
		id := ids.NewV7()
		if in.DefaultAsk {
			if err := clearDefaultAsk(ctx, tx, ids.Nil); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO knowledge_corpus (id, name, description, topic_statement, min_similarity, default_ask, captured_by)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			id, in.Name, in.Description, in.TopicStatement, floor, in.DefaultAsk, by); err != nil {
			return fmt.Errorf("insert knowledge corpus: %w", err)
		}
		if _, err := storekit.Audit(ctx, tx, "create", "knowledge_corpus", id, nil, map[string]any{
			"name":            in.Name,
			"topic_statement": in.TopicStatement,
			"min_similarity":  floor,
			"default_ask":     in.DefaultAsk,
		}); err != nil {
			return fmt.Errorf("audit knowledge corpus create: %w", err)
		}
		row, err := readCorpus(ctx, tx, id, storekit.LiveOnly)
		out = row.wire()
		return err
	})
	return out, err
}

// UpdateCorpus is a sparse merge-patch: nil keeps the stored value.
type UpdateCorpus struct {
	Name           *string
	Description    *string
	TopicStatement *string
	MinSimilarity  *float64
	DefaultAsk     *bool
}

// EditCorpus applies the sparse patch. Raising default_ask takes the flag from
// whichever corpus held it, in this transaction, so the partial unique index is
// never the thing that reports the collision.
func (s *Store) EditCorpus(ctx context.Context, id ids.UUID, in UpdateCorpus) (crmcontracts.KnowledgeCorpus, error) {
	if err := auth.Require(ctx, "knowledge_corpus", principal.ActionUpdate); err != nil {
		return crmcontracts.KnowledgeCorpus{}, err
	}
	var out crmcontracts.KnowledgeCorpus
	err := s.tx(ctx, func(tx pgx.Tx) error {
		lock, err := storekit.LockRow(ctx, tx, "knowledge_corpus", id, storekit.LiveOnly)
		if err != nil {
			return err
		}
		row, err := readCorpus(ctx, tx, id, storekit.LiveOnly)
		if err != nil {
			return err
		}
		current := row.wire()
		if in.DefaultAsk != nil && *in.DefaultAsk {
			if err := clearDefaultAsk(ctx, tx, id); err != nil {
				return err
			}
		}
		p := buildCorpusPatch(current, in)
		if p.Empty() {
			out = current
			return nil
		}
		if err := p.ApplyLocked(ctx, tx, lock); err != nil {
			return fmt.Errorf("apply knowledge corpus patch: %w", err)
		}
		if _, err := storekit.Audit(ctx, tx, "update", "knowledge_corpus", id, p.Before(), p.After()); err != nil {
			return fmt.Errorf("audit knowledge corpus update: %w", err)
		}
		updated, err := readCorpus(ctx, tx, id, storekit.LiveOnly)
		out = updated.wire()
		return err
	})
	return out, err
}

// buildCorpusPatch folds the sparse edit into a patch carrying each set
// field's before/after image. updated_at is absent on purpose: the table's
// BEFORE UPDATE trigger owns it, so a patch that named it would be the second
// writer of one value.
func buildCorpusPatch(current crmcontracts.KnowledgeCorpus, in UpdateCorpus) *storekit.Patch {
	p := storekit.NewPatch()
	if in.Name != nil {
		p.Set("name", current.Name, *in.Name)
	}
	if in.Description != nil {
		p.Set("description", current.Description, *in.Description)
	}
	if in.TopicStatement != nil {
		p.Set("topic_statement", current.TopicStatement, *in.TopicStatement)
	}
	if in.MinSimilarity != nil {
		p.Set("min_similarity", current.MinSimilarity, *in.MinSimilarity)
	}
	if in.DefaultAsk != nil {
		p.Set("default_ask", current.DefaultAsk, *in.DefaultAsk)
	}
	return p
}

// clearDefaultAsk lowers the flag everywhere except keep, in the same
// transaction as the write that raises it. Two defaults is a palette that asks
// a different corpus depending on row order, so the second one MOVES the flag
// rather than joining it — and doing that here rather than leaving it to the
// partial unique index is what makes the second create a success instead of a
// 409 the caller cannot act on.
func clearDefaultAsk(ctx context.Context, tx pgx.Tx, keep ids.UUID) error {
	if _, err := tx.Exec(ctx,
		`UPDATE knowledge_corpus SET default_ask = false
		 WHERE default_ask AND archived_at IS NULL AND id <> $1`, keep); err != nil {
		return fmt.Errorf("clear the previous default corpus: %w", err)
	}
	return nil
}

// ArchiveCorpus archives the corpus and everything filed in it, in one
// transaction: the documents, and their chunks with them.
//
// The chunks are stamped rather than joined through to their document at ask
// time. An archived document whose chunks stayed retrievable would be cited by
// name from a corpus the screen shows as empty, and the join would put a
// row-visibility question on the ask's hot path to prevent it.
//
// A repeat archive is a no-op: same answer, no second audit row.
func (s *Store) ArchiveCorpus(ctx context.Context, id ids.UUID) error {
	if err := auth.Require(ctx, "knowledge_corpus", principal.ActionDelete); err != nil {
		return err
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		current, err := readCorpus(ctx, tx, id, storekit.IncludeArchived)
		if err != nil {
			return err
		}
		if current.ArchivedAt() != nil {
			return nil
		}
		if _, err := tx.Exec(ctx,
			`UPDATE knowledge_chunk SET archived_at = now()
			 WHERE corpus_id = $1 AND archived_at IS NULL`, id); err != nil {
			return fmt.Errorf("archive the corpus's chunks: %w", err)
		}
		if _, err := tx.Exec(ctx,
			`UPDATE knowledge_document SET archived_at = now()
			 WHERE corpus_id = $1 AND archived_at IS NULL`, id); err != nil {
			return fmt.Errorf("archive the corpus's documents: %w", err)
		}
		if _, err := tx.Exec(ctx,
			`UPDATE knowledge_corpus SET archived_at = now()
			 WHERE id = $1 AND archived_at IS NULL`, id); err != nil {
			return fmt.Errorf("archive knowledge corpus: %w", err)
		}
		if _, err := storekit.Audit(ctx, tx, "archive", "knowledge_corpus", id, nil, nil); err != nil {
			return fmt.Errorf("audit knowledge corpus archive: %w", err)
		}
		return nil
	})
}

// ReadCorpus resolves one corpus and its coverage.
func (s *Store) ReadCorpus(ctx context.Context, id ids.UUID) (crmcontracts.KnowledgeCorpus, error) {
	if err := auth.Require(ctx, "knowledge_corpus", principal.ActionRead); err != nil {
		return crmcontracts.KnowledgeCorpus{}, err
	}
	var out crmcontracts.KnowledgeCorpus
	err := s.tx(ctx, func(tx pgx.Tx) error {
		row, err := readCorpus(ctx, tx, id, storekit.LiveOnly)
		out = row.wire()
		return err
	})
	return out, err
}

// ListCorpora returns the workspace's live corpora, newest first.
//
// No cursor: a workspace defines corpora by hand, in ones and twos, and the
// screen shows all of them. A page envelope here would be a contract promise
// with no producer behind it.
func (s *Store) ListCorpora(ctx context.Context) ([]crmcontracts.KnowledgeCorpus, error) {
	if err := auth.Require(ctx, "knowledge_corpus", principal.ActionRead); err != nil {
		return nil, err
	}
	out := []crmcontracts.KnowledgeCorpus{}
	err := s.tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT `+corpusColumns+` FROM knowledge_corpus c
			 WHERE archived_at IS NULL ORDER BY created_at DESC, id DESC`)
		if err != nil {
			return fmt.Errorf("list knowledge corpora: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			c, err := scanCorpus(rows)
			if err != nil {
				return err
			}
			out = append(out, c.wire())
		}
		return rows.Err()
	})
	return out, err
}

// corpusColumns is the read shape, coverage included.
//
// The three counts are correlated subqueries rather than a GROUP BY join: a
// corpus with no documents must report zeros and stay in the list, and an
// aggregate join drops exactly that row unless it is spelled as an outer join
// whose COUNT then has to be coalesced anyway.
const corpusColumns = `c.id, c.name, c.description, c.topic_statement, c.min_similarity,
	c.default_ask, c.reindexing, c.created_at, c.archived_at,
	(SELECT count(*) FROM knowledge_document d WHERE d.corpus_id = c.id AND d.archived_at IS NULL),
	(SELECT count(*) FROM knowledge_chunk k WHERE k.corpus_id = c.id AND k.archived_at IS NULL),
	(SELECT count(*) FROM knowledge_chunk k WHERE k.corpus_id = c.id AND k.archived_at IS NULL AND k.embed_identity IS NOT NULL)`

// corpusRow is the scanned row. It exists because the contract type carries no
// archived_at and the archive path needs to know — an already-archived corpus
// is a no-op, not a second audit row.
type corpusRow struct {
	corpus     crmcontracts.KnowledgeCorpus
	archivedAt *time.Time
}

func (r corpusRow) wire() crmcontracts.KnowledgeCorpus { return r.corpus }

// ArchivedAt reports when the corpus was archived, or nil while it is live.
func (r corpusRow) ArchivedAt() *time.Time { return r.archivedAt }

func readCorpus(ctx context.Context, tx pgx.Tx, id ids.UUID, archived storekit.ArchivedFilter) (corpusRow, error) {
	q := `SELECT ` + corpusColumns + ` FROM knowledge_corpus c WHERE c.id = $1`
	if archived == storekit.LiveOnly {
		q += ` AND c.archived_at IS NULL`
	}
	row, err := scanCorpus(tx.QueryRow(ctx, q, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return corpusRow{}, apperrors.ErrNotFound
	}
	return row, err
}

func scanCorpus(row pgx.Row) (corpusRow, error) {
	var out corpusRow
	var id ids.UUID
	var reindexing bool
	var coverage crmcontracts.KnowledgeCoverage
	if err := row.Scan(
		&id, &out.corpus.Name, &out.corpus.Description, &out.corpus.TopicStatement,
		&out.corpus.MinSimilarity, &out.corpus.DefaultAsk, &reindexing,
		&out.corpus.CreatedAt, &out.archivedAt,
		&coverage.DocumentsTotal, &coverage.ChunksTotal, &coverage.ChunksEmbedded,
	); err != nil {
		return corpusRow{}, err
	}
	out.corpus.Id = openapi_types.UUID(id)
	out.corpus.Coverage = coverage
	out.corpus.Reindexing = &reindexing
	return out, nil
}
