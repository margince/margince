// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package search owns cross-object retrieval (B-EP05.15+): ranked
// full-text search over the generated search_tsv columns today, the
// pgvector/RRF hybrid and the context graph as they land. Every branch
// of every query carries the caller's row-scope predicate — a search
// hit IS a read, so existence-hiding holds here exactly as on the
// per-entity lists.
//
// Tables owned: embed_store_binding, embedding, graph_interaction_edge,
// graph_contact_edge — the retrieval slice and the interaction projections,
// all derived and rebuildable. Domain tables are read through their declared
// indexes only. Imports shared + platform only; never a sibling module.
package search
