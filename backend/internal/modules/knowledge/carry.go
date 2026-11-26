// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package knowledge

// An attempt deletes its document's passages before it writes the new ones, so
// the vectors that document already paid for are read first and carried across.
// Only the document's own: sharing across documents needs an index this table lacks.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/vectorkit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// carriedVector is one vector the document held when an attempt began.
type carriedVector struct {
	hash     string
	identity string
	vector   []float32
}

// carriedVectors holds a document's stored vectors by the hash of their text.
type carriedVectors map[string][]carriedVector

// readCarriedVectors reads every vector the document holds, live or retired by a
// replacement, inside the transaction that is about to delete them.
func readCarriedVectors(ctx context.Context, tx pgx.Tx, documentID ids.UUID) (carriedVectors, error) {
	rows, err := tx.Query(ctx,
		`SELECT text, chunk_hash, embed_identity, embedding::real[]
		   FROM knowledge_chunk
		  WHERE document_id = $1 AND embedding IS NOT NULL`, documentID)
	if err != nil {
		return nil, fmt.Errorf("read the document's stored vectors: %w", err)
	}
	defer rows.Close()
	carried := carriedVectors{}
	for rows.Next() {
		var text, hash, identity string
		var vector []float32
		if err := rows.Scan(&text, &hash, &identity, &vector); err != nil {
			return nil, fmt.Errorf("read a stored vector: %w", err)
		}
		carried.add(text, carriedVector{hash: hash, identity: identity, vector: vector})
	}
	return carried, rows.Err()
}

// add keeps a vector once per binding, and only when its row's text still hashes
// to the stored hash: a row that disagrees holds a vector of prose it no longer has.
func (c carriedVectors) add(text string, v carriedVector) {
	if (Chunk{Text: text}).Hash() != v.hash {
		return
	}
	for _, held := range c[v.hash] {
		if held.identity == v.identity {
			return
		}
	}
	c[v.hash] = append(c[v.hash], v)
}

// take splits pending passages into the ones a carried vector answers under the
// live binding, beside those vectors, and the ones still owed a model call.
func (c carriedVectors) take(pending []pendingChunk, identity string) (reused []pendingChunk, vectors [][]float32, owed []pendingChunk) {
	for _, p := range pending {
		if vector, ok := c.current(p.text, identity); ok {
			reused = append(reused, p)
			vectors = append(vectors, vector)
			continue
		}
		owed = append(owed, p)
	}
	return reused, vectors, owed
}

func (c carriedVectors) current(text, identity string) ([]float32, bool) {
	hash := Chunk{Text: text}.Hash()
	for _, v := range c[hash] {
		if vectorkit.Unchanged(v.hash, v.identity, hash, identity) {
			return v.vector, true
		}
	}
	return nil, false
}
