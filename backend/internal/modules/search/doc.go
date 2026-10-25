// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package search owns cross-object retrieval. That is ranked full-text search
// over the generated search_tsv columns, the pgvector and full-text hybrid
// fused by RRF, and the context graph. Every branch of every query carries
// the caller's row-scope predicate. A search hit is a read, so existence-hiding
// holds here as on the per-entity lists.
//
// Tables owned: docs/reference/entity-model/search.md. They are the retrieval
// slice and the interaction projections, all derived and rebuildable. Domain tables are read through their declared
// indexes only. Imports shared + platform only; never a sibling module.
package search
