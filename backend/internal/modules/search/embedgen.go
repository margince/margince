// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	kevents "github.com/margince/margince/backend/internal/shared/kernel/events"
)

// EmbedGen keeps the vector store current: it consumes entity events
// off the bus (cg:context-graph) and re-embeds the changed entity's
// text. The content-hash guard in UpsertEmbedding makes redelivery and
// no-op updates free — at-least-once delivery costs no model calls.
type EmbedGen struct {
	store    *Store
	embedder Embedder
}

func NewEmbedGen(store *Store, embedder Embedder) *EmbedGen {
	return &EmbedGen{store: store, embedder: embedder}
}

// The embeddable entity types, named once so embedText and binding.go's
// pendingSources (the per-id and set-form views of the same source columns)
// key off the same identifiers rather than each repeating the literal.
const (
	entityPerson        = "person"
	entityOrganization  = "organization"
	entityDeal          = "deal"
	entityLead          = "lead"
	entityActivity      = "activity"
	entityProject       = "project"
	entityProduct       = "product"
	entityOfferTemplate = "offer_template"
)

// embedText mirrors each entity's search_tsv source columns — the
// vector lane and the lexical lane index the same content, so a hybrid
// hit means agreement about one text, not two.
var embedText = map[string]string{
	entityPerson:       `SELECT full_name FROM person WHERE id = $1 AND archived_at IS NULL`,
	entityOrganization: `SELECT concat_ws(' ', display_name, legal_name, industry) FROM organization WHERE id = $1 AND archived_at IS NULL`,
	entityDeal:         `SELECT name FROM deal WHERE id = $1 AND archived_at IS NULL`,
	entityLead:         `SELECT concat_ws(' ', full_name, company_name, title) FROM lead WHERE id = $1 AND archived_at IS NULL`,
	// The audience clause is the vector lane's half of capture privacy. An
	// embedding is built as the system principal and queried by everyone, so a
	// held message indexed here is retrievable by semantic neighbourhood no
	// matter how tightly the row itself is scoped — the query-side gate filters
	// the ROWS it returns, not the fact that a phrase from a colleague's legal
	// mail is what pulled them back. A limited activity is therefore not indexed
	// at all, and audiencerescope drops the vector when a row narrows.
	entityActivity: `SELECT concat_ws(' ', subject, body) FROM activity WHERE id = $1 AND archived_at IS NULL AND audience = 'workspace'`,
	entityProject:  `SELECT concat_ws(' ', name, key, description) FROM project WHERE id = $1 AND archived_at IS NULL`,
	// Not narrowed on `active`: the lexical branch indexes a discontinued
	// product too, and a vector lane that dropped it would answer the same
	// question two different ways depending on which lane ranked first.
	entityProduct: `SELECT concat_ws(' ', name, sku, description) FROM product WHERE id = $1 AND archived_at IS NULL`,
	// One column, because one column is all this table holds as prose.
	entityOfferTemplate: `SELECT name FROM offer_template WHERE id = $1 AND archived_at IS NULL`,
}

// HandleEvent maintains embeddings for created/updated/captured
// entities. Events that are not entity-content changes are not ours —
// nil, so the consumer group keeps flowing.
func (g *EmbedGen) HandleEvent(ctx context.Context, env kevents.Envelope) error {
	query, embeddable := embedText[env.Entity.Type]
	if !embeddable || !contentChanging(env.Type) {
		return nil
	}
	// The generator reads AS the system: embeddings are an index over
	// the whole workspace, filtered per caller at QUERY time — an
	// index built through one user's row scope would silently hide
	// records from everyone else's retrieval.
	// The workspace is the STORE's, not the envelope's: this consumer is
	// wired for one installation and its handle already names it (ADR-0091 §6
	// — the envelope carries no tenant).
	ws, err := g.store.db.Workspace(ctx)
	if err != nil {
		return err
	}
	wsCtx := systemWorkspaceContext(ctx, ws.UUID)

	var text string
	err = g.store.db.Tx(wsCtx, func(tx pgx.Tx) error {
		return tx.QueryRow(wsCtx, query, env.Entity.ID).Scan(&text)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil // archived or gone since the event — nothing to index
	}
	if err != nil {
		return fmt.Errorf("search: loading %s %s for embedding: %w", env.Entity.Type, env.Entity.ID, err)
	}
	_, err = g.store.UpsertEmbedding(wsCtx, env.Entity.Type, env.Entity.ID, text, g.embedder)
	return err
}

// contentChanging filters the event types whose payload implies the
// indexed text may have moved.
func contentChanging(eventType string) bool {
	for _, suffix := range []string{".created", ".updated", ".captured", ".promoted", ".merged"} {
		if strings.HasSuffix(eventType, suffix) {
			return true
		}
	}
	return false
}
