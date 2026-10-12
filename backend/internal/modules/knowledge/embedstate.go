// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package knowledge

import (
	"slices"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// ingestInFlight is a document whose ingest has not settled, over alias d.
const ingestInFlight = `d.ingest_status IN ('queued', 'running')`

// passageColumns selects live passage totals, vectored passages and the
// identities those vectors were computed under, over the chunks scope matches.
// scope is always a compile-time literal naming the alias k.
func passageColumns(scope string) string {
	live := `FROM knowledge_chunk k WHERE ` + scope + ` AND k.archived_at IS NULL`
	return `(SELECT count(*) ` + live + `),
	(SELECT count(*) ` + live + ` AND k.embed_identity IS NOT NULL),
	(SELECT array_agg(DISTINCT k.embed_identity) ` + live + ` AND k.embed_identity IS NOT NULL)`
}

// passageCoverage is what a corpus or one document still owes before its
// passages are searchable. Both read their embed state through state.
type passageCoverage struct {
	total      int
	embedded   int
	identities []string
	inFlight   bool
	reindexing bool
}

// state reads the coverage under the binding named by identity. An empty
// identity is the unbound lane /me reports as no reindex surface.
func (c passageCoverage) state(identity string) crmcontracts.KnowledgeEmbedState {
	if identity == "" {
		return crmcontracts.KnowledgeEmbedStateNoEmbeddingModel
	}
	stale := slices.ContainsFunc(c.identities, func(stored string) bool { return stored != identity })
	if c.reindexing || c.inFlight || c.embedded < c.total || stale {
		return crmcontracts.KnowledgeEmbedStateIndexing
	}
	return crmcontracts.KnowledgeEmbedStateComplete
}
